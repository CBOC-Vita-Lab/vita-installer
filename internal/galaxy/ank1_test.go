package galaxy

import (
	"bytes"
	"encoding/binary"
	"testing"
)

// ank1Block builds a big-endian ANK1 block (header 0x24 bytes, joint table at 0x40) with the given key counts.
func ank1Block(nS, nR, nT int) []byte {
	const tbl = 0x40
	sc := tbl + 0x20
	rot := sc + 4*nS
	tr := rot + 2*nR
	size := tr + 4*nT
	b := make([]byte, size)
	copy(b, "ANK1")
	binary.BigEndian.PutUint32(b[4:], uint32(size))
	binary.BigEndian.PutUint16(b[0x0E:], uint16(nS))
	binary.BigEndian.PutUint16(b[0x10:], uint16(nR))
	binary.BigEndian.PutUint16(b[0x12:], uint16(nT))
	for i, v := range []int{tbl, sc, rot, tr} {
		binary.BigEndian.PutUint32(b[0x14+4*i:], uint32(v))
	}
	for i := tbl; i < size; i++ {
		b[i] = byte(i*7 + i>>8)
	}
	return b
}

func TestANK1CountsAreUnsigned(t *testing.T) {
	for _, c := range []struct{ nS, nR, nT int }{{3, 0x8000, 5}, {0x8001, 4, 4}, {2, 2, 0x9000}, {0x7FFF, 0x7FFF, 0x7FFF}} {
		in := ank1Block(c.nS, c.nR, c.nT)
		out := append([]byte(nil), in...)
		ank1(out, 0, len(out))
		sc, rot, tr := int(binary.BigEndian.Uint32(in[0x18:])), int(binary.BigEndian.Uint32(in[0x1C:])), int(binary.BigEndian.Uint32(in[0x20:]))
		for i := 0; i < c.nR; i++ { // every rotation key is little-endian now, whatever the count
			if out[rot+2*i] != in[rot+2*i+1] || out[rot+2*i+1] != in[rot+2*i] {
				t.Fatalf("%v: rotation key %d not swapped", c, i)
			}
		}
		for _, a := range []struct{ off, n int }{{sc, c.nS}, {tr, c.nT}} {
			for i := 0; i < a.n; i++ {
				p := a.off + 4*i
				if binary.LittleEndian.Uint32(out[p:]) != binary.BigEndian.Uint32(in[p:]) {
					t.Fatalf("%v: 32-bit key at %#x not swapped", c, p)
				}
			}
		}
		big := c.nS >= 0x8000 || c.nR >= 0x8000 || c.nT >= 0x8000
		if got := bytes.Equal(out[0x24:0x28], []byte("UCNT")); got != big {
			t.Fatalf("%v: marker %v, want %v", c, got, big)
		}
		if !big && !bytes.Equal(out[0x24:0x28], in[0x24:0x28]) {
			t.Fatalf("%v: padding changed", c)
		}
	}
}
