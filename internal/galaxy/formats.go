package galaxy

import "sort"

// ---- BCSV (JMap tables) ----

func looksLikeBCSV(b []byte) bool {
	if len(b) < 16 {
		return false
	}
	n, items, off, esize := s32(b, 0), s32(b, 4), s32(b, 8), u32(b, 12)
	if n < 0 || items < 0 || items > 512 || n > 1000000 {
		return false
	}
	if off != 16+items*12 {
		return false
	}
	return off+n*esize <= len(b)
}

func swapBCSV(b []byte) {
	n, items, off, esize := s32(b, 0), s32(b, 4), s32(b, 8), u32(b, 12)
	sw32(b, 0, 4)
	type slot struct{ off, size int }
	set := map[slot]bool{}
	for i := 0; i < items; i++ {
		p := 16 + i*12
		need(b, p, 12)
		offs, typ := u16(b, p+8), int(b[p+11])
		sw32(b, p, 2)
		sw16(b, p+8, 1)
		size := map[int]int{0: 4, 2: 4, 3: 4, 6: 4, 4: 2, 5: 1, 1: 0}[typ]
		if size > 1 {
			set[slot{offs, size}] = true
		}
	}
	var slots []slot
	for s := range set {
		slots = append(slots, s)
	}
	sort.Slice(slots, func(i, j int) bool {
		if slots[i].off != slots[j].off {
			return slots[i].off < slots[j].off
		}
		return slots[i].size < slots[j].size
	})
	for e := 0; e < n; e++ {
		base := off + e*esize
		for _, s := range slots {
			p := base + s.off
			if p+s.size > len(b) {
				continue
			}
			if p < 0 {
				panic(oob{p, s.size, len(b)})
			}
			if s.size == 4 {
				b[p], b[p+1], b[p+2], b[p+3] = b[p+3], b[p+2], b[p+1], b[p]
			} else {
				b[p], b[p+1] = b[p+1], b[p]
			}
		}
	}
}

// ---- BMG messages ----

func swapBMG(b []byte) {
	nblocks := u32(b, 0x0C)
	sw32(b, 0, 4)
	o := 0x20
	for k := 0; k < nblocks; k++ {
		if o+8 > len(b) {
			break
		}
		kind, size := tag(b, o, 4), u32(b, o+4)
		switch kind {
		case "INF1":
			n, item := u16(b, o+8), u16(b, o+0xA)
			sw16(b, o+8, 2)
			sw32(b, o+0xC, 1)
			for i := 0; i < n; i++ {
				it := o + 0x10 + item*i
				sw32(b, it, 1)
				sw16(b, it+4, 1)
			}
		case "DAT1":
			swapText(b, o+8, o+size)
		case "FLW1":
			nodes, branches := u16(b, o+8), u16(b, o+0xA)
			sw16(b, o+8, 2)
			sw32(b, o+0xC, 1)
			p := o + 0x10
			for i := 0; i < nodes; i++ {
				sw16(b, p+2, 1)
				if u8(b, p) == 3 {
					sw32(b, p+4, 1)
				} else {
					sw16(b, p+4, 2)
				}
				p += 8
			}
			sw16(b, p, branches)
		case "FLI1":
			n, item := u16(b, o+8), u8(b, o+0xA)
			sw16(b, o+8, 1)
			for i := 0; i < n; i++ {
				it := o + 0x10 + item*i
				sw32(b, it, 1)
				sw16(b, it+4, 1)
			}
		}
		sw32(b, o, 2)
		if size < 8 {
			break
		}
		o += size
	}
}

// ---- NW4R layouts, TPL, BTI, RFNT ----

func swapFileHeader(b []byte) {
	sw32(b, 0, 1)
	sw16(b, 4, 2)
	sw32(b, 8, 1)
	sw16(b, 0xC, 2)
}

type nwBlock struct {
	kind      string
	off, size int
}

func nwBlocks(b []byte) []nwBlock {
	var out []nwBlock
	o := u16(b, 0xC)
	n := u16(b, 0xE)
	for i := 0; i < n; i++ {
		if o+8 > len(b) {
			break
		}
		kind, size := tag(b, o, 4), u32(b, o+4)
		out = append(out, nwBlock{kind, o, size})
		if size < 8 {
			break
		}
		o += size
	}
	return out
}

// swapText converts UTF-16 text with 0x1A message tags.
func swapText(b []byte, start, end int) {
	p := start
	for p+2 <= end {
		c := u16(b, p)
		sw16(b, p, 1)
		if c != 0x1A {
			p += 2
			continue
		}
		if p+6 > end {
			break
		}
		length, group := u8(b, p+2), u8(b, p+3)
		sw16(b, p+4, 1)
		params, pend := p+6, p+length
		switch {
		case group == 1 && length >= 8:
			sw16(b, params, 1)
		case group == 2:
			swRange(b, params, pend, 2)
		case (group == 6 || group == 7) && length >= 14:
			sw32(b, params, 2)
		}
		if length < 6 {
			break
		}
		p += length
	}
}

