package install

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/CBOC-Vita-Lab/vita-installer/internal/device"
	"github.com/CBOC-Vita-Lab/vita-installer/internal/fakevita"
	"github.com/CBOC-Vita-Lab/vita-installer/internal/release"
)

// A release whose game files go through a converter it ships, then a
// decompression step and a check against its own file list.
func TestPreparePipelineWithTool(t *testing.T) {
	dir := t.TempDir()
	rel := filepath.Join(dir, "Port")
	os.MkdirAll(filepath.Join(rel, "tools"), 0o755)
	fakevita.MakeVPK(filepath.Join(rel, "Port.vpk"), "PORT00001", "Port", nil)

	// The converter: copies every file and adds converted.txt.
	src := filepath.Join(dir, "conv.go")
	os.WriteFile(src, []byte(`package main
import ("os";"path/filepath")
func main(){ in,out:=os.Args[1],os.Args[2]
 filepath.Walk(in,func(p string,i os.FileInfo,e error)error{ if e!=nil||i.IsDir(){return e}; r,_:=filepath.Rel(in,p); b,_:=os.ReadFile(p); os.MkdirAll(filepath.Dir(filepath.Join(out,r)),0755); return os.WriteFile(filepath.Join(out,r),b,0644)})
 os.WriteFile(filepath.Join(out,"converted.txt"),[]byte("ok"),0644)}`), 0o644)
	exe := filepath.Join(rel, "tools", "conv")
	if runtime.GOOS == "windows" {
		exe += ".exe"
	}
	if out, err := exec.Command("go", "build", "-o", exe, src).CombinedOutput(); err != nil {
		t.Fatalf("%v %s", err, out)
	}
	prog, _ := os.ReadFile(exe)
	sum := sha256.Sum256(prog)
	key := runtime.GOOS + "-" + runtime.GOARCH

	// The player's game folder: one Yaz0-compressed file.
	game := filepath.Join(dir, "MyGame")
	os.MkdirAll(game, 0o755)
	plain := bytes.Repeat([]byte("hello vita "), 20)
	os.WriteFile(filepath.Join(game, "data.bin"), yaz0Store(plain), 0o644)
	expect := sha256.Sum256(plain)
	cat := map[string]interface{}{"files": [][]interface{}{
		{"data.bin", len(plain), hex.EncodeToString(expect[:])},
		{"converted.txt", 2, "2689367b205c16ce32ed4200942b8b8b1e262dfc70d9bc9fbc77c49699a4f1df"},
	}}
	b, _ := json.Marshal(cat)
	os.WriteFile(filepath.Join(rel, "files.json"), b, 0o644)
	m := map[string]interface{}{
		"needs": []map[string]interface{}{{
			"id": "game", "type": "folder", "to": "ux0:data/port", "label": "Your game folder",
			"prepare": []map[string]interface{}{
				{"step": "tool", "program": map[string]string{key: "tools/" + filepath.Base(exe)}, "sha256": map[string]string{key: hex.EncodeToString(sum[:])}, "args": []string{"{in}", "{out}"}},
				{"step": "yaz0"},
				{"step": "verify", "catalog": "files.json"},
			},
		}},
	}
	b, _ = json.Marshal(m)
	os.WriteFile(filepath.Join(rel, release.ManifestName), b, 0o644)

	r := load(t, rel)
	var consent *release.Input
	for _, in := range r.Inputs {
		switch in.Type {
		case release.NeedFolder:
			CheckInput(in, game)
		case release.NeedConsent:
			consent = in
		}
	}
	if consent == nil {
		t.Fatal("running a release's converter must ask first")
	}
	vroot := filepath.Join(dir, "vita")
	v, _ := fakevita.Start(vroot, true, false)
	defer v.Close()
	vita := &device.Vita{Host: "127.0.0.1", Port: v.Port()}
	if _, err := Run(context.Background(), r, Options{Vita: vita, Cache: t.TempDir()}); err == nil {
		t.Fatal("installed without permission to run the converter")
	}
	CheckInput(consent, "yes")
	if _, err := Run(context.Background(), r, Options{Vita: vita, Cache: t.TempDir()}); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(filepath.Join(vroot, "ux0/data/port/data.bin"))
	if !bytes.Equal(got, plain) {
		t.Fatal("data.bin was not decompressed")
	}
	mustFile(t, vroot, "ux0/data/port/converted.txt")
	p, err := Check(context.Background(), vita, "", r)
	if err != nil || !p.Inputs["game"] {
		t.Fatalf("prepared data not recognised on the Vita: %v %+v", err, p)
	}
	// A converter that does not match its hash is refused.
	os.WriteFile(exe, append(prog, 0), 0o755)
	r2 := load(t, rel)
	for _, in := range r2.Inputs {
		if in.Type == release.NeedFolder {
			CheckInput(in, game)
		} else if in.Type == release.NeedConsent {
			CheckInput(in, "yes")
		}
	}
	if _, err := Run(context.Background(), r2, Options{Vita: vita, Cache: t.TempDir()}); err == nil {
		t.Fatal("a tampered converter ran")
	}
}

// yaz0Store wraps data in a Yaz0 stream made only of literal bytes.
func yaz0Store(data []byte) []byte {
	out := []byte("Yaz0")
	out = append(out, byte(len(data)>>24), byte(len(data)>>16), byte(len(data)>>8), byte(len(data)))
	out = append(out, make([]byte, 8)...)
	for i := 0; i < len(data); i += 8 {
		out = append(out, 0xFF)
		end := i + 8
		if end > len(data) {
			end = len(data)
		}
		out = append(out, data[i:end]...)
	}
	return out
}
