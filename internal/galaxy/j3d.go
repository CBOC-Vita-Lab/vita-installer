package galaxy

// J3D models (BMD/BDL) and animations. Display lists, texture images and
// palettes stay big-endian.

type span struct{ off, width, count int }
type recordLayout struct {
	size  int
	spans []span
}

func swapRecords(b []byte, start, end int, r recordLayout) {
	if r.size <= 0 {
		return
	}
	if end > len(b) {
		end = len(b)
	}
	n := floorDiv(end-start, r.size)
	for i := 0; i < n; i++ {
		base := start + i*r.size
		for _, s := range r.spans {
			if s.width == 2 {
				sw16(b, base+s.off, s.count)
			} else if s.width == 4 {
				sw32(b, base+s.off, s.count)
			}
		}
	}
}

var (
	matInit   = recordLayout{0x14C, []span{{0x08, 2, (0x9C - 0x08) / 2}, {0xBC, 2, (0x14C - 0xBC) / 2}}}
	indInit   = recordLayout{0x138, []span{{0x14, 4, 6}, {0x30, 4, 6}, {0x4C, 4, 6}}}
	light     = recordLayout{0x34, []span{{0x00, 4, 6}, {0x1C, 4, 6}}}
	texMtx    = recordLayout{0x64, []span{{0x04, 4, 3}, {0x10, 4, 2}, {0x18, 2, 1}, {0x1C, 4, 2}, {0x24, 4, 16}}}
	fog       = recordLayout{0x2C, []span{{0x02, 2, 1}, {0x04, 4, 4}, {0x18, 2, 10}}}
	nbt       = recordLayout{0x10, []span{{0x04, 4, 3}}}
	joint     = recordLayout{0x40, []span{{0x00, 2, 1}, {0x04, 4, 3}, {0x10, 2, 3}, {0x18, 4, 3}, {0x24, 4, 1}, {0x28, 4, 6}}}
	shapeInit = recordLayout{0x28, []span{{0x02, 2, 4}, {0x0C, 4, 7}}}
	shapeMtx  = recordLayout{0x08, []span{{0x00, 2, 2}, {0x04, 4, 1}}}
	timg      = recordLayout{0x20, []span{{0x02, 2, 2}, {0x0A, 2, 1}, {0x0C, 4, 1}, {0x1A, 2, 1}, {0x1C, 4, 1}}}
	regTable  = recordLayout{0x1C, []span{{0x00, 2, 12}}}
	tptTable  = recordLayout{0x08, []span{{0x00, 2, 2}, {0x06, 2, 1}}}
)

// table kinds: 0 = skip, 1 = bytes, 2/4 = element width, -1 = name table, or a record layout.
type tkind struct {
	width int
	rec   *recordLayout
	ntab  bool
}

func w(n int) tkind            { return tkind{width: n} }
func rec(r recordLayout) tkind { return tkind{rec: &r} }

var ntab = tkind{ntab: true}
var skip = tkind{}

var mat3Tables = []tkind{
	rec(matInit), w(2), ntab, rec(indInit), w(4), w(1), w(1), w(1), w(1), rec(light), w(1), w(1), w(1), rec(texMtx),
	rec(texMtx), w(2), w(1), w(2), w(1), w(1), w(1), w(1), w(1), rec(fog), w(1), w(1), w(1), w(1), w(1), rec(nbt),
}

func applyKind(b []byte, start, end int, k tkind) {
	switch {
	case k.ntab:
		swapNtab(b, start)
	case k.rec != nil:
		swapRecords(b, start, end, *k.rec)
	case k.width == 2 || k.width == 4:
		swRange(b, start, end, k.width)
	}
}

// anmTables swaps the tables whose u32 offsets start at o+offsAt.
func anmTables(b []byte, o, end, offsAt int, kinds []tkind) {
	offs := make([]int, len(kinds))
	for i := range kinds {
		offs[i] = u32(b, o+offsAt+4*i)
	}
	ends := extents(offs, end-o)
	for i, k := range kinds {
		if offs[i] == 0 || (k == skip) {
			continue
		}
		applyKind(b, o+offs[i], o+ends[i], k)
	}
	sw32(b, o+offsAt, len(kinds))
}

