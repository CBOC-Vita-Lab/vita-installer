package art

import (
	"archive/zip"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"image"
	"io"
	"os"
	"regexp"
	"strings"
)

// Choice is the player's artwork for one game: picture files on this
// computer for each slot (empty = keep the game's own).
type Choice struct {
	Icon       string `json:"icon,omitempty"`       // bubble icon
	Background string `json:"background,omitempty"` // LiveArea background (and pic0)
	Logo       string `json:"logo,omitempty"`       // drawn on the LiveArea start image
}

// Empty reports whether nothing is replaced.
func (c Choice) Empty() bool { return c.Icon == "" && c.Background == "" && c.Logo == "" }

// Fingerprint identifies the pictures, so a changed choice counts as a
// different build.
func (c Choice) Fingerprint() string {
	if c.Empty() {
		return ""
	}
	h := sha1.New()
	for _, p := range []string{c.Icon, c.Background, c.Logo} {
		if p == "" {
			h.Write([]byte{0})
			continue
		}
		f, err := os.Open(p)
		if err == nil {
			io.Copy(h, f)
			f.Close()
		}
		h.Write([]byte{1})
	}
	return hex.EncodeToString(h.Sum(nil))[:12]
}

// Paths of the images inside a VPK.
type Paths struct {
	Icon, Background, Startup, Pic0 string // "" when the VPK has none
}

var (
	reBG      = regexp.MustCompile(`(?s)<livearea-background>.*?<image>([^<]+)</image>`)
	reStartup = regexp.MustCompile(`<startup-image>([^<]+)</startup-image>`)
)

// Find reads which images the VPK uses.
func Find(z *zip.Reader) Paths {
	have := map[string]bool{}
	var tmpl []byte
	for _, f := range z.File {
		have[f.Name] = true
		if f.Name == "sce_sys/livearea/contents/template.xml" {
			if rc, err := f.Open(); err == nil {
				tmpl, _ = io.ReadAll(io.LimitReader(rc, 1<<20))
				rc.Close()
			}
		}
	}
	p := Paths{Icon: "sce_sys/icon0.png"}
	if have["sce_sys/pic0.png"] {
		p.Pic0 = "sce_sys/pic0.png"
	}
	bg, st := "bg.png", "startup.png"
	if m := reBG.FindSubmatch(tmpl); m != nil {
		bg = strings.TrimSpace(string(m[1]))
	}
	if m := reStartup.FindSubmatch(tmpl); m != nil {
		st = strings.TrimSpace(string(m[1]))
	}
	if n := "sce_sys/livearea/contents/" + bg; have[n] {
		p.Background = n
	}
	if n := "sce_sys/livearea/contents/" + st; have[n] {
		p.Startup = n
	}
	return p
}

// Original returns a picture from the VPK (nil if it is not there).
func Original(z *zip.Reader, name string) image.Image {
	for _, f := range z.File {
		if f.Name == name {
			rc, err := f.Open()
			if err != nil {
				return nil
			}
			defer rc.Close()
			img, err := Decode(rc)
			if err != nil {
				return nil
			}
			return img
		}
	}
	return nil
}

// Rendered is what goes on the Vita for each slot (only replaced ones).
type Rendered struct {
	Icon, Background, Startup, Pic0 image.Image
}

// Render makes the final pictures for a choice. Slots the choice doesn't
// touch stay nil; the start image follows a new background or logo.
func Render(c Choice, z *zip.Reader) (*Rendered, error) {
	r := &Rendered{}
	load := func(p string) (image.Image, error) {
		if p == "" {
			return nil, nil
		}
		return Load(p)
	}
	icon, err := load(c.Icon)
	if err != nil {
		return nil, err
	}
	bg, err := load(c.Background)
	if err != nil {
		return nil, err
	}
	logo, err := load(c.Logo)
	if err != nil {
		return nil, err
	}
	if icon != nil {
		r.Icon = Cover(icon, IconW, IconH)
	}
	if bg != nil {
		r.Background = Cover(bg, BGW, BGH)
		r.Pic0 = Cover(bg, Pic0W, Pic0H)
	}
	if bg != nil || logo != nil {
		base := bg
		if base == nil && z != nil { // a logo on the game's own background
			base = Original(z, Find(z).Background)
		}
		r.Startup = Startup(base, logo)
	}
	return r, nil
}

// Build writes a copy of the VPK at src with the chosen pictures to dst.
func Build(src, dst string, c Choice) error {
	zr, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer zr.Close()
	paths := Find(&zr.Reader)
	r, err := Render(c, &zr.Reader)
	if err != nil {
		return err
	}
	repl := map[string][]byte{}
	put := func(name string, img image.Image) error {
		if name == "" || img == nil {
			return nil
		}
		b, err := EncodeVita(img)
		if err != nil {
			return err
		}
		repl[name] = b
		return nil
	}
	for _, e := range []struct {
		name string
		img  image.Image
	}{{paths.Icon, r.Icon}, {paths.Background, r.Background}, {paths.Startup, r.Startup}, {paths.Pic0, r.Pic0}} {
		if err := put(e.name, e.img); err != nil {
			return err
		}
	}
	if len(repl) == 0 {
		return errors.New("this game has no pictures that can be replaced")
	}
	out, err := os.Create(dst + ".part")
	if err != nil {
		return err
	}
	zw := zip.NewWriter(out)
	for _, f := range zr.File {
		if b, ok := repl[f.Name]; ok {
			w, err := zw.CreateHeader(&zip.FileHeader{Name: f.Name, Method: zip.Deflate, Modified: f.Modified})
			if err == nil {
				_, err = w.Write(b)
			}
			if err != nil {
				out.Close()
				return err
			}
			continue
		}
		raw, err := f.OpenRaw()
		if err == nil {
			var w io.Writer
			h := f.FileHeader
			if w, err = zw.CreateRaw(&h); err == nil {
				_, err = io.Copy(w, raw)
			}
		}
		if err != nil {
			out.Close()
			return err
		}
	}
	if err := zw.Close(); err != nil {
		out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	return os.Rename(dst+".part", dst)
}
