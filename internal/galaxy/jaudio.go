package galaxy

import "sort"

// JAudio2 resources. Sequence byte code stays big-endian.

const (
	dataSE, dataBGM, dataStream = 0x50, 0x60, 0x70
)

// swapper swaps each location at most once (records can be shared).
type swapper struct {
	b    []byte
	done map[int]bool
}

func newSwapper(b []byte) *swapper { return &swapper{b, map[int]bool{}} }

func (s *swapper) w32(o, n int) {
	for i := 0; i < n; i++ {
		p := o + 4*i
		if !s.done[p] && p+4 <= len(s.b) {
			s.done[p] = true
			sw32(s.b, p, 1)
		}
	}
}

func (s *swapper) w16(o, n int) {
	for i := 0; i < n; i++ {
		p := o + 2*i
		if !s.done[p] && p+2 <= len(s.b) {
			s.done[p] = true
			sw16(s.b, p, 1)
		}
	}
}

func swapBST(b []byte, base int) {
	s := newSwapper(b)
	root := base + u32(b, base+12)
	nsec := u32(b, root)
	secs := make([]int, nsec)
	for i := range secs {
		secs[i] = u32(b, root+4+4*i)
	}
	for _, so := range secs {
		if so == 0 {
			continue
		}
		sec := base + so
		ngrp := u32(b, sec)
		grps := make([]int, ngrp)
		for i := range grps {
			grps[i] = u32(b, sec+4+4*i)
		}
		for _, gofs := range grps {
			if gofs == 0 {
				continue
			}
			grp := base + gofs
			nitems := u32(b, grp)
			for i := 0; i < nitems; i++ {
				word := u32(b, grp+8+4*i)
				typ, off := word>>24, word&0xFFFFFF
				if off == 0 {
					continue
				}
				item := base + off
				switch typ & 0xF0 {
				case dataSE:
					s.w16(item+2, 2)
				case dataBGM:
					s.w16(item+2, 3)
				case dataStream:
					s.w16(item+2, 1)
					s.w32(item+4, 1)
				default:
					s.w16(item+2, 1)
				}
			}
			s.w32(grp, 2+nitems)
		}
		s.w32(sec, 1+ngrp)
	}
	s.w32(root, 1+nsec)
	s.w32(base, 4)
}

func swapBSTN(b []byte, base int) {
	s := newSwapper(b)
	root := base + u32(b, base+12)
	nsec := u32(b, root)
	secs := make([]int, nsec)
	for i := range secs {
		secs[i] = u32(b, root+4+4*i)
	}
	for _, so := range secs {
		if so == 0 {
			continue
		}
		sec := base + so
		ngrp := u32(b, sec)
		grps := make([]int, ngrp)
		for i := range grps {
			grps[i] = u32(b, sec+8+4*i)
		}
		for _, gofs := range grps {
			if gofs == 0 {
				continue
			}
			grp := base + gofs
			s.w32(grp, 2+u32(b, grp))
		}
		s.w32(sec, 2+ngrp)
	}
	s.w32(root, 1+nsec)
	s.w32(base, 4)
}

func swapBSC(b []byte, base int) {
	s := newSwapper(b)
	ngroups := u16(b, base+2)
	tables := make([]int, ngroups)
	for i := range tables {
		tables[i] = u32(b, base+8+4*i)
	}
	for _, to := range tables {
		t := base + to
		s.w32(t, 1+u32(b, t))
	}
	s.w16(base+2, 1)
	s.w32(base+4, 1+ngroups)
}