const (
	vaPOS, vaNRM, vaCLR0, vaCLR1, vaTEX0, vaNBT, vaNULL = 9, 10, 11, 12, 13, 25, 0xFF
)

var compWidth = map[int]int{0: 1, 1: 1, 2: 2, 3: 2, 4: 4}
var colorWidth = map[int]int{0: 2, 1: 1, 2: 1, 3: 2, 4: 1, 5: 1}

func get1(m map[int]int, k int) int {
	if v, ok := m[k]; ok {
		return v
	}
	return 1
}

func blkINF1(b []byte, o, end int) {
	if hier := u32(b, o+0x14); hier != 0 {
		swRange(b, o+hier, end, 2)
	}
	sw16(b, o+0x08, 1)
	sw32(b, o+0x0C, 3)
}

func blkVTX1(b []byte, o, end int) {
	// fmt pos nrm nbt clr0 clr1 tex0..7
	offs := make([]int, 14)
	for i := range offs {
		offs[i] = u32(b, o+0x08+4*i)
	}
	ends := extents(offs, end-o)
	type fmtT struct{ cnt, typ int }
	fmts := map[int]fmtT{}
	p := 0
	if offs[0] != 0 {
		p = o + offs[0]
	}
	for p != 0 && p+16 <= end {
		attr, cnt, typ := u32(b, p), u32(b, p+4), u32(b, p+8)
		sw32(b, p, 3)
		if attr == vaNULL {
			break
		}
		fmts[attr] = fmtT{cnt, typ}
		p += 16
	}
	attrOf := []int{0, vaPOS, vaNRM, vaNBT, vaCLR0, vaCLR1}
	for i := 0; i < 8; i++ {
		attrOf = append(attrOf, vaTEX0+i)
	}
	for i := 1; i < 14; i++ {
		if offs[i] == 0 {
			continue
		}
		attr := attrOf[i]
		f, ok := fmts[attr]
		if !ok && attr == vaNBT {
			f, ok = fmts[vaNRM]
		}
		if !ok {
			continue
		}
		width := get1(compWidth, f.typ)
		if attr == vaCLR0 || attr == vaCLR1 {
			width = get1(colorWidth, f.typ)
		}
		swRange(b, o+offs[i], o+ends[i], width)
	}
	sw32(b, o+0x08, 14)
}

func blkEVP1(b []byte, o, end int) {
	offs := make([]int, 4)
	for i := range offs {
		offs[i] = u32(b, o+0x0C+4*i)
	}
	ends := extents(offs, end-o)
	if offs[1] != 0 {
		swRange(b, o+offs[1], o+ends[1], 2)
	}
	if offs[2] != 0 {
		swRange(b, o+offs[2], o+ends[2], 4)
	}
	if offs[3] != 0 {
		swRange(b, o+offs[3], o+ends[3], 4)
	}
	sw16(b, o+0x08, 1)
	sw32(b, o+0x0C, 4)
}

func blkDRW1(b []byte, o, end int) {
	offs := []int{u32(b, o+0x0C), u32(b, o+0x10)}
	ends := extents(offs, end-o)
	if offs[1] != 0 {
		swRange(b, o+offs[1], o+ends[1], 2)
	}
	sw16(b, o+0x08, 1)
	sw32(b, o+0x0C, 2)
}

func blkJNT1(b []byte, o, end int) {
	num := u16(b, o+0x08)
	init, index, name := u32(b, o+0x0C), u32(b, o+0x10), u32(b, o+0x14)
	if init != 0 {
		swapRecords(b, o+init, o+init+num*joint.size, joint)
	}
	if index != 0 {
		sw16(b, o+index, num)
	}
	if name != 0 {
		swapNtab(b, o+name)
	}
	sw16(b, o+0x08, 1)
	sw32(b, o+0x0C, 3)
}

func blkMAT3(b []byte, o, end int) {
	offs := make([]int, len(mat3Tables))
	for i := range offs {
		offs[i] = u32(b, o+0x0C+4*i)
	}
	ends := extents(offs, end-o)
	for i, k := range mat3Tables {
		if offs[i] == 0 {
			continue
		}
		applyKind(b, o+offs[i], o+ends[i], k)
	}
	sw32(b, o+0x0C, len(mat3Tables))
	sw16(b, o+0x08, 1)
}

