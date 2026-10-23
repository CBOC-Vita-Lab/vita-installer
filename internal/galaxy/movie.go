package galaxy

// Re-encodes the video of a cooked (little-endian) THP movie at a lower
// quality, keeping the container, frame rate, audio and picture size. The
// game's movies are ~40 KB of JPEG per frame at 59.94 frames a second; the
// Vita spends most of its decode time on the entropy-coded data, so a
// smaller stream plays at full speed and the files shrink a lot.
//
// Frames are decoded, then re-encoded as baseline 4:2:0 JPEG with per-frame
// optimal Huffman tables and written the way THP stores them (no 0xFF00
// byte stuffing in the entropy-coded data), as the port's own
// tools/thp_transcode.cpp does.

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"os"
	"runtime"
	"sync"
)

var le = binary.LittleEndian

type block [64]int32

// MovieStats summarises a re-encode.
type MovieStats struct {
	Frames            int
	InBytes, OutBytes int64
}

// unzig maps zig-zag order to natural order.
var unzig = [64]int{
	0, 1, 8, 16, 9, 2, 3, 10, 17, 24, 32, 25, 18, 11, 4, 5,
	12, 19, 26, 33, 40, 48, 41, 34, 27, 20, 13, 6, 7, 14, 21, 28,
	35, 42, 49, 56, 57, 50, 43, 36, 29, 22, 15, 23, 30, 37, 44, 51,
	58, 59, 52, 45, 38, 31, 39, 46, 53, 60, 61, 54, 47, 55, 62, 63,
}

// baseQuant are the standard (Annex K) tables in zig-zag order.
var baseQuant = [2][64]int{
	{16, 11, 12, 14, 12, 10, 16, 14, 13, 14, 18, 17, 16, 19, 24, 40, 26, 24, 22, 22, 24, 49, 35, 37, 29, 40, 58, 51, 61, 60, 57, 51,
		56, 55, 64, 72, 92, 78, 64, 68, 87, 69, 55, 56, 80, 109, 81, 87, 95, 98, 103, 104, 103, 62, 77, 113, 121, 112, 100, 120, 92, 101, 103, 99},
	{17, 18, 18, 24, 21, 24, 47, 26, 26, 47, 99, 66, 56, 66, 99, 99, 99, 99, 99, 99, 99, 99, 99, 99, 99, 99, 99, 99, 99, 99, 99, 99,
		99, 99, 99, 99, 99, 99, 99, 99, 99, 99, 99, 99, 99, 99, 99, 99, 99, 99, 99, 99, 99, 99, 99, 99, 99, 99, 99, 99, 99, 99, 99, 99},
}

func quantTables(quality int) [2][64]int32 {
	if quality < 1 {
		quality = 1
	}
	if quality > 100 {
		quality = 100
	}
	scale := 200 - quality*2
	if quality < 50 {
		scale = 5000 / quality
	}
	var q [2][64]int32
	for t := 0; t < 2; t++ {
		for i := 0; i < 64; i++ {
			v := (baseQuant[t][i]*scale + 50) / 100
			if v < 1 {
				v = 1
			} else if v > 255 {
				v = 255
			}
			q[t][i] = int32(v)
		}
	}
	return q
}

func qdiv(a, b int32) int32 {
	if a >= 0 {
		return (a + (b >> 1)) / b
	}
	return -((-a + (b >> 1)) / b)
}

// planes is a 4:2:0 picture.
type planes struct {
	w, h          int
	y, cb, cr     []byte
	yStride, cStr int
}

func bitCount(v int32) int {
	if v < 0 {
		v = -v
	}
	n := 0
	for v > 0 {
		n++
		v >>= 1
	}
	return n
}

