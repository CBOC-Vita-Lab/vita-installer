package galaxy

import (
	"bytes"
	"errors"
	"fmt"
	"os"
)

// Bucket folders: the Vita build keeps a directory with more than MinFiles
// files as sub-folders _bk00, _bk01, ... of about PerBucket files each
// (FNV-1a of the lower-case name modulo the folder count), because looking
// a name up in a big folder on the memory card is slow. The layout follows
// from the disc's file table (sys/fst.bin) alone.

// BucketOf returns the bucket folder index for a file name.
func BucketOf(name []byte, n int) int {
	h := uint32(2166136261)
	for _, c := range name {
		if c >= 'A' && c <= 'Z' {
			c += 32
		}
		h = (h ^ uint32(c)) * 16777619
	}
	return int(h % uint32(n))
}

type fstEntry struct {
	dir  bool
	name []byte
	a, b int
}

func readFST(path string) ([]fstEntry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(data) < 12 {
		return nil, errors.New("fst.bin is damaged")
	}
	count := int(be.Uint32(data[8:]))
	strs := count * 12
	if strs > len(data) {
		return nil, errors.New("fst.bin is damaged")
	}
	out := make([]fstEntry, count)
	for i := 0; i < count; i++ {
		e := data[i*12 : i*12+12]
		out[i] = fstEntry{dir: e[0] != 0, a: int(be.Uint32(e[4:])), b: int(be.Uint32(e[8:]))}
		if i > 0 {
			off := strs + int(be.Uint32(e)&0xFFFFFF)
			if off >= len(data) {
				return nil, errors.New("fst.bin is damaged")
			}
			z := bytes.IndexByte(data[off:], 0)
			if z < 0 {
				return nil, errors.New("fst.bin is damaged")
			}
			out[i].name = data[off : off+z]
		}
	}
	return out, nil
}

// Move is one file's flat path and bucketed path, relative to the output.
type Move struct{ From, To string }

// BucketLayout computes where each file of a big directory goes.
func BucketLayout(fstPath string, minFiles, perBucket int) ([]Move, error) {
	entries, err := readFST(fstPath)
	if err != nil {
		return nil, err
	}
	count := len(entries)
	dirPath := make([]string, count)
	stack := []int{0}
	for i := 1; i < count; i++ {
		for len(stack) > 1 && i >= entries[stack[len(stack)-1]].b {
			stack = stack[:len(stack)-1]
		}
		if entries[i].dir {
			parent := dirPath[stack[len(stack)-1]]
			if parent != "" {
				parent += "/"
			}
			dirPath[i] = parent + latin1(entries[i].name)
			stack = append(stack, i)
		}
	}
	var moves []Move
	for d := 0; d < count; d++ {
		if !entries[d].dir {
			continue
		}
		end := count
		if d != 0 {
			end = entries[d].b
		}
		var kids []int
		for i := d + 1; i < end; {
			if entries[i].dir {
				if entries[i].b <= i {
					return nil, errors.New("fst.bin is damaged")
				}
				i = entries[i].b
			} else {
				kids = append(kids, i)
				i++
			}
		}
		if len(kids) <= minFiles {
			continue
		}
		n := (len(kids) + perBucket - 1) / perBucket
		prefix := "files/"
		if dirPath[d] != "" {
			prefix += dirPath[d] + "/"
		}
		for _, k := range kids {
			nm := latin1(entries[k].name)
			moves = append(moves, Move{prefix + nm, prefix + fmt.Sprintf("_bk%02d/", BucketOf(entries[k].name, n)) + nm})
		}
	}
	return moves, nil
}

func latin1(b []byte) string {
	r := make([]rune, len(b))
	for i, c := range b {
		r[i] = rune(c)
	}
	return string(r)
}