func blkMDL3(b []byte, o, end int) {
	num := u16(b, o+0x08)
	v := make([]int, 6)
	for i := range v {
		v[i] = u32(b, o+0x0C+4*i)
	}
	dl, patch, cur, name := v[0], v[1], v[2], v[5]
	if dl != 0 {
		sw32(b, o+dl, 2*num)
	}
	if patch != 0 {
		swapRecords(b, o+patch, o+patch+16*num, recordLayout{16, []span{{0, 2, 6}}})
	}
	if cur != 0 {
		sw32(b, o+cur, 2*num)
	}
	if name != 0 {
		swapNtab(b, o+name)
	}
	sw16(b, o+0x08, 1)
	sw32(b, o+0x0C, 6)
}

func blkSHP1(b []byte, o, end int) {
	num := u16(b, o+0x08)
	offs := make([]int, 8) // init index name vtxDesc mtxTable dl mtxInit drawInit
	for i := range offs {
		offs[i] = u32(b, o+0x0C+4*i)
	}
	ends := extents(offs, end-o)
	if offs[0] != 0 {
		swapRecords(b, o+offs[0], o+offs[0]+num*shapeInit.size, shapeInit)
	}
	if offs[1] != 0 {
		sw16(b, o+offs[1], num)
	}
	if offs[2] != 0 {
		swapNtab(b, o+offs[2])
	}
	if offs[3] != 0 {
		swRange(b, o+offs[3], o+ends[3], 4)
	}
	if offs[4] != 0 {
		swRange(b, o+offs[4], o+ends[4], 2)
	}
	if offs[6] != 0 {
		swapRecords(b, o+offs[6], o+ends[6], shapeMtx)
	}
	if offs[7] != 0 {
		swRange(b, o+offs[7], o+ends[7], 4)
	}
	sw16(b, o+0x08, 1)
	sw32(b, o+0x0C, 8)
}

func blkTEX1(b []byte, o, end int) {
	num := u16(b, o+0x08)
	res, name := u32(b, o+0x0C), u32(b, o+0x10)
	if res != 0 {
		swapRecords(b, o+res, o+res+num*timg.size, timg)
	}
	if name != 0 {
		swapNtab(b, o+name)
	}
	sw16(b, o+0x08, 1)
	sw32(b, o+0x0C, 2)
}

var modelBlocks = map[string]func([]byte, int, int){
	"INF1": blkINF1, "VTX1": blkVTX1, "EVP1": blkEVP1, "DRW1": blkDRW1, "JNT1": blkJNT1, "MAT3": blkMAT3,
	"MDL3": blkMDL3, "SHP1": blkSHP1, "TEX1": blkTEX1,
}

func swapModel(b []byte) {
	nblocks := u32(b, 0x0C)
	sw32(b, 0, 4)
	o := 0x20
	for i := 0; i < nblocks; i++ {
		if o+8 > len(b) {
			break
		}
		t, size := tag(b, o, 4), u32(b, o+4)
		if fn := modelBlocks[t]; fn != nil {
			fn(b, o, o+size)
		}
		sw32(b, o, 2)
		if size <= 0 {
			break
		}
		o += size
	}
}

// ---- animations ----

// ank1CountLimit and ank1Marker: a key count of 0x8000 or more is unsigned (galaxy-vita 1.2). Read as signed it came
// out negative and that array stayed big-endian (Bowser's battle-start cutscenes, among others, twisted and spun).
// Such a block gets the marker in its header padding, so the game knows it needs no repair at load.
const ank1CountLimit = 0x8000

var ank1Marker = []byte("UCNT")

func ank1(b []byte, o, end int) {
	nS, nR, nT := u16(b, o+0x0E), u16(b, o+0x10), u16(b, o+0x12)
	tbl, sc, rot, tr := u32(b, o+0x14), u32(b, o+0x18), u32(b, o+0x1C), u32(b, o+0x20)
	if (nS >= ank1CountLimit || nR >= ank1CountLimit || nT >= ank1CountLimit) && tbl >= 0x28 {
		need(b, o+0x24, 4)
		copy(b[o+0x24:o+0x28], ank1Marker)
	}
	if tbl != 0 {
		stop := end
		if sc != 0 {
			stop = o + sc
		}
		swRange(b, o+tbl, stop, 2)
	}
	if sc != 0 {
		sw32(b, o+sc, nS)
	}
	if rot != 0 {
		sw16(b, o+rot, nR)
	}
	if tr != 0 {
		sw32(b, o+tr, nT)
	}
	sw16(b, o+0x0A, 5)
	sw32(b, o+0x14, 4)
}