// encodeTHPFrame encodes a 4:2:0 picture as THP-style JPEG.
func encodeTHPFrame(p planes, q *[2][64]int32) []byte {
	mw, mh := (p.w+15)/16, (p.h+15)/16
	nblocks := mw * mh * 6
	coeffs := make([][64]int32, nblocks) // zig-zag order, quantized
	var src block
	sample := func(plane []byte, stride, pw, ph, x, y int) int32 {
		if x >= pw {
			x = pw - 1
		}
		if y >= ph {
			y = ph - 1
		}
		return int32(plane[y*stride+x])
	}
	cw, ch := (p.w+1)/2, (p.h+1)/2
	n := 0
	put := func(t int) {
		fdct(&src)
		for zi := 0; zi < 64; zi++ {
			coeffs[n][zi] = qdiv(src[unzig[zi]], 8*q[t][zi])
		}
		n++
	}
	for my := 0; my < mh; my++ {
		for mx := 0; mx < mw; mx++ {
			for by := 0; by < 2; by++ {
				for bx := 0; bx < 2; bx++ {
					x0, y0 := mx*16+bx*8, my*16+by*8
					for j := 0; j < 8; j++ {
						for i := 0; i < 8; i++ {
							src[j*8+i] = sample(p.y, p.yStride, p.w, p.h, x0+i, y0+j)
						}
					}
					put(0)
				}
			}
			for _, plane := range [][]byte{p.cb, p.cr} {
				x0, y0 := mx*8, my*8
				for j := 0; j < 8; j++ {
					for i := 0; i < 8; i++ {
						src[j*8+i] = sample(plane, p.cStr, cw, ch, x0+i, y0+j)
					}
				}
				put(1)
			}
		}
	}
	// Symbol frequencies for the optimal tables (DC luma, AC luma, DC chroma, AC chroma).
	var freq [4][257]int64
	tableOf := func(i int) int { // block index -> 0 luma, 1 chroma
		if i%6 < 4 {
			return 0
		}
		return 1
	}
	walk := func(emit func(t, dcac int, sym byte, extra int32, extraBits int)) {
		var prev [3]int32
		for i := 0; i < nblocks; i++ {
			t := tableOf(i)
			comp := 0
			if k := i % 6; k >= 4 {
				comp = k - 3
			}
			c := &coeffs[i]
			d := c[0] - prev[comp]
			prev[comp] = c[0]
			nb := bitCount(d)
			emit(t, 0, byte(nb), d, nb)
			run := 0
			for zi := 1; zi < 64; zi++ {
				v := c[zi]
				if v == 0 {
					run++
					continue
				}
				for run > 15 {
					emit(t, 1, 0xF0, 0, 0)
					run -= 16
				}
				nb := bitCount(v)
				emit(t, 1, byte(run<<4|nb), v, nb)
				run = 0
			}
			if run > 0 {
				emit(t, 1, 0x00, 0, 0)
			}
		}
	}
	walk(func(t, dcac int, sym byte, _ int32, _ int) { freq[t*2+dcac][sym]++ })
	var tables [4]huffTable
	for i := range tables {
		tables[i] = optimalTable(freq[i])
	}
	var out bytes.Buffer
	out.Write([]byte{0xFF, 0xD8})
	for t := 0; t < 2; t++ {
		out.Write([]byte{0xFF, 0xDB, 0, 67, byte(t)})
		for i := 0; i < 64; i++ {
			out.WriteByte(byte(q[t][i]))
		}
	}
	out.Write([]byte{0xFF, 0xC0, 0, 17, 8, byte(p.h >> 8), byte(p.h), byte(p.w >> 8), byte(p.w), 3,
		1, 0x22, 0, 2, 0x11, 1, 3, 0x11, 1})
	for i, cls := range []byte{0x00, 0x10, 0x01, 0x11} {
		tb := tables[i]
		n := 0
		for _, c := range tb.bits[1:] {
			n += int(c)
		}
		l := 2 + 1 + 16 + n
		out.Write([]byte{0xFF, 0xC4, byte(l >> 8), byte(l), cls})
		out.Write(tb.bits[1:17])
		out.Write(tb.vals[:n])
	}
	out.Write([]byte{0xFF, 0xDA, 0, 12, 3, 1, 0x00, 2, 0x11, 3, 0x11, 0, 63, 0})
	var bw bitWriter
	walk(func(t, dcac int, sym byte, extra int32, extraBits int) {
		tb := &tables[t*2+dcac]
		bw.put(uint32(tb.code[sym]), int(tb.size[sym]))
		if extraBits > 0 {
			v := extra
			if v < 0 {
				v += (1 << uint(extraBits)) - 1
			}
			bw.put(uint32(v)&(1<<uint(extraBits)-1), extraBits)
		}
	})
	bw.flush()
	out.Write(bw.buf) // THP keeps the entropy data unstuffed
	out.Write([]byte{0xFF, 0xD9})
	return out.Bytes()
}

