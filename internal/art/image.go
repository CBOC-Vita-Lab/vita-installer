// Package art turns any picture into the Vita's bubble and LiveArea images
// and puts them into a VPK.
package art

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/draw"
	_ "image/gif" // decoders
	_ "image/jpeg"
	"image/png"
	"io"
	"math"
	"os"
	"sort"
)

// Slot sizes, as the Vita shows them.
const (
	IconW, IconH       = 128, 128 // sce_sys/icon0.png
	BGW, BGH           = 840, 500 // LiveArea background
	StartupW, StartupH = 280, 158 // LiveArea "gate" (start button)
	Pic0W, Pic0H       = 960, 544 // sce_sys/pic0.png
)

// Decode reads a PNG, JPEG or GIF.
func Decode(r io.Reader) (image.Image, error) {
	img, _, err := image.Decode(io.LimitReader(r, 64<<20))
	if err != nil {
		return nil, errors.New("this picture can't be read (use PNG or JPEG)")
	}
	b := img.Bounds()
	if b.Dx() < 16 || b.Dy() < 16 || b.Dx() > 12000 || b.Dy() > 12000 {
		return nil, errors.New("this picture is too small or too big")
	}
	return img, nil
}

// Load reads a picture file.
func Load(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return Decode(f)
}

// Cover scales img to fill w×h exactly, cropping what doesn't fit (centred).
func Cover(img image.Image, w, h int) *image.NRGBA {
	src := toNRGBA(img)
	sb := src.Bounds()
	sw, sh := float64(sb.Dx()), float64(sb.Dy())
	scale := math.Max(float64(w)/sw, float64(h)/sh)
	cw, ch := float64(w)/scale, float64(h)/scale // source area that is used
	x0, y0 := (sw-cw)/2, (sh-ch)/2
	return resample(src, x0, y0, cw, ch, w, h)
}

// Contain scales img to fit inside w×h, keeping all of it.
func Contain(img image.Image, w, h int) *image.NRGBA {
	src := toNRGBA(img)
	sb := src.Bounds()
	scale := math.Min(float64(w)/float64(sb.Dx()), float64(h)/float64(sb.Dy()))
	nw, nh := int(math.Max(1, math.Round(float64(sb.Dx())*scale))), int(math.Max(1, math.Round(float64(sb.Dy())*scale)))
	return resample(src, 0, 0, float64(sb.Dx()), float64(sb.Dy()), nw, nh)
}

func toNRGBA(img image.Image) *image.NRGBA {
	if n, ok := img.(*image.NRGBA); ok && n.Bounds().Min == (image.Point{}) {
		return n
	}
	b := img.Bounds()
	out := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(out, out.Bounds(), img, b.Min, draw.Src)
	return out
}

// resample maps the source rectangle (x0,y0,cw,ch) onto a w×h image: area
// averaging when shrinking, bilinear when enlarging. Premultiplied alpha.
func resample(src *image.NRGBA, x0, y0, cw, ch float64, w, h int) *image.NRGBA {
	out := image.NewNRGBA(image.Rect(0, 0, w, h))
	sw, sh := src.Bounds().Dx(), src.Bounds().Dy()
	fx, fy := cw/float64(w), ch/float64(h)
	px := func(x, y int) (r, g, b, a float64) {
		if x < 0 {
			x = 0
		}
		if y < 0 {
			y = 0
		}
		if x >= sw {
			x = sw - 1
		}
		if y >= sh {
			y = sh - 1
		}
		i := src.PixOffset(x, y)
		p := src.Pix[i : i+4]
		a = float64(p[3]) / 255
		return float64(p[0]) * a, float64(p[1]) * a, float64(p[2]) * a, a
	}
	for dy := 0; dy < h; dy++ {
		for dx := 0; dx < w; dx++ {
			var r, g, b, a, n float64
			if fx > 1 || fy > 1 {
				sx0, sy0 := x0+float64(dx)*fx, y0+float64(dy)*fy
				ix0, iy0 := int(sx0), int(sy0)
				ix1, iy1 := int(math.Ceil(sx0+fx)), int(math.Ceil(sy0+fy))
				for y := iy0; y < iy1; y++ {
					for x := ix0; x < ix1; x++ {
						pr, pg, pb, pa := px(x, y)
						r, g, b, a, n = r+pr, g+pg, b+pb, a+pa, n+1
					}
				}
			} else {
				sx, sy := x0+(float64(dx)+0.5)*fx-0.5, y0+(float64(dy)+0.5)*fy-0.5
				ix, iy := int(math.Floor(sx)), int(math.Floor(sy))
				tx, ty := sx-float64(ix), sy-float64(iy)
				for _, c := range [4][3]float64{{0, 0, (1 - tx) * (1 - ty)}, {1, 0, tx * (1 - ty)}, {0, 1, (1 - tx) * ty}, {1, 1, tx * ty}} {
					pr, pg, pb, pa := px(ix+int(c[0]), iy+int(c[1]))
					r, g, b, a, n = r+pr*c[2], g+pg*c[2], b+pb*c[2], a+pa*c[2], n+c[2]
				}
			}
			if n == 0 || a == 0 {
				continue
			}
			i := out.PixOffset(dx, dy)
			out.Pix[i] = clamp(r / a)
			out.Pix[i+1] = clamp(g / a)
			out.Pix[i+2] = clamp(b / a)
			out.Pix[i+3] = clamp(a / n * 255)
		}
	}
	return out
}

