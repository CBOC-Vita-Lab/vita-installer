package install

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/CBOC-Vita-Lab/vita-installer/internal/device"
	"github.com/CBOC-Vita-Lab/vita-installer/internal/fakevita"
)

// The player's artwork goes into the installed app, and changing it counts
// as a different build.
func TestInstallWithOwnArtwork(t *testing.T) {
	rel, dir := makeRelease(t, false)
	pic := filepath.Join(dir, "cover.png")
	img := image.NewNRGBA(image.Rect(0, 0, 300, 400))
	for i := range img.Pix {
		img.Pix[i] = uint8(i)
	}
	img.Set(0, 0, color.NRGBA{255, 0, 0, 255})
	f, _ := os.Create(pic)
	png.Encode(f, img)
	f.Close()

	vroot := filepath.Join(dir, "vita")
	v, err := fakevita.Start(vroot, true, false)
	if err != nil {
		t.Fatal(err)
	}
	defer v.Close()
	vita := &device.Vita{Host: "127.0.0.1", Port: v.Port()}
	r := load(t, rel)
	for _, in := range r.Inputs {
		in.State = "skip"
	}
	before := buildHash(r)
	r.Art.Icon = pic
	if buildHash(r) == before {
		t.Error("artwork does not change the build fingerprint")
	}
	if _, err := Run(context.Background(), r, Options{Vita: vita, Cache: t.TempDir()}); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(vroot, "ux0/app/MYGM00001/sce_sys/icon0.png"))
	if err != nil {
		t.Fatal(err)
	}
	got, err := png.Decode(bytes.NewReader(b))
	if err != nil || got.Bounds().Dx() != 128 || got.Bounds().Dy() != 128 || b[24] != 8 || b[25] != 3 {
		t.Fatalf("installed icon is not the 128x128 8-bit picture: %v %v", err, got)
	}
	p, err := Check(context.Background(), vita, "", r)
	if err != nil || !p.Same {
		t.Fatalf("the installed build with artwork is not recognised: %v %+v", err, p)
	}
}
