package galaxy

import (
	"errors"
	"sort"
)

// Yaz0Decompress returns data unpacked if it starts with "Yaz0", else data.
func Yaz0Decompress(data []byte) ([]byte, error) {
	if len(data) < 16 || string(data[:4]) != "Yaz0" {
		return data, nil
	}
	size := int(be.Uint32(data[4:]))
	out := make([]byte, size)
	src, dst := 16, 0
	for dst < size {
		if src >= len(data) {
			return nil, errors.New("bad Yaz0 stream")
		}
		code := data[src]
		src++
		for bit := 0; bit < 8 && dst < size; bit++ {
			if code&(0x80>>bit) != 0 {
				if src >= len(data) {
					return nil, errors.New("bad Yaz0 stream")
				}
				out[dst] = data[src]
				dst++
				src++
				continue
			}
			if src+1 >= len(data) {
				return nil, errors.New("bad Yaz0 stream")
			}
			b1, b2 := int(data[src]), int(data[src+1])
			src += 2
			from := dst - ((b1&0x0F)<<8 | b2) - 1
			n := b1 >> 4
			if n == 0 {
				if src >= len(data) {
					return nil, errors.New("bad Yaz0 stream")
				}
				n = int(data[src]) + 0x12
				src++
			} else {
				n += 2
			}
			if from < 0 {
				return nil, errors.New("bad Yaz0 stream")
			}
			for i := 0; i < n && dst < size; i++ {
				out[dst] = out[from]
				dst++
				from++
			}
		}
	}
	return out, nil
}

// RARC entry flags.
const (
	flagFolder     = 0x02
	flagCompressed = 0x04
	flagMRAM       = 0x10
	flagARAM       = 0x20
	flagYaz0       = 0x80
)

type rarcEntry struct {
	index, flags, off, size int
	path                    string
}

func cstr(b []byte, o int) string {
	need(b, o, 1)
	e := o
	for e < len(b) && b[e] != 0 {
		e++
	}
	if e >= len(b) {
		panic(oob{o, e - o + 1, len(b)})
	}
	return string(b[o:e])
}

// rarcEntries lists every file entry of a big-endian RARC.
func rarcEntries(b []byte) []rarcEntry {
	hdr := u32(b, 8)
	dataStart := hdr + u32(b, 0xC)
	info := hdr
	ndirs, dirOff, fileOff, strOff := u32(b, info), u32(b, info+4)+info, u32(b, info+0xC)+info, u32(b, info+0x14)+info
	type dir struct {
		name         string
		count, first int
	}
	dirs := make([]dir, ndirs)
	for i := 0; i < ndirs; i++ {
		d := dirOff + 0x10*i
		dirs[i] = dir{cstr(b, strOff+u32(b, d+4)), u16(b, d+0xA), u32(b, d+0xC)}
	}
	var out []rarcEntry
	var walk func(di int, prefix string, depth int)
	walk = func(di int, prefix string, depth int) {
		if depth > 64 {
			panic(errors.New("RARC directory loop"))
		}
		if di < 0 || di >= len(dirs) {
			panic(oob{di, 1, len(dirs)})
		}
		d := dirs[di]
		for e := d.first; e < d.first+d.count; e++ {
			f := fileOff + 0x14*e
			word := u32(b, f+4)
			flags, nameOff := word>>24, word&0xFFFFFF
			name := cstr(b, strOff+nameOff)
			if flags&flagFolder != 0 {
				if name == "." || name == ".." {
					continue
				}
				walk(u32(b, f+8), prefix+name+"/", depth+1)
			} else {
				out = append(out, rarcEntry{e, flags, dataStart + u32(b, f+8), u32(b, f+0xC), prefix + name})
			}
		}
	}
	if len(dirs) == 0 {
		panic(oob{0, 1, 0})
	}
	walk(0, dirs[0].name+"/", 0)
	return out
}

type replacement struct {
	data  []byte
	flags int
}

