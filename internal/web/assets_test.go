package web

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/CBOC-Vita-Lab/vita-installer/internal/fakevita"
)

// GitHub releases are handled by what their files are, whatever they are called.
func TestReleaseAssetsByContent(t *testing.T) {
	tmp := t.TempDir()
	vpkPath := filepath.Join(tmp, "x.vpk")
	if err := fakevita.MakeVPK(vpkPath, "MSCN00003", "Mario Strikers Charged", nil); err != nil {
		t.Fatal(err)
	}
	vpk, _ := os.ReadFile(vpkPath)
	sum := sha256.Sum256(vpk)
	vpkSHA := hex.EncodeToString(sum[:])
	manifest := fmt.Sprintf(`{"schemaVersion":1,"titleId":"MSCN00003","displayName":"Mario Strikers Charged","releaseState":"ready",
	 "package":{"path":"Some-Old-Name.vpk","sha256":%q},"requiredInputs":[{"id":"game","kind":"strikers-disc"}],
	 "destinations":{"data":"ux0:data/mscharged/game"}}`, vpkSHA)
	zipOf := func(files map[string][]byte) []byte {
		var b bytes.Buffer
		z := zip.NewWriter(&b)
		for n, d := range files {
			w, _ := z.Create(n)
			w.Write(d)
		}
		z.Close()
		return b.Bytes()
	}
	files := map[string][]byte{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		d, ok := files[r.URL.Path[1:]]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Write(d)
	}))
	defer srv.Close()
	as := func(name string, digest bool) asset {
		a := asset{Name: name, URL: srv.URL + "/" + name, Size: int64(len(files[name]))}
		if digest {
			h := sha256.Sum256(files[name])
			a.Digest = "sha256:" + hex.EncodeToString(h[:])
		}
		return a
	}
	cases := []struct {
		name   string
		files  map[string][]byte
		assets func() []asset
	}{
		{"renamed app and release file", map[string][]byte{"Strikers-Charged-Vita.vpk": vpk, "info.json": []byte(manifest)},
			func() []asset { return []asset{as("Strikers-Charged-Vita.vpk", true), as("info.json", false)} }},
		{"app with no extension, found by checksum", map[string][]byte{"game": vpk, "other": bytes.Repeat([]byte("z"), 70000), "cboc-release.json": []byte(manifest)},
			func() []asset { return []asset{as("other", false), as("game", true), as("cboc-release.json", false)} }},
		{"source-only ZIP is skipped", map[string][]byte{"Game-1.0.zip": zipOf(map[string][]byte{"src/main.c": []byte("x")}), "Strikers.vpk": vpk, "cboc-release.json": []byte(manifest)},
			func() []asset {
				return []asset{as("Game-1.0.zip", false), as("Strikers.vpk", false), as("cboc-release.json", false)}
			}},
		{"complete ZIP under any name", map[string][]byte{"download.zip": zipOf(map[string][]byte{"a/whatever.vpk": vpk, "a/cboc-release.json": []byte(manifest)})},
			func() []asset { return []asset{as("download.zip", true)} }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			for k := range files {
				delete(files, k)
			}
			for k, v := range c.files {
				files[k] = v
			}
			app := NewApp("1.2", t.TempDir(), t.TempDir())
			if err := app.addRelease(context.Background(), "CBOC-Vita-Lab/test", ghRelease{Tag: "v1.0", Assets: c.assets()}); err != nil {
				t.Fatal(err)
			}
			if len(app.st.Items) != 1 {
				t.Fatalf("%d items", len(app.st.Items))
			}
			it := app.st.Items[0]
			if it.TitleID != "MSCN00003" || it.Tag != "v1.0" || len(it.Inputs) == 0 || it.Inputs[0].Type != "wii-disc" {
				t.Fatalf("release not read with its release file: %+v %+v", it.Release, it.Inputs)
			}
		})
	}
	t.Run("damaged download is refused", func(t *testing.T) {
		files["Strikers.vpk"] = vpk
		bad := as("Strikers.vpk", false)
		bad.Digest = "sha256:" + hex.EncodeToString(make([]byte, 32))
		app := NewApp("1.2", t.TempDir(), t.TempDir())
		if err := app.addRelease(context.Background(), "CBOC-Vita-Lab/test", ghRelease{Tag: "v1.0", Assets: []asset{bad}}); err == nil {
			t.Fatal("a file with the wrong checksum was accepted")
		}
	})
}