func clamp(v float64) uint8 {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return uint8(v + 0.5)
}

// Flatten puts img on an opaque background colour.
func Flatten(img *image.NRGBA, bg color.Color) *image.NRGBA {
	out := image.NewNRGBA(img.Bounds())
	draw.Draw(out, out.Bounds(), image.NewUniform(bg), image.Point{}, draw.Src)
	draw.Draw(out, out.Bounds(), img, img.Bounds().Min, draw.Over)
	return out
}

// Startup makes the LiveArea start image: the background, darkened a little,
// with the logo centred on it when there is one.
func Startup(bg, logo image.Image) *image.NRGBA {
	var base *image.NRGBA
	if bg != nil {
		base = Flatten(Cover(bg, StartupW, StartupH), color.Black)
	} else {
		base = image.NewNRGBA(image.Rect(0, 0, StartupW, StartupH))
		draw.Draw(base, base.Bounds(), image.NewUniform(color.NRGBA{20, 24, 28, 255}), image.Point{}, draw.Src)
	}
	if logo != nil {
		shade := image.NewUniform(color.NRGBA{0, 0, 0, 90})
		draw.Draw(base, base.Bounds(), shade, image.Point{}, draw.Over)
		l := Contain(logo, StartupW*84/100, StartupH*74/100)
		off := image.Pt((StartupW-l.Bounds().Dx())/2, (StartupH-l.Bounds().Dy())/2)
		draw.Draw(base, l.Bounds().Add(off), l, image.Point{}, draw.Over)
	}
	return base
}

// EncodeVita writes img as an 8-bit indexed PNG, the format the Vita's
// installer accepts for bubble and LiveArea images (fewer bits are refused).
func EncodeVita(img image.Image) ([]byte, error) {
	src := Flatten(toNRGBA(img), color.Black)
	pal := medianCut(src, 256)
	for len(pal) < 256 { // a full palette keeps the PNG at 8 bits per pixel
		pal = append(pal, color.RGBA{0, 0, 0, 255})
	}
	dst := image.NewPaletted(src.Bounds(), pal)
	draw.FloydSteinberg.Draw(dst, dst.Bounds(), src, image.Point{})
	var buf bytes.Buffer
	enc := png.Encoder{CompressionLevel: png.BestCompression}
	if err := enc.Encode(&buf, dst); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// EncodePreview writes img as an ordinary PNG for the page.
func EncodePreview(img image.Image) []byte {
	var buf bytes.Buffer
	png.Encode(&buf, img)
	return buf.Bytes()
}

// medianCut picks up to n colours that represent img well.
func medianCut(img *image.NRGBA, n int) color.Palette {
	type px [3]uint8
	var pts []px
	b := img.Bounds()
	step := 1
	if b.Dx()*b.Dy() > 120000 {
		step = int(math.Sqrt(float64(b.Dx()*b.Dy()) / 120000))
	}
	for y := b.Min.Y; y < b.Max.Y; y += step {
		for x := b.Min.X; x < b.Max.X; x += step {
			i := img.PixOffset(x, y)
			pts = append(pts, px{img.Pix[i], img.Pix[i+1], img.Pix[i+2]})
		}
	}
	type box struct{ p []px }
	rng := func(bx box) (int, int) {
		lo, hi := [3]int{255, 255, 255}, [3]int{}
		for _, p := range bx.p {
			for c := 0; c < 3; c++ {
				if int(p[c]) < lo[c] {
					lo[c] = int(p[c])
				}
				if int(p[c]) > hi[c] {
					hi[c] = int(p[c])
				}
			}
		}
		best, ch := -1, 0
		for c := 0; c < 3; c++ {
			if hi[c]-lo[c] > best {
				best, ch = hi[c]-lo[c], c
			}
		}
		return best, ch
	}
	boxes := []box{{pts}}
	for len(boxes) < n {
		// split the box with the widest colour range (weighted by size)
		bi, bw, bc := -1, -1, 0
		for i, bx := range boxes {
			if len(bx.p) < 2 {
				continue
			}
			r, c := rng(bx)
			if w := r * int(math.Sqrt(float64(len(bx.p)))); r > 0 && w > bw {
				bi, bw, bc = i, w, c
			}
		}
		if bi < 0 {
			break
		}
		p := boxes[bi].p
		sort.Slice(p, func(i, j int) bool { return p[i][bc] < p[j][bc] })
		mid := len(p) / 2
		boxes[bi] = box{p[:mid]}
		boxes = append(boxes, box{p[mid:]})
	}
	pal := make(color.Palette, 0, n)
	for _, bx := range boxes {
		var s [3]int
		for _, p := range bx.p {
			s[0], s[1], s[2] = s[0]+int(p[0]), s[1]+int(p[1]), s[2]+int(p[2])
		}
		if k := len(bx.p); k > 0 {
			pal = append(pal, color.RGBA{uint8(s[0] / k), uint8(s[1] / k), uint8(s[2] / k), 255})
		}
	}
	return pal
}
