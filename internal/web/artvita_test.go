package web

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/jpeg"
	"os"
	"path/filepath"
	"testing"

	"github.com/CBOC-Vita-Lab/vita-installer/internal/art"
	"github.com/CBOC-Vita-Lab/vita-installer/internal/device"
	"github.com/CBOC-Vita-Lab/vita-installer/internal/fakevita"
)

// The "Your Vita" artwork source offers the game's own title capture and the
// screenshots taken in it, and they can be chosen like any other picture.
func TestArtFromTheVita(t *testing.T) {
	dir := t.TempDir()
	rel := filepath.Join(dir, "Galaxy")
	os.MkdirAll(rel, 0o755)
	if err := fakevita.MakeVPK(filepath.Join(rel, "Galaxy.vpk"), "SMGV00001", "Galaxy", nil); err != nil {
		t.Fatal(err)
	}
	m, _ := json.Marshal(map[string]interface{}{"needs": []interface{}{}})
	os.WriteFile(filepath.Join(rel, "vita-install.json"), m, 0o644)
	v, err := fakevita.Start(filepath.Join(dir, "vita"), true, false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(v.Close)
	app := NewApp("2.0", t.TempDir(), t.TempDir())
	if err := app.Add(rel); err != nil {
		t.Fatal(err)
	}
	app.bg.Wait()
	item := app.st.Items[0].ID

	if _, err := app.ArtSearch(context.Background(), item, "vita", "", ""); err == nil {
		t.Fatal("no Vita connected, but no error")
	}
	app.st.Vita = &device.Vita{Host: "127.0.0.1", Port: v.Port()}
	if _, err := app.ArtSearch(context.Background(), item, "vita", "", ""); err == nil {
		t.Fatal("nothing on the Vita, but no error")
	}

	os.MkdirAll(filepath.Join(v.Root, "ux0/data/smg/art"), 0o755)
	os.WriteFile(filepath.Join(v.Root, "ux0/data/smg/art/title.rgba"), bytes.Repeat([]byte{10, 20, 200, 0}, 960*544), 0o644)
	shots := filepath.Join(v.Root, "ux0/picture/SCREENSHOT/SMGV00001")
	os.MkdirAll(shots, 0o755)
	var jb bytes.Buffer
	jpeg.Encode(&jb, image.NewRGBA(image.Rect(0, 0, 960, 544)), nil)
	os.WriteFile(filepath.Join(shots, "2026-11-06-130000.jpg"), jb.Bytes(), 0o644)
	os.WriteFile(filepath.Join(shots, "broken.jpg"), []byte("not a picture"), 0o644)

	res, err := app.ArtSearch(context.Background(), item, "vita", "", "")
	if err != nil {
		t.Fatal(err)
	}
	cands := res.(map[string]interface{})["candidates"].([]art.Candidate)
	if len(cands) != 4 {
		t.Fatalf("%d candidates: %+v", len(cands), cands)
	}
	if _, err := art.Thumb(context.Background(), app.cache, cands[0].Thumb); err != nil {
		t.Fatalf("thumbnail: %v", err)
	}
	if err := app.SetArt(context.Background(), item, "background", "", cands[1].URL, "Your Vita, title"); err != nil {
		t.Fatal(err)
	}
	app.bg.Wait()
	if b, err := app.ArtPreview(item, "background"); err != nil || len(b) == 0 {
		t.Fatalf("preview: %v", err)
	}
	// Links outside the copied pictures are refused.
	outside := filepath.Join(dir, "elsewhere.png")
	os.WriteFile(outside, jb.Bytes(), 0o644)
	if err := app.SetArt(context.Background(), item, "icon", "", art.LinkFor(outside), "x"); err == nil {
		t.Fatal("a file: link outside the Vita pictures was accepted")
	}
	app.bg.Wait()
}
