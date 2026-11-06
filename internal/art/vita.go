package art

import (
	"bytes"
	"errors"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
)

// Pictures from the player's own Vita (its screenshots, and title screens a
// game captured of itself) are copied into VitaDir and offered as candidates
// with a file: link. Only files in that folder are accepted as links.
func VitaDir(cache string) string { return filepath.Join(cache, "art", "vita") }

// localLink returns the file behind a file: link when it is a copied Vita
// picture.
func localLink(cache, raw string) (string, error) {
	p := filepath.Clean(filepath.FromSlash(strings.TrimPrefix(raw, "file:")))
	dir := filepath.Clean(VitaDir(cache)) + string(filepath.Separator)
	if !strings.HasPrefix(p, dir) {
		return "", errors.New("not a picture link")
	}
	if _, err := os.Stat(p); err != nil {
		return "", errors.New("that picture is no longer here; look on your Vita again")
	}
	return p, nil
}

// LinkFor is the link a copied Vita picture is offered with.
func LinkFor(path string) string { return "file:" + filepath.ToSlash(path) }

// RawRGBA turns a raw w×h RGBA capture (as games write them) into a PNG.
func RawRGBA(b []byte, w, h int) ([]byte, error) {
	if len(b) != w*h*4 {
		return nil, errors.New("the capture is not the expected size")
	}
	img := &image.NRGBA{Pix: append([]byte(nil), b...), Stride: w * 4, Rect: image.Rect(0, 0, w, h)}
	for i := 3; i < len(img.Pix); i += 4 {
		img.Pix[i] = 0xff // the frame is opaque; its alpha channel is not meaningful
	}
	var out bytes.Buffer
	if err := png.Encode(&out, img); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}