type bitWriter struct {
	buf  []byte
	acc  uint32
	bits int
}

func (w *bitWriter) put(v uint32, n int) {
	for n > 0 {
		take := n
		if take > 16 {
			take = 16
		}
		n -= take
		w.acc = w.acc<<uint(take) | (v>>uint(n))&(1<<uint(take)-1)
		w.bits += take
		for w.bits >= 8 {
			w.bits -= 8
			w.buf = append(w.buf, byte(w.acc>>uint(w.bits)))
		}
		w.acc &= 1<<uint(w.bits) - 1
	}
}

func (w *bitWriter) flush() {
	if w.bits > 0 {
		pad := 8 - w.bits
		w.put(1<<uint(pad)-1, pad)
	}
}

type huffTable struct {
	bits [17]byte
	vals [256]byte
	code [256]uint16
	size [256]byte
}

// optimalTable builds a Huffman table from symbol counts the way libjpeg
// does (jpeg_gen_optimal_table, JPEG Annex K.2).
func optimalTable(f [257]int64) huffTable {
	var freq [257]int64
	copy(freq[:], f[:])
	freq[256] = 1 // reserve one code point so no code is all ones
	var codesize [257]int
	var others [257]int
	for i := range others {
		others[i] = -1
	}
	for {
		c1, c2 := -1, -1
		var v int64 = 1 << 62
		for i := 0; i <= 256; i++ {
			if freq[i] != 0 && freq[i] <= v {
				v, c1 = freq[i], i
			}
		}
		v = 1 << 62
		for i := 0; i <= 256; i++ {
			if freq[i] != 0 && freq[i] <= v && i != c1 {
				v, c2 = freq[i], i
			}
		}
		if c2 < 0 {
			break
		}
		freq[c1] += freq[c2]
		freq[c2] = 0
		codesize[c1]++
		for others[c1] >= 0 {
			c1 = others[c1]
			codesize[c1]++
		}
		others[c1] = c2
		codesize[c2]++
		for others[c2] >= 0 {
			c2 = others[c2]
			codesize[c2]++
		}
	}
	var bits [33]int
	for i := 0; i <= 256; i++ {
		if codesize[i] > 0 {
			bits[codesize[i]]++
		}
	}
	for i := 32; i > 16; i-- {
		for bits[i] > 0 {
			j := i - 2
			for bits[j] == 0 {
				j--
			}
			bits[i] -= 2
			bits[i-1]++
			bits[j+1] += 2
			bits[j]--
		}
	}
	i := 16
	for bits[i] == 0 {
		i--
	}
	bits[i]--
	var t huffTable
	for k := 1; k <= 16; k++ {
		t.bits[k] = byte(bits[k])
	}
	p := 0
	for l := 1; l <= 32; l++ {
		for j := 0; j < 256; j++ {
			if codesize[j] == l {
				t.vals[p] = byte(j)
				p++
			}
		}
	}
	code, k := uint16(0), 0
	for l := 1; l <= 16; l++ {
		for n := 0; n < int(t.bits[l]); n++ {
			t.code[t.vals[k]] = code
			t.size[t.vals[k]] = byte(l)
			code++
			k++
		}
		code <<= 1
	}
	return t
}

