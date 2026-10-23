package galaxy

import (
	"errors"
	"io"
	"os"
)

// CookTHP converts a big-endian THP movie to the little-endian layout the
// port reads (dst may be src).
func CookTHP(src, dst string) error { return cookTHP(src, dst) }

// cookTHP writes a little-endian copy of a THP movie: the file header, the
// per-frame headers and the audio record headers are swapped; video (JPEG)
// and ADPCM data stay as they are. Movies are hundreds of megabytes, so the
// copy is patched in place.
func cookTHP(src, dst string) error {
	head := make([]byte, 0x30)
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	_, err = io.ReadFull(in, head)
	in.Close()
	if err != nil || string(head[:4]) != "THP\x00" || string(head[4:8]) != "\x00\x01\x10\x00" {
		return errors.New(src + ": not a version 1.1 THP file")
	}
	tmp := dst + ".tmp"
	if err := copyFile(src, tmp); err != nil {
		return err
	}
	f, err := os.OpenFile(tmp, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	st, _ := f.Stat()
	p := &patcher{f: f, size: st.Size()}
	if !safe(func() { swapTHP(p) }) || p.err != nil {
		f.Close()
		os.Remove(tmp)
		return errors.New(src + ": damaged THP file")
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, dst)
}

// patcher swaps words of a file in place.
type patcher struct {
	f    *os.File
	size int64
	err  error
}

func (p *patcher) read(off, n int) []byte {
	if off < 0 || int64(off+n) > p.size {
		panic(oob{off, n, int(p.size)})
	}
	b := make([]byte, n)
	if _, err := p.f.ReadAt(b, int64(off)); err != nil {
		panic(err)
	}
	return b
}

func (p *patcher) write(off int, b []byte) {
	if _, err := p.f.WriteAt(b, int64(off)); err != nil {
		panic(err)
	}
}

// sw32 swaps count words at off and returns their big-endian values.
func (p *patcher) sw32(off, count int) []int {
	b := p.read(off, 4*count)
	out := make([]int, count)
	for i := range out {
		out[i] = int(be.Uint32(b[4*i:]))
	}
	sw32(b, 0, count)
	p.write(off, b)
	return out
}

func (p *patcher) sw16(off, count int) {
	b := p.read(off, 2*count)
	sw16(b, 0, count)
	p.write(off, b)
}

const audioHeaderHalfwords = 16 + 16 + 4

func swapTHP(p *patcher) {
	fields := p.sw32(4, 11)
	numFrames, firstSize, compOff, movieOff := fields[4], fields[5], fields[7], fields[9]
	ncomp := p.sw32(compOff, 1)[0]
	kb := p.read(compOff+4, 16)
	var kinds []int
	for i := 0; i < ncomp && i < 16; i++ {
		kinds = append(kinds, int(kb[i]))
	}
	pos := compOff + 0x14
	tracks := 1
	for _, k := range kinds {
		switch k {
		case 0:
			p.sw32(pos, 3)
			pos += 12
		case 1:
			tracks = p.sw32(pos, 4)[3]
			pos += 16
		default:
			panic(errors.New("unknown THP component"))
		}
	}
	off, size := movieOff, firstSize
	for f := 0; f < numFrames; f++ {
		words := p.sw32(off, 2+ncomp)
		next, compSizes := words[0], words[2:]
		data := off + 8 + 4*ncomp
		for i, k := range kinds {
			cs := compSizes[i]
			if k == 1 {
				for t := 0; t < tracks; t++ {
					rec := data + cs*t
					p.sw32(rec, 2)
					p.sw16(rec+8, audioHeaderHalfwords)
				}
			}
			data += cs
		}
		off += size
		size = next
	}
}

func writeFile(dst string, data []byte) error {
	tmp := dst + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, dst)
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.CopyBuffer(out, in, make([]byte, 1<<20)); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