func materialSwap(b []byte, m int) {
	bits := u32(b, m+0x3C)
	texMap := bits & 0xF
	texSRT := (bits >> 4) & 0xF
	texGen := (bits >> 8) & 0xF
	hasSwap := (bits >> 12) & 1
	indSRT := (bits >> 13) & 3
	_ = (bits >> 15) & 7
	chanCtrl := (bits >> 25) & 1
	matCol := (bits >> 27) & 1
	sw16(b, m+0x14, 12)
	sw32(b, m+0x3C, 1)
	o := m + 0x40
	for i := 0; i < texMap; i++ {
		sw16(b, o, 1)
		o += 4
	}
	sw32(b, o, 5*texSRT)
	o += 20 * texSRT
	o += 4*texGen + 4*chanCtrl + 4*matCol + 4*hasSwap
	sw32(b, o, 5*indSRT)
}

func paneBase(b []byte, o int) { sw32(b, o+0x24, 10) }

func swapRLYT(b []byte) {
	bl := nwBlocks(b)
	swapFileHeader(b)
	for _, k := range bl {
		o, size := k.off, k.size
		end := o + size
		switch k.kind {
		case "lyt1":
			sw32(b, o+0xC, 2)
		case "txl1", "fnl1":
			n := u16(b, o+8)
			sw16(b, o+8, 1)
			for i := 0; i < n; i++ {
				sw32(b, o+0xC+8*i, 1)
			}
		case "mat1":
			n := u16(b, o+8)
			offs := make([]int, n)
			for i := range offs {
				offs[i] = u32(b, o+0xC+4*i)
			}
			sw16(b, o+8, 1)
			sw32(b, o+0xC, n)
			for _, mo := range offs {
				materialSwap(b, o+mo)
			}
		case "pan1", "bnd1":
			paneBase(b, o)
		case "pic1":
			ntc := u8(b, o+0x5E)
			paneBase(b, o)
			sw32(b, o+0x4C, 4)
			sw16(b, o+0x5C, 1)
			sw32(b, o+0x60, 8*ntc)
		case "txt1":
			strBytes := u16(b, o+0x4E)
			strOff := u32(b, o+0x58)
			paneBase(b, o)
			sw16(b, o+0x4C, 4)
			sw32(b, o+0x58, 3)
			sw32(b, o+0x64, 4)
			if strBytes != 0 {
				e := o + strOff + strBytes
				if end < e {
					e = end
				}
				swapText(b, o+strOff, e)
			}
		case "wnd1":
			nframes := u8(b, o+0x5C)
			content, frames := u32(b, o+0x60), u32(b, o+0x64)
			paneBase(b, o)
			sw32(b, o+0x4C, 4)
			sw32(b, o+0x60, 2)
			c := o + content
			ntc := u8(b, c+0x12)
			sw32(b, c, 4)
			sw16(b, c+0x10, 1)
			sw32(b, c+0x14, 8*ntc)
			for i := 0; i < nframes; i++ {
				fo := u32(b, o+frames+4*i)
				sw32(b, o+frames+4*i, 1)
				sw16(b, o+fo, 1)
			}
		case "grp1":
			sw16(b, o+0x18, 1)
		}
		sw32(b, o, 2)
	}
}

func swapRLAN(b []byte) {
	bl := nwBlocks(b)
	swapFileHeader(b)
	for _, k := range bl {
		o := k.off
		switch k.kind {
		case "pat1":
			sw16(b, o+8, 2)
			sw32(b, o+0xC, 2)
			sw16(b, o+0x14, 2)
		case "pah1":
			sw32(b, o+8, 1)
			sw16(b, o+0xC, 1)
		case "pai1":
			fileNum := u16(b, o+0xC)
			contNum := u16(b, o+0xE)
			contOffsAt := o + u32(b, o+0x10)
			conts := make([]int, contNum)
			for i := range conts {
				conts[i] = u32(b, contOffsAt+4*i)
			}
			sw16(b, o+8, 1)
			sw16(b, o+0xC, 2)
			sw32(b, o+0x10, 1)
			sw32(b, o+0x14, fileNum)
			sw32(b, contOffsAt, contNum)
			for _, co := range conts {
				c := o + co
				ninfo := u8(b, c+20)
				infos := make([]int, ninfo)
				for i := range infos {
					infos[i] = u32(b, c+24+4*i)
				}
				sw32(b, c+24, ninfo)
				for _, io := range infos {
					info := c + io
					ntgt := u8(b, info+4)
					tgts := make([]int, ntgt)
					for i := range tgts {
						tgts[i] = u32(b, info+8+4*i)
					}
					sw32(b, info, 1)
					sw32(b, info+8, ntgt)
					for _, to := range tgts {
						t := info + to
						curve := u8(b, t+2)
						nkeys := u16(b, t+4)
						keys := t + u32(b, t+8)
						sw16(b, t+4, 1)
						sw32(b, t+8, 1)
						if curve == 1 {
							for i := 0; i < nkeys; i++ {
								sw32(b, keys+8*i, 1)
								sw16(b, keys+8*i+4, 1)
							}
						} else if curve == 2 {
							sw32(b, keys, 3*nkeys)
						}
					}
				}
			}
		}
		sw32(b, o, 2)
	}
}