func swapWSYS(b []byte, base int) {
	s := newSwapper(b)
	arcBank := base + u32(b, base+0x10)
	ctrlGroup := base + u32(b, base+0x14)
	narc := u32(b, arcBank+4)
	for i := 0; i < narc; i++ {
		ao := u32(b, arcBank+8+4*i)
		if ao == 0 {
			continue
		}
		arc := base + ao
		nwave := u32(b, arc+0x70)
		for j := 0; j < nwave; j++ {
			wo := u32(b, arc+0x74+4*j)
			if wo == 0 {
				continue
			}
			wave := base + wo
			s.w32(wave+4, 7)
			s.w16(wave+0x20, 2)
		}
		s.w32(arc+0x70, 1+nwave)
	}
	s.w32(arcBank+4, 1+narc)
	ngroups := u32(b, ctrlGroup+8)
	for i := 0; i < ngroups; i++ {
		so := u32(b, ctrlGroup+0xC+4*i)
		if so == 0 {
			continue
		}
		scene := base + so
		for k := 0; k < 3; k++ {
			co := u32(b, scene+4+4*k)
			if co == 0 {
				continue
			}
			ctrl := base + co
			if tag(b, ctrl, 4) != "C-DF" && k != 2 {
				s.w32(scene+4+4*k, 1)
				continue
			}
			nw := u32(b, ctrl+4)
			for j := 0; j < nw; j++ {
				if cw := u32(b, ctrl+8+4*j); cw != 0 {
					s.w32(base+cw, 1)
				}
			}
			s.w32(ctrl+4, 1+nw)
			s.w32(scene+4+4*k, 1)
		}
	}
	s.w32(ctrlGroup+8, 1+ngroups)
	s.w32(base+4, 5)
}

func swapIBNK(b []byte, base int) {
	s := newSwapper(b)
	size := u32(b, base+4)
	if u32(b, base+0xC) != 1 {
		return
	}
	end := base + size
	type chunk struct{ c, size int }
	chunks := map[string]chunk{}
	var order []chunk
	c := base + 0x20
	for c+8 <= end {
		cid, csize := tag(b, c, 4), u32(b, c+4)
		chunks[cid] = chunk{c, csize}
		order = append(order, chunk{c, csize})
		next := (c + 0xB + csize) &^ 3
		if next <= c {
			break
		}
		c = next
	}
	effect := func(off int) {
		if off == 0 {
			return
		}
		e := base + off
		if s.done[e] {
			return
		}
		t := tag(b, e, 4)
		if t == "Rand" || t == "Sens" {
			s.w32(e+8, 2)
			s.w32(e, 1)
		}
	}
	veloRegions := func(p, count int) int {
		for i := 0; i < count; i++ {
			s.w32(p+4, 3)
			p += 16
		}
		return p
	}
	if ch, ok := chunks["ENVT"]; ok {
		s.w16(ch.c+8, ch.size/2)
	}
	if ch, ok := chunks["OSCT"]; ok {
		n := u32(b, ch.c+8)
		for i := 0; i < n; i++ {
			o := ch.c + 0xC + 0x1C*i
			s.w32(o, 1)
			s.w32(o+8, 5)
		}
		s.w32(ch.c+8, 1)
	}
	if ch, ok := chunks["LIST"]; ok {
		n := u32(b, ch.c+8)
		for i := 0; i < n; i++ {
			off := u32(b, ch.c+0xC+4*i)
			if off == 0 {
				continue
			}
			p := base + off
			switch tag(b, p, 4) {
			case "Inst":
				q := p + 4
				nosc := u32(b, q)
				s.w32(q, 1+nosc)
				q += 4 + 4*nosc
				neff := u32(b, q)
				for k := 0; k < neff; k++ {
					effect(u32(b, q+4+4*k))
				}
				s.w32(q, 1+neff)
				q += 4 + 4*neff
				nkey := u32(b, q)
				s.w32(q, 1)
				q += 4
				for k := 0; k < nkey; k++ {
					nvel := u32(b, q+4)
					s.w32(q, 2)
					q = veloRegions(q+8, nvel)
				}
				s.w32(q, 2)
				s.w32(p, 1)
			case "Perc":
				cnt := u32(b, p+4)
				for k := 0; k < cnt; k++ {
					po := u32(b, p+8+4*k)
					if po == 0 {
						continue
					}
					pm := base + po
					s.w32(pm+4, 2)
					s.w16(pm+0xE, 1)
					neff := u32(b, pm+0x10)
					for e := 0; e < neff; e++ {
						effect(u32(b, pm+0x14+4*e))
					}
					q := pm + 0x14 + 4*neff
					nvel := u32(b, q)
					veloRegions(q+4, nvel)
					s.w32(q, 1)
					s.w32(pm+0x10, 1+neff)
					s.w32(pm, 1)
				}
				s.w32(p, 2+cnt)
			}
		}
		s.w32(ch.c+8, 1+n)
	}
	for _, ch := range order {
		s.w32(ch.c, 2)
	}
	s.w32(base, 4)
}