// Standard DHT segment (Annex K.3), added when a frame has none.
var stdDHT = func() []byte {
	tables := []struct {
		cls  byte
		bits []byte
		vals []byte
	}{
		{0x00, []byte{0, 1, 5, 1, 1, 1, 1, 1, 1, 0, 0, 0, 0, 0, 0, 0}, []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11}},
		{0x10, []byte{0, 2, 1, 3, 3, 2, 4, 3, 5, 5, 4, 4, 0, 0, 1, 0x7d}, []byte{
			0x01, 0x02, 0x03, 0x00, 0x04, 0x11, 0x05, 0x12, 0x21, 0x31, 0x41, 0x06, 0x13, 0x51, 0x61, 0x07,
			0x22, 0x71, 0x14, 0x32, 0x81, 0x91, 0xa1, 0x08, 0x23, 0x42, 0xb1, 0xc1, 0x15, 0x52, 0xd1, 0xf0,
			0x24, 0x33, 0x62, 0x72, 0x82, 0x09, 0x0a, 0x16, 0x17, 0x18, 0x19, 0x1a, 0x25, 0x26, 0x27, 0x28,
			0x29, 0x2a, 0x34, 0x35, 0x36, 0x37, 0x38, 0x39, 0x3a, 0x43, 0x44, 0x45, 0x46, 0x47, 0x48, 0x49,
			0x4a, 0x53, 0x54, 0x55, 0x56, 0x57, 0x58, 0x59, 0x5a, 0x63, 0x64, 0x65, 0x66, 0x67, 0x68, 0x69,
			0x6a, 0x73, 0x74, 0x75, 0x76, 0x77, 0x78, 0x79, 0x7a, 0x83, 0x84, 0x85, 0x86, 0x87, 0x88, 0x89,
			0x8a, 0x92, 0x93, 0x94, 0x95, 0x96, 0x97, 0x98, 0x99, 0x9a, 0xa2, 0xa3, 0xa4, 0xa5, 0xa6, 0xa7,
			0xa8, 0xa9, 0xaa, 0xb2, 0xb3, 0xb4, 0xb5, 0xb6, 0xb7, 0xb8, 0xb9, 0xba, 0xc2, 0xc3, 0xc4, 0xc5,
			0xc6, 0xc7, 0xc8, 0xc9, 0xca, 0xd2, 0xd3, 0xd4, 0xd5, 0xd6, 0xd7, 0xd8, 0xd9, 0xda, 0xe1, 0xe2,
			0xe3, 0xe4, 0xe5, 0xe6, 0xe7, 0xe8, 0xe9, 0xea, 0xf1, 0xf2, 0xf3, 0xf4, 0xf5, 0xf6, 0xf7, 0xf8,
			0xf9, 0xfa}},
		{0x01, []byte{0, 3, 1, 1, 1, 1, 1, 1, 1, 1, 1, 0, 0, 0, 0, 0}, []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11}},
		{0x11, []byte{0, 2, 1, 2, 4, 4, 3, 4, 7, 5, 4, 4, 0, 1, 2, 0x77}, []byte{
			0x00, 0x01, 0x02, 0x03, 0x11, 0x04, 0x05, 0x21, 0x31, 0x06, 0x12, 0x41, 0x51, 0x07, 0x61, 0x71,
			0x13, 0x22, 0x32, 0x81, 0x08, 0x14, 0x42, 0x91, 0xa1, 0xb1, 0xc1, 0x09, 0x23, 0x33, 0x52, 0xf0,
			0x15, 0x62, 0x72, 0xd1, 0x0a, 0x16, 0x24, 0x34, 0xe1, 0x25, 0xf1, 0x17, 0x18, 0x19, 0x1a, 0x26,
			0x27, 0x28, 0x29, 0x2a, 0x35, 0x36, 0x37, 0x38, 0x39, 0x3a, 0x43, 0x44, 0x45, 0x46, 0x47, 0x48,
			0x49, 0x4a, 0x53, 0x54, 0x55, 0x56, 0x57, 0x58, 0x59, 0x5a, 0x63, 0x64, 0x65, 0x66, 0x67, 0x68,
			0x69, 0x6a, 0x73, 0x74, 0x75, 0x76, 0x77, 0x78, 0x79, 0x7a, 0x82, 0x83, 0x84, 0x85, 0x86, 0x87,
			0x88, 0x89, 0x8a, 0x92, 0x93, 0x94, 0x95, 0x96, 0x97, 0x98, 0x99, 0x9a, 0xa2, 0xa3, 0xa4, 0xa5,
			0xa6, 0xa7, 0xa8, 0xa9, 0xaa, 0xb2, 0xb3, 0xb4, 0xb5, 0xb6, 0xb7, 0xb8, 0xb9, 0xba, 0xc2, 0xc3,
			0xc4, 0xc5, 0xc6, 0xc7, 0xc8, 0xc9, 0xca, 0xd2, 0xd3, 0xd4, 0xd5, 0xd6, 0xd7, 0xd8, 0xd9, 0xda,
			0xe2, 0xe3, 0xe4, 0xe5, 0xe6, 0xe7, 0xe8, 0xe9, 0xea, 0xf2, 0xf3, 0xf4, 0xf5, 0xf6, 0xf7, 0xf8,
			0xf9, 0xfa}},
	}
	var b []byte
	for _, t := range tables {
		l := 2 + 1 + 16 + len(t.vals)
		b = append(b, 0xFF, 0xC4, byte(l>>8), byte(l), t.cls)
		b = append(b, t.bits...)
		b = append(b, t.vals...)
	}
	return b
}()