const tplVersion = 2142000

func swapTPL(b []byte) {
	n, table := u32(b, 4), u32(b, 8)
	type desc struct{ tex, clut int }
	descs := make([]desc, n)
	for i := range descs {
		descs[i] = desc{u32(b, table+8*i), u32(b, table+8*i+4)}
	}
	sw32(b, 0, 3)
	sw32(b, table, 2*n)
	for _, d := range descs {
		if d.tex != 0 {
			sw16(b, d.tex, 2)
			sw32(b, d.tex+4, 7)
		}
		if d.clut != 0 {
			sw16(b, d.clut, 1)
			sw32(b, d.clut+4, 2)
		}
	}
}

func swapTIMG(b []byte, o int) {
	sw16(b, o+0x02, 2)
	sw16(b, o+0x0A, 1)
	sw32(b, o+0x0C, 1)
	sw16(b, o+0x1A, 1)
	sw32(b, o+0x1C, 1)
}

func swapRFNT(b []byte) {
	bl := nwBlocks(b)
	swapFileHeader(b)
	for _, k := range bl {
		o, d := k.off, k.off+8
		switch k.kind {
		case "FINF":
			sw16(b, d+2, 1)
			sw32(b, d+8, 3)
		case "TGLP":
			sw32(b, d+4, 1)
			sw16(b, d+8, 6)
			sw32(b, d+0x14, 1)
		case "CWDH":
			sw16(b, d, 2)
			sw32(b, d+4, 1)
		case "CMAP":
			sw16(b, d, 4)
			sw32(b, d+8, 1)
			swRange(b, d+0xC, o+k.size, 2)
		}
		sw32(b, o, 2)
	}
}

// ---- JParticle (.jpc) ----

func swapJPC(b []byte) {
	if tag(b, 4, 4) != "2-10" {
		return
	}
	nres, ntex, texOff := u16(b, 8), u16(b, 0xA), u32(b, 0xC)
	sw32(b, 4, 1)
	sw16(b, 8, 2)
	sw32(b, 0xC, 1)
	o := 0x10
	for r := 0; r < nres; r++ {
		nblocks := u16(b, o+2)
		sw16(b, o, 2)
		o += 8
		for k := 0; k < nblocks; k++ {
			kind, size := tag(b, o, 4), u32(b, o+4)
			switch kind {
			case "BEM1":
				sw32(b, o+0x08, 24)
				sw16(b, o+0x68, 8)
			case "BSP1":
				flags := u32(b, o+0x08)
				prmOff, envOff := s16(b, o+0x0C), s16(b, o+0x0E)
				clrFlg, prmNum, envNum := u8(b, o+0x21), u8(b, o+0x22), u8(b, o+0x23)
				sw32(b, o+0x08, 1)
				sw16(b, o+0x0C, 2)
				sw32(b, o+0x10, 2)
				sw16(b, o+0x18, 1)
				sw16(b, o+0x24, 1)
				if flags&0x01000000 != 0 {
					sw32(b, o+0x34, 10)
				}
				if clrFlg&0x02 != 0 {
					for i := 0; i < prmNum; i++ {
						sw16(b, o+prmOff+6*i, 1)
					}
				}
				if clrFlg&0x08 != 0 {
					for i := 0; i < envNum; i++ {
						sw16(b, o+envOff+6*i, 1)
					}
				}
			case "ESP1":
				sw32(b, o+0x08, 8)
				sw16(b, o+0x28, 2)
				sw32(b, o+0x2C, 13)
			case "SSP1":
				sw32(b, o+0x08, 11)
				sw32(b, o+0x3C, 1)
				sw16(b, o+0x40, 2)
				sw16(b, o+0x46, 1)
			case "ETX1":
				sw32(b, o+0x08, 7)
			case "FLD1":
				sw32(b, o+0x08, 14)
			case "KFA1":
				sw32(b, o+0x0C, 4*u8(b, o+9))
			case "TDB1":
				swRange(b, o+8, o+size, 2)
			}
			sw32(b, o, 2)
			o += size
		}
	}
	o = texOff
	for t := 0; t < ntex; t++ {
		size := u32(b, o+4)
		sw32(b, o, 3)
		swapTIMG(b, o+0x20)
		o += size
	}
}
