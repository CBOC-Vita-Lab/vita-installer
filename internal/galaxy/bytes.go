// Package galaxy prepares a Super Mario Galaxy disc for the PS Vita port.
//
// It is a Go translation of the port's own cooker (tools/cook in
// CBOC-Vita-Lab/galaxy-vita): every archive is decompressed and every
// big-endian structure the port reads directly is converted to
// little-endian, the data held by main.dol is extracted, the movies are
// converted (and optionally re-encoded smaller) and the big folders are
// arranged in the Vita's bucket layout. The output is checked file by file
// against the hashes of the reference cooker's output.
package galaxy

import (
	"encoding/binary"
	"fmt"
	"sort"
)

var be = binary.BigEndian

// oob is raised (as a panic) when a read falls outside the buffer, like the
// reference cooker's struct.error; cookFile turns it into an error.
type oob struct{ off, n, size int }

func (e oob) Error() string {
	return fmt.Sprintf("read of %d bytes at %#x outside a %d-byte file", e.n, e.off, e.size)
}

func need(b []byte, o, n int) {
	if o < 0 || o+n > len(b) {
		panic(oob{o, n, len(b)})
	}
}

func u8(b []byte, o int) int      { need(b, o, 1); return int(b[o]) }
func u16(b []byte, o int) int     { need(b, o, 2); return int(be.Uint16(b[o:])) }
func s16(b []byte, o int) int     { need(b, o, 2); return int(int16(be.Uint16(b[o:]))) }
func u32(b []byte, o int) int     { need(b, o, 4); return int(be.Uint32(b[o:])) }
func s32(b []byte, o int) int     { need(b, o, 4); return int(int32(be.Uint32(b[o:]))) }
func u32x(b []byte, o int) uint32 { need(b, o, 4); return be.Uint32(b[o:]) }

// tag returns up to n bytes at o as a string (shorter near the end), like a
// Python slice.
func tag(b []byte, o, n int) string {
	if o < 0 || o >= len(b) {
		return ""
	}
	e := o + n
	if e > len(b) {
		e = len(b)
	}
	return string(b[o:e])
}

// sw16 swaps n 16-bit values at o, clipped to the buffer.
func sw16(b []byte, o int, n int) {
	if o < 0 {
		panic(oob{o, 2, len(b)})
	}
	if m := floorDiv(len(b)-o, 2); m < n {
		n = m
	}
	for i := 0; i < n; i++ {
		p := o + 2*i
		b[p], b[p+1] = b[p+1], b[p]
	}
}

// sw32 swaps n 32-bit values at o, clipped to the buffer.
func sw32(b []byte, o int, n int) {
	if o < 0 {
		panic(oob{o, 4, len(b)})
	}
	if m := floorDiv(len(b)-o, 4); m < n {
		n = m
	}
	for i := 0; i < n; i++ {
		p := o + 4*i
		b[p], b[p+1], b[p+2], b[p+3] = b[p+3], b[p+2], b[p+1], b[p]
	}
}

func floorDiv(a, b int) int {
	q := a / b
	if (a%b != 0) && ((a < 0) != (b < 0)) {
		q--
	}
	return q
}

// swRange swaps every width-byte element in [start, end).
func swRange(b []byte, start, end, width int) {
	if end > len(b) {
		end = len(b)
	}
	n := floorDiv(end-start, width)
	if n <= 0 || width == 1 {
		return
	}
	if width == 2 {
		sw16(b, start, n)
	} else if width == 4 {
		sw32(b, start, n)
	}
}

// extents gives each non-zero offset the next larger one (or limit) as its end.
func extents(offs []int, limit int) []int {
	var vals []int
	seen := map[int]bool{}
	for _, v := range offs {
		if v != 0 && !seen[v] {
			seen[v] = true
			vals = append(vals, v)
		}
	}
	sort.Ints(vals)
	out := make([]int, len(offs))
	for i, v := range offs {
		if v == 0 {
			continue
		}
		next := limit
		for _, w := range vals {
			if w > v {
				next = w
				break
			}
		}
		out[i] = next
	}
	return out
}

// swapNtab converts a ResNTAB: u16 count, u16 pad, {u16 hash, u16 offset}[count].
func swapNtab(b []byte, o int) {
	if o <= 0 || o+4 > len(b) {
		return
	}
	n := u16(b, o)
	sw16(b, o, 2+2*n)
}