// decodeTHPFrame decodes one THP-style JPEG (entropy data without byte
// stuffing) into 4:2:0 planes.
func decodeTHPFrame(data []byte) (planes, error) {
	sos := -1
	hasDHT := false
	i := 2
	if len(data) < 4 || data[0] != 0xFF || data[1] != 0xD8 {
		return planes{}, errors.New("frame is not JPEG")
	}
	for i+4 <= len(data) {
		if data[i] != 0xFF {
			return planes{}, errors.New("bad JPEG marker")
		}
		m := data[i+1]
		l := int(data[i+2])<<8 | int(data[i+3])
		if m == 0xC4 {
			hasDHT = true
		}
		if m == 0xDA {
			sos = i
			break
		}
		i += 2 + l
	}
	if sos < 0 || sos+4 > len(data) {
		return planes{}, errors.New("frame has no image data")
	}
	scan := sos + 2 + (int(data[sos+2])<<8 | int(data[sos+3]))
	end := bytes.LastIndex(data, []byte{0xFF, 0xD9})
	if end < scan {
		end = len(data)
	}
	var j bytes.Buffer
	j.Grow(len(data) + len(data)/64 + 512)
	j.Write(data[:2])
	if !hasDHT {
		j.Write(stdDHT)
	}
	j.Write(data[2:scan])
	for _, c := range data[scan:end] {
		j.WriteByte(c)
		if c == 0xFF {
			j.WriteByte(0)
		}
	}
	j.Write([]byte{0xFF, 0xD9})
	img, err := jpeg.Decode(&j)
	if err != nil {
		return planes{}, err
	}
	yc, ok := img.(*image.YCbCr)
	if !ok || yc.SubsampleRatio != image.YCbCrSubsampleRatio420 {
		return planes{}, errors.New("movie frames are not 4:2:0 JPEG")
	}
	b := yc.Rect
	return planes{w: b.Dx(), h: b.Dy(), y: yc.Y[yc.YOffset(b.Min.X, b.Min.Y):], cb: yc.Cb[yc.COffset(b.Min.X, b.Min.Y):],
		cr: yc.Cr[yc.COffset(b.Min.X, b.Min.Y):], yStride: yc.YStride, cStr: yc.CStride}, nil
}