func anf1(b []byte, o, end int) {
	anmTables(b, o, end, 0x14, []tkind{w(2), w(4), w(2), w(4)})
	sw16(b, o+0x0A, 5)
}

func ttk1(b []byte, o, end int) {
	kinds := []tkind{w(2), w(2), ntab, skip, w(4), w(4), w(2), w(4)}
	offs := make([]int, 16)
	for g, base := range []int{0x14, 0x3C} {
		for i := 0; i < 8; i++ {
			offs[g*8+i] = u32(b, o+base+4*i)
		}
	}
	ends := extents(offs, end-o)
	for j, off := range offs {
		k := kinds[j%8]
		if off == 0 || k == skip {
			continue
		}
		applyKind(b, o+off, o+ends[j], k)
	}
	sw32(b, o+0x14, 8)
	sw32(b, o+0x3C, 8)
	sw16(b, o+0x0A, 5)
	sw16(b, o+0x34, 4)
	sw32(b, o+0x5C, 1)
}

func trk1(b []byte, o, end int) {
	anmTables(b, o, end, 0x20, []tkind{rec(regTable), rec(regTable), w(2), w(2), ntab, ntab, w(2), w(2), w(2), w(2), w(2), w(2), w(2), w(2)})
	sw16(b, o+0x0A, 11)
}

func tpt1(b []byte, o, end int) {
	anmTables(b, o, end, 0x10, []tkind{rec(tptTable), w(2), w(2), ntab})
	sw16(b, o+0x0A, 3)
}

func vaf1(b []byte, o, end int) {
	anmTables(b, o, end, 0x10, []tkind{w(2), skip})
	sw16(b, o+0x0A, 3)
}

func pak1(b []byte, o, end int) {
	anmTables(b, o, end, 0x18, []tkind{w(2), w(2), ntab, w(2), w(2), w(2), w(2)})
	sw16(b, o+0x0C, 6)
}

func paf1(b []byte, o, end int) {
	anmTables(b, o, end, 0x18, []tkind{w(2), w(2), ntab, skip, skip, skip, skip})
	sw16(b, o+0x0C, 6)
}

func clk1(b []byte, o, end int) {
	anmTables(b, o, end, 0x10, []tkind{w(2), w(4)})
	sw16(b, o+0x0A, 1)
	sw32(b, o+0x0C, 1)
}

var animBlocks = map[string]func([]byte, int, int){
	"ANK1": ank1, "ANF1": anf1, "TTK1": ttk1, "TRK1": trk1, "TPT1": tpt1, "VAF1": vaf1, "PAK1": pak1, "PAF1": paf1,
	"CLK1": clk1, "CLF1": clk1,
}

// swapSoundAnim converts a JAUSoundAnimation at o.
func swapSoundAnim(b []byte, o int) {
	if o < 0 || o+8 > len(b) {
		return
	}
	num := u16(b, o)
	sw16(b, o, 1)
	sw32(b, o+4, 1)
	p := o + 8
	for i := 0; i < num; i++ {
		if p+0x20 > len(b) {
			break
		}
		sw32(b, p, 5)
		sw32(b, p+0x1C, 1)
		p += 0x20
	}
}

func swapAnim(b []byte) {
	se := u32(b, 0x1C)
	nblocks := u32(b, 0x0C)
	sw32(b, 0, 4)
	sw32(b, 0x1C, 1)
	o := 0x20
	for i := 0; i < nblocks; i++ {
		if o+8 > len(b) {
			break
		}
		t, size := tag(b, o, 4), u32(b, o+4)
		if fn := animBlocks[t]; fn != nil {
			fn(b, o, o+size)
		}
		sw32(b, o, 2)
		if size <= 0 {
			break
		}
		o += size
	}
	if se > 0 && se < len(b) {
		swapSoundAnim(b, se)
	}
}