var cmdArgs = map[string]int{"ws  ": 3, "bnk ": 2, "bl_<": 2, ">_bl": 0, "bsc ": 2, "bst ": 2, "bstn": 2, "bms ": 3, "bmsa": 1, "vbnk": 2, "dsqb": 1, "bsft": 1}

func swapBAA(b []byte) {
	if tag(b, 0, 4) != "AA_<" {
		return
	}
	p := 4
	words := []int{0}
	for {
		t := tag(b, p, 4)
		words = append(words, p)
		p += 4
		if t == ">_AA" {
			break
		}
		if t == "sect" {
			p += 4
			continue
		}
		n, ok := cmdArgs[t]
		if !ok {
			break
		}
		args := make([]int, n)
		for i := range args {
			args[i] = u32(b, p+4*i)
			words = append(words, p+4*i)
		}
		p += 4 * n
		switch t {
		case "ws  ":
			swapWSYS(b, args[1])
		case "bnk ":
			swapIBNK(b, args[1])
		case "bst ":
			swapBST(b, args[0])
		case "bstn":
			swapBSTN(b, args[0])
		case "bsc ":
			swapBSC(b, args[0])
		}
	}
	for _, o := range words {
		sw32(b, o, 1)
	}
}

func swapCIT(b []byte) {
	if tag(b, 4, 4) != "CITS" {
		return
	}
	nchord, nscale := u16(b, 0xC), u16(b, 0xE)
	scales := make([]int, nscale)
	for i := range scales {
		scales[i] = u32(b, 0x10+4*(nchord+i))
	}
	s := newSwapper(b)
	for _, so := range scales {
		s.w32(so, 2)
	}
	s.w32(0x10, nchord+nscale)
	s.w16(0xC, 2)
	s.w32(0, 1)
	s.w32(8, 1)
}

func swapMETable(b []byte) {
	n, entries, names := s32(b, 0), s32(b, 4), s32(b, 8)
	for i := 0; i < n; i++ {
		sw16(b, entries+8*i+4, 1)
	}
	sw32(b, names, n)
	sw32(b, 0, 3)
}

func swapMESeq(b []byte) {
	n := s32(b, 0)
	sw32(b, 0, 1+n)
}

func swapWords(b []byte) { sw32(b, 0, len(b)/4) }

func swapSpkTable(b []byte) {
	n, entries, names := s32(b, 0), s32(b, 4), s32(b, 8)
	for i := 0; i < n; i++ {
		e := entries + 8*i
		sw16(b, e, 1)
		sw16(b, e+4, 2)
	}
	sw32(b, names, n)
	sw32(b, 0, 4)
}

func swapSpkWave(b []byte) {
	n := u32(b, 4)
	offs := make([]int, n)
	set := map[int]bool{len(b): true}
	for i := range offs {
		offs[i] = u32(b, 8+4*i)
		set[offs[i]] = true
	}
	var ends []int
	for v := range set {
		ends = append(ends, v)
	}
	sort.Ints(ends)
	for _, o := range offs {
		end := len(b)
		for i, v := range ends {
			if v == o && i+1 < len(ends) {
				end = ends[i+1]
				break
			}
		}
		sw32(b, o, 3)
		swRange(b, o+12, end, 2)
	}
	sw32(b, 0, 2+n)
}

func swapAST(b []byte) {
	channels := u16(b, 0x0C)
	sw16(b, 0x0C, 2)
	sw32(b, 0x10, 5)
	o := 0x40
	for o+0x20 <= len(b) {
		size := u32(b, o+4)
		sw32(b, o+4, 1)
		sw16(b, o+8, 12)
		o += 0x20 + size*channels
	}
}