// ReencodeTHP writes a smaller copy of a cooked (little-endian) THP movie.
func ReencodeTHP(src, dst string, quality, workers int, progress func(done, total int)) (MovieStats, error) {
	var st MovieStats
	in, err := os.Open(src)
	if err != nil {
		return st, err
	}
	defer in.Close()
	head := make([]byte, 0x60)
	if _, err := io.ReadFull(in, head); err != nil || string(head[:4]) != "THP\x00" {
		return st, errors.New("not a THP movie")
	}
	numFrames, firstSize := int(le.Uint32(head[0x14:])), int(le.Uint32(head[0x18:]))
	compOff, movieOff := int(le.Uint32(head[0x20:])), int(le.Uint32(head[0x28:]))
	if compOff+0x14 > len(head) {
		return st, errors.New("unexpected THP header")
	}
	ncomp := int(le.Uint32(head[compOff:]))
	if ncomp < 1 || ncomp > 2 || head[compOff+4] != 0 || (ncomp == 2 && head[compOff+5] != 1) {
		return st, errors.New("unsupported THP layout")
	}
	hdrSize := 8 + 4*ncomp
	q := quantTables(quality)
	if workers <= 0 {
		workers = runtime.NumCPU()
	}
	type frame struct {
		index int
		video []byte
		audio []byte
		out   []byte
		err   error
	}
	jobs := make(chan *frame, workers*2)
	results := make(chan *frame, workers*2)
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for fr := range jobs {
				p, err := decodeTHPFrame(fr.video)
				if err != nil {
					fr.err = fmt.Errorf("frame %d: %v", fr.index, err)
				} else {
					fr.out = encodeTHPFrame(p, &q)
					for len(fr.out)%4 != 0 {
						fr.out = append(fr.out, 0)
					}
				}
				results <- fr
			}
		}()
	}
	readErr := make(chan error, 1)
	go func() {
		defer close(jobs)
		pos, size := int64(movieOff), firstSize
		for i := 0; i < numFrames; i++ {
			if size < hdrSize || size > 16<<20 {
				readErr <- fmt.Errorf("frame %d has a bad size", i)
				return
			}
			b := make([]byte, size)
			if _, err := in.ReadAt(b, pos); err != nil {
				readErr <- fmt.Errorf("frame %d: %v", i, err)
				return
			}
			next := int(le.Uint32(b[0:]))
			vsz := int(le.Uint32(b[8:]))
			asz := 0
			if ncomp == 2 {
				asz = int(le.Uint32(b[12:]))
			}
			if hdrSize+vsz+asz > size {
				readErr <- fmt.Errorf("frame %d is damaged", i)
				return
			}
			st.InBytes += int64(size)
			jobs <- &frame{index: i, video: b[hdrSize : hdrSize+vsz], audio: b[hdrSize+vsz : hdrSize+vsz+asz]}
			pos += int64(size)
			size = next
		}
		readErr <- nil
	}()
	go func() { wg.Wait(); close(results) }()

	tmp := dst + ".tmp"
	out, err := os.Create(tmp)
	if err != nil {
		return st, err
	}
	fail := func(e error) (MovieStats, error) {
		out.Close()
		os.Remove(tmp)
		for range results {
		}
		return st, e
	}
	if _, err := out.Write(head); err != nil {
		return fail(err)
	}
	pending := map[int]*frame{}
	next := 0
	var prevSize, firstOut, maxOut int
	var lastFrame []byte
	written := int64(len(head))
	var lastOffset int64
	emit := func(fr *frame) error {
		size := (hdrSize + len(fr.out) + len(fr.audio) + 31) &^ 31
		b := make([]byte, size)
		le.PutUint32(b[4:], uint32(prevSize))
		le.PutUint32(b[8:], uint32(len(fr.out)))
		if ncomp == 2 {
			le.PutUint32(b[12:], uint32(len(fr.audio)))
		}
		copy(b[hdrSize:], fr.out)
		copy(b[hdrSize+len(fr.out):], fr.audio)
		if lastFrame != nil {
			le.PutUint32(lastFrame[0:], uint32(size))
			if _, err := out.Write(lastFrame); err != nil {
				return err
			}
			written += int64(len(lastFrame))
		} else {
			firstOut = size
		}
		lastFrame = b
		prevSize = size
		if size > maxOut {
			maxOut = size
		}
		st.OutBytes += int64(size)
		st.Frames++
		return nil
	}
	for fr := range results {
		if fr.err != nil {
			return fail(fr.err)
		}
		pending[fr.index] = fr
		for {
			f, ok := pending[next]
			if !ok {
				break
			}
			delete(pending, next)
			if err := emit(f); err != nil {
				return fail(err)
			}
			next++
			if progress != nil && next%60 == 0 {
				progress(next, numFrames)
			}
		}
	}
	if err := <-readErr; err != nil {
		return fail(err)
	}
	if lastFrame == nil || st.Frames != numFrames {
		return fail(errors.New("movie has no frames"))
	}
	le.PutUint32(lastFrame[0:], uint32(firstOut)) // THP loops: the last frame points at the first
	lastOffset = written
	if _, err := out.Write(lastFrame); err != nil {
		return fail(err)
	}
	written += int64(len(lastFrame))
	le.PutUint32(head[0x14:], uint32(st.Frames))
	le.PutUint32(head[0x18:], uint32(firstOut))
	le.PutUint32(head[0x1c:], uint32(written))
	le.PutUint32(head[0x2c:], uint32(lastOffset))
	le.PutUint32(head[0x08:], uint32((maxOut+31)&^31))
	if _, err := out.WriteAt(head, 0); err != nil {
		return fail(err)
	}
	var pv [4]byte
	le.PutUint32(pv[:], uint32(prevSize))
	if _, err := out.WriteAt(pv[:], int64(len(head))+4); err != nil {
		return fail(err)
	}
	if err := out.Close(); err != nil {
		os.Remove(tmp)
		return st, err
	}
	if progress != nil {
		progress(numFrames, numFrames)
	}
	return st, os.Rename(tmp, dst)
}