// relayout rebuilds the data section with replaced files, keeping the file
// order (the MRAM/ARAM/DVD grouping relies on it).
func relayout(b []byte, repl map[int]replacement) []byte {
	hdr := u32(b, 8)
	info := hdr
	dataStart := hdr + u32(b, 0xC)
	nfiles, fileOff := u32(b, info+8), info+u32(b, info+0xC)
	type file struct{ off, i, flags, size int }
	var files []file
	for i := 0; i < nfiles; i++ {
		f := fileOff + 0x14*i
		flags := u32(b, f+4) >> 24
		if flags&flagFolder == 0 {
			files = append(files, file{u32(b, f+8), i, flags, u32(b, f+0xC)})
		}
	}
	sort.Slice(files, func(x, y int) bool {
		a, c := files[x], files[y]
		if a.off != c.off {
			return a.off < c.off
		}
		if a.i != c.i {
			return a.i < c.i
		}
		if a.flags != c.flags {
			return a.flags < c.flags
		}
		return a.size < c.size
	})
	var data []byte
	type key struct {
		isNew bool
		a, c  int
	}
	placed := map[key]int{}
	mram, aram := 0, 0
	for _, fl := range files {
		var blob []byte
		var k key
		flags := fl.flags
		if r, ok := repl[fl.i]; ok {
			blob, flags = r.data, r.flags
			k = key{true, fl.i, 0}
		} else {
			s, e := dataStart+fl.off, dataStart+fl.off+fl.size
			if s > len(b) {
				s = len(b)
			}
			if e > len(b) {
				e = len(b)
			}
			if s > e {
				s = e
			}
			blob = b[s:e]
			k = key{false, fl.off, fl.size}
		}
		noff, ok := placed[k]
		if !ok {
			noff = len(data)
			placed[k] = noff
			data = append(data, blob...)
			for len(data)%32 != 0 {
				data = append(data, 0)
			}
		}
		f := fileOff + 0x14*fl.i
		nameOff := u32(b, f+4) & 0xFFFFFF
		be.PutUint32(b[f+4:], uint32(flags<<24|nameOff))
		be.PutUint32(b[f+8:], uint32(noff))
		be.PutUint32(b[f+12:], uint32(len(blob)))
		aligned := (len(blob) + 31) &^ 31
		if flags&flagMRAM != 0 {
			mram += aligned
		} else if flags&flagARAM != 0 {
			aram += aligned
		}
	}
	out := make([]byte, 0, dataStart+len(data))
	out = append(out, b[:dataStart]...)
	out = append(out, data...)
	be.PutUint32(out[4:], uint32(len(out)))
	be.PutUint32(out[0x10:], uint32(len(data)))
	be.PutUint32(out[0x14:], uint32(mram))
	be.PutUint32(out[0x18:], uint32(aram))
	return out
}

// cookFileFunc converts one archive member. It returns nil (unchanged), a
// same-size slice, or a replacement.
type cookFileFunc func(path string, blob []byte, flags int) (same []byte, repl *replacement)

// swapRarc converts a big-endian RARC to little-endian, letting cookFile
// convert or replace each member first.
func swapRarc(b []byte, cookFile cookFileFunc) []byte {
	repl := map[int]replacement{}
	for _, e := range rarcEntries(b) {
		s, end := e.off, e.off+e.size
		if s > len(b) {
			s = len(b)
		}
		if end > len(b) {
			end = len(b)
		}
		if s > end {
			s = end
		}
		blob := append([]byte(nil), b[s:end]...)
		same, r := cookFile(e.path, blob, e.flags)
		if r != nil {
			repl[e.index] = *r
			continue
		}
		if same == nil {
			continue
		}
		if len(same) != e.size {
			panic(errors.New(e.path + " changed size"))
		}
		copy(b[s:end], same)
	}
	if len(repl) > 0 {
		b = relayout(b, repl)
	}
	hdr := u32(b, 8)
	info := hdr
	ndirs, dirOff, nfiles, fileOff := u32(b, info), u32(b, info+4), u32(b, info+8), u32(b, info+0xC)
	sw32(b, 0, 8)
	sw32(b, info, 6)
	sw16(b, info+0x18, 2)
	sw32(b, info+0x1C, 1)
	for i := 0; i < ndirs; i++ {
		d := info + dirOff + 0x10*i
		sw32(b, d, 2)
		sw16(b, d+8, 2)
		sw32(b, d+0xC, 1)
	}
	for i := 0; i < nfiles; i++ {
		f := info + fileOff + 0x14*i
		sw16(b, f, 2)
		sw32(b, f+4, 4)
	}
	return b
}

// rarcFiles lists (path, data) of a big-endian RARC like the reference
// rarc_list (paths keep the root directory's name).
func rarcFiles(data []byte) (paths []string, blobs [][]byte) {
	if tag(data, 0, 4) != "RARC" {
		panic(errors.New("not RARC"))
	}
	dataOff := u32(data, 0x0C) + 0x20
	info := 0x20
	numNodes, nodeOff := u32(data, info), u32(data, info+4)+0x20
	entryOff, strOff := u32(data, info+0xC)+0x20, u32(data, info+0x14)+0x20
	type node struct {
		name     string
		n, first int
	}
	nodes := make([]node, numNodes)
	for i := range nodes {
		p := nodeOff + i*0x10
		nodes[i] = node{cstr(data, strOff+u32(data, p+4)), u16(data, p+0xA), u32(data, p+0xC)}
	}
	var walk func(ni int, prefix string, depth int)
	walk = func(ni int, prefix string, depth int) {
		if depth > 64 || ni < 0 || ni >= len(nodes) {
			panic(errors.New("bad RARC"))
		}
		nd := nodes[ni]
		for e := nd.first; e < nd.first+nd.n; e++ {
			p := entryOff + e*0x14
			ftype := u16(data, p+4) >> 8
			name := cstr(data, strOff+u16(data, p+6))
			off, size := u32(data, p+8), u32(data, p+12)
			if ftype&0x02 != 0 {
				if name == "." || name == ".." {
					continue
				}
				walk(off, prefix+name+"/", depth+1)
			} else {
				s, e2 := dataOff+off, dataOff+off+size
				if s > len(data) {
					s = len(data)
				}
				if e2 > len(data) {
					e2 = len(data)
				}
				if s > e2 {
					s = e2
				}
				paths = append(paths, prefix+name)
				blobs = append(blobs, data[s:e2])
			}
		}
	}
	if len(nodes) == 0 {
		panic(errors.New("bad RARC"))
	}
	walk(0, nodes[0].name+"/", 0)
	return
}
