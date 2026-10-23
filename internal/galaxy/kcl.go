package galaxy

import (
	"errors"
)

const kclHeader = 0x38

func looksLikeKCL(b []byte) bool {
	if len(b) < kclHeader {
		return false
	}
	pos, nrm, prism, octree := u32(b, 0), u32(b, 4), u32(b, 8), u32(b, 12)
	return pos == kclHeader && pos <= nrm && nrm <= prism+0x10 && prism+0x10 <= octree && octree <= len(b)
}

func kclRootCount(b []byte) int {
	xmask, ymask, zmask := uint32(s32(b, 0x20)), uint32(s32(b, 0x24)), uint32(s32(b, 0x28))
	shift, xshift, xyshift := s32(b, 0x2C), s32(b, 0x30), s32(b, 0x34)
	if xshift == -1 && xyshift == -1 {
		return 1
	}
	if shift < 0 || shift > 63 {
		panic(errors.New("bad KCL shift"))
	}
	nx := int(uint64(^xmask)>>uint(shift)) + 1
	ny := int(uint64(^ymask)>>uint(shift)) + 1
	nz := int(uint64(^zmask)>>uint(shift)) + 1
	return nx * ny * nz
}

func swapKCL(b []byte) {
	n := len(b)
	posOff, nrmOff, prismOff, octreeOff := u32(b, 0), u32(b, 4), u32(b, 8), u32(b, 12)
	nodeWords := map[int]bool{}
	listHalves := map[int]bool{}
	type blk struct{ base, count int }
	stack := []blk{{octreeOff, kclRootCount(b)}}
	seen := map[blk]bool{}
	for len(stack) > 0 {
		top := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if seen[top] {
			continue
		}
		seen[top] = true
		for i := 0; i < top.count; i++ {
			off := top.base + 4*i
			if off+4 > n {
				panic(errors.New("octree node outside file"))
			}
			nodeWords[off] = true
			v := s32(b, off)
			if v >= 0 {
				stack = append(stack, blk{top.base + v, 8})
				continue
			}
			p := top.base + (v & 0x7FFFFFFF) + 2
			for {
				if p+2 > n {
					panic(errors.New("prism list runs off the end of the file"))
				}
				listHalves[p] = true
				if u16(b, p) == 0 {
					break
				}
				p += 2
			}
		}
	}
	for o := range listHalves {
		if nodeWords[o&^3] {
			panic(errors.New("octree nodes and prism lists overlap"))
		}
	}
	sw32(b, 0, kclHeader/4)
	swRange(b, posOff, nrmOff, 4)
	swRange(b, nrmOff, prismOff+0x10, 4)
	for r := prismOff + 0x10; r < octreeOff; r += 0x10 {
		sw32(b, r, 1)
		sw16(b, r+4, 6)
	}
	for off := range nodeWords {
		sw32(b, off, 1)
	}
	for off := range listHalves {
		sw16(b, off, 1)
	}
}
