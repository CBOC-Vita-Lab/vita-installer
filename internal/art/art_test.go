package art

import (
	"archive/zip"
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func writePNG(t *testing.T, path string, w, h int, c color.NRGBA) {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.NRGBA{uint8(x * 255 / w), uint8(y * 255 / h), c.B, 255})
		}
	}
	f, _ := os.Create(path)
	png.Encode(f, img)
	f.Close()
}

func TestBuildReplacesPicturesAsVitaPNGs(t *testing.T) {
	dir := t.TempDir()
	// A VPK with a template that names its own pictures.
	vpk := filepath.Join(dir, "game.vpk")
	f, _ := os.Create(vpk)
	z := zip.NewWriter(f)
	add := func(n string, b []byte) { w, _ := z.Create(n); w.Write(b) }
	add("eboot.bin", []byte("ELF"))
	add("sce_sys/param.sfo", []byte("SFO"))
	var small bytes.Buffer
	png.Encode(&small, image.NewNRGBA(image.Rect(0, 0, 128, 128)))
	add("sce_sys/icon0.png", small.Bytes())
	add("sce_sys/livearea/contents/back.png", small.Bytes())
	add("sce_sys/livearea/contents/gate.png", small.Bytes())
	add("sce_sys/livearea/contents/template.xml", []byte(`<livearea><livearea-background><image>back.png</image></livearea-background><gate><startup-image>gate.png</startup-image></gate></livearea>`))
	z.Close()
	f.Close()

	icon, bg := filepath.Join(dir, "cover.png"), filepath.Join(dir, "hero.png")
	writePNG(t, icon, 600, 900, color.NRGBA{B: 200})
	writePNG(t, bg, 1920, 620, color.NRGBA{B: 40})
	out := filepath.Join(dir, "out.vpk")
	if err := Build(vpk, out, Choice{Icon: icon, Background: bg}); err != nil {
		t.Fatal(err)
	}
	zr, err := zip.OpenReader(out)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	want := map[string][2]int{"sce_sys/icon0.png": {128, 128}, "sce_sys/livearea/contents/back.png": {840, 500}, "sce_sys/livearea/contents/gate.png": {280, 158}}
	seen := 0
	for _, f := range zr.File {
		sz, ok := want[f.Name]
		if !ok {
			continue
		}
		rc, _ := f.Open()
		var b bytes.Buffer
		b.ReadFrom(rc)
		rc.Close()
		raw := b.Bytes()
		if raw[24] != 8 || raw[25] != 3 {
			t.Errorf("%s is not an 8-bit indexed PNG (depth %d, type %d)", f.Name, raw[24], raw[25])
		}
		img, err := png.Decode(bytes.NewReader(raw))
		if err != nil || img.Bounds().Dx() != sz[0] || img.Bounds().Dy() != sz[1] {
			t.Errorf("%s: %v %v", f.Name, err, img.Bounds())
		}
		seen++
	}
	if seen != 3 || len(zr.File) != 6 {
		t.Fatalf("replaced %d, files %d", seen, len(zr.File))
	}
}
