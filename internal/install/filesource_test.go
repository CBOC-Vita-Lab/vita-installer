package install

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/CBOC-Vita-Lab/vita-installer/internal/release"
)

func hdInput() *release.Input {
	m, err := release.ParseManifest([]byte(`{"schemaVersion":1,"titleId":"MSCN00003","package":{"path":"a.vpk"},
		"requiredInputs":[{"id":"game","kind":"strikers-disc"}],"destinations":{"data":"ux0:data/mscharged/game"}}`))
	if err != nil {
		panic(err)
	}
	for _, n := range m.Needs {
		if n.ID == "hd" {
			return &release.Input{Need: n}
		}
	}
	panic("no HD slot")
}

func TestHDPackAnyShape(t *testing.T) {
	dir := t.TempDir()
	pack := append([]byte("HDTX\x01\x00\x00\x00"), bytes.Repeat([]byte{7}, 5000)...)
	write := func(name string, b []byte) string {
		p := filepath.Join(dir, name)
		os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, b, 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	renamed := write("my textures.bin", pack)
	write("split/hdtex.pack.part00", pack[:2000])
	write("split/hdtex.pack.part01", pack[2000:4000])
	write("split/hdtex.pack.part02", pack[4000:])
	var zb bytes.Buffer
	zw := zip.NewWriter(&zb)
	w, _ := zw.Create("Strikers HD/hdtex.pack")
	w.Write(pack)
	w, _ = zw.Create("Strikers HD/README.txt")
	w.Write([]byte("hello"))
	zw.Close()
	zipped := write("Strikers HD textures.zip", zb.Bytes())
	write("folder/inner/hdtex2.pack", pack)
	wrong := write("wrong.pack", []byte("HDTX\x02\x00\x00\x00 later version"))

	for _, p := range []string{renamed, filepath.Join(dir, "split/hdtex.pack.part00"), zipped, filepath.Join(dir, "folder"), filepath.Join(dir, "split")} {
		in := hdInput()
		CheckInput(in, p)
		if in.State != "ok" || in.Bytes != int64(len(pack)) {
			t.Fatalf("%s: %s %q %d", p, in.State, in.Message, in.Bytes)
		}
		src, _ := findFileSource(in, p)
		local, temp, err := src.local(filepath.Join(dir, "cache"))
		if err != nil {
			t.Fatal(err)
		}
		got, _ := os.ReadFile(local)
		if !bytes.Equal(got, pack) {
			t.Fatalf("%s: contents differ", p)
		}
		if temp {
			os.Remove(local)
		}
	}
	in := hdInput()
	CheckInput(in, wrong)
	if in.State != "error" {
		t.Fatal("a pack of another format was accepted")
	}
	if in.To != "ux0:data/mscharged/hdtex.pack" || !in.Optional {
		t.Fatal(in.To)
	}
}
