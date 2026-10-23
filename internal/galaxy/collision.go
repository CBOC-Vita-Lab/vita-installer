package galaxy

// Collision rebuilt from a model's own triangles for the planets whose
// original collision is a stand-in mesh (see collision.py in the port).
//
// The arithmetic follows the reference step by step; every product goes
// through f() so the compiler cannot fuse a multiply and an add (which
// would change the last bit on ARM and break the byte-exact output).

import (
	"errors"
	"math"
)

// fromModel maps an archive (relative to files/) to the model inside it.
var fromModel = map[string]string{
	"ObjectData/MoonPlanet.arc":                 "moonplanet",
	"ObjectData/FullMoonPlanet.arc":             "fullmoonplanet",
	"ObjectData/HeavensDoorBlackHolePlanet.arc": "heavensdoorblackholeplanet",
}

type vec3 [3]float64

func f(x float64) float64 { return x }

func mul(a, b float64) float64 { return f(a * b) }

func sub3(a, b vec3) vec3 { return vec3{a[0] - b[0], a[1] - b[1], a[2] - b[2]} }
func cross3(a, b vec3) vec3 {
	return vec3{mul(a[1], b[2]) - mul(a[2], b[1]), mul(a[2], b[0]) - mul(a[0], b[2]), mul(a[0], b[1]) - mul(a[1], b[0])}
}
func dot3(a, b vec3) float64 { return f(f(mul(a[0], b[0])+mul(a[1], b[1])) + mul(a[2], b[2])) }
func normalize3(a vec3) vec3 {
	n := math.Sqrt(dot3(a, a))
	return vec3{a[0] / n, a[1] / n, a[2] / n}
}
func f32v(v vec3) vec3 {
	return vec3{float64(float32(v[0])), float64(float32(v[1])), float64(float32(v[2]))}
}

func bef32(b []byte, o int) float64 { return float64(math.Float32frombits(u32x(b, o))) }

type mat34 [3][4]float64

var identity = mat34{{1, 0, 0, 0}, {0, 1, 0, 0}, {0, 0, 1, 0}}

func jointMatrix(sx, sy, sz float64, rx, ry, rz int, tx, ty, tz float64) mat34 {
	ax, ay, az := float64(rx)*math.Pi/32768.0, float64(ry)*math.Pi/32768.0, float64(rz)*math.Pi/32768.0
	cx, snx, cy, sny, cz, snz := math.Cos(ax), math.Sin(ax), math.Cos(ay), math.Sin(ay), math.Cos(az), math.Sin(az)
	r := [3][3]float64{
		{mul(cy, cz), mul(mul(snx, sny), cz) - mul(cx, snz), mul(mul(cx, sny), cz) + mul(snx, snz)},
		{mul(cy, snz), mul(mul(snx, sny), snz) + mul(cx, cz), mul(mul(cx, sny), snz) - mul(snx, cz)},
		{-sny, mul(snx, cy), mul(cx, cy)},
	}
	s, t := [3]float64{sx, sy, sz}, [3]float64{tx, ty, tz}
	var m mat34
	for i := 0; i < 3; i++ {
		m[i] = [4]float64{mul(r[i][0], s[0]), mul(r[i][1], s[1]), mul(r[i][2], s[2]), t[i]}
	}
	return m
}

func concat(a, b mat34) mat34 {
	var m mat34
	for i := 0; i < 3; i++ {
		for j := 0; j < 4; j++ {
			extra := 0.0
			if j == 3 {
				extra = a[i][3]
			}
			m[i][j] = f(f(f(mul(a[i][0], b[0][j])+mul(a[i][1], b[1][j]))+mul(a[i][2], b[2][j])) + extra)
		}
	}
	return m
}

func apply(m mat34, p vec3) vec3 {
	var v vec3
	for i := 0; i < 3; i++ {
		v[i] = f(f(f(mul(m[i][0], p[0])+mul(m[i][1], p[1]))+mul(m[i][2], p[2])) + m[i][3])
	}
	return v
}

func j3dBlocks(b []byte) map[string]int {
	out := map[string]int{}
	off := 0x20
	for off+8 <= len(b) {
		size := u32(b, off+4)
		if size == 0 {
			break
		}
		out[tag(b, off, 4)] = off
		off += size
	}
	return out
}

func blockAt(blk map[string]int, name string) int {
	o, ok := blk[name]
	if !ok {
		panic(errors.New("model has no " + name + " block"))
	}
	return o
}

func jointWorld(b []byte, blk map[string]int) []mat34 {
	o := blockAt(blk, "JNT1")
	num := u16(b, o+8)
	init, index := u32(b, o+0x0C), u32(b, o+0x10)
	local := make([]mat34, num)
	for j := 0; j < num; j++ {
		rec := o + init + 0x40*u16(b, o+index+2*j)
		local[j] = jointMatrix(bef32(b, rec+4), bef32(b, rec+8), bef32(b, rec+12),
			s16(b, rec+0x10), s16(b, rec+0x12), s16(b, rec+0x14),
			bef32(b, rec+0x18), bef32(b, rec+0x1C), bef32(b, rec+0x20))
	}
	inf := blockAt(blk, "INF1")
	p := inf + u32(b, inf+0x14)
	world := make([]mat34, num)
	for i := range world {
		world[i] = identity
	}
	parents, last := []mat34{identity}, identity
	for {
		kind, value := u16(b, p), u16(b, p+2)
		p += 4
		if kind == 0 {
			break
		}
		switch kind {
		case 1:
			parents = append(parents, last)
		case 2:
			if len(parents) == 0 {
				panic(errors.New("bad joint hierarchy"))
			}
			parents = parents[:len(parents)-1]
		case 0x10:
			if value >= num || len(parents) == 0 {
				panic(errors.New("bad joint hierarchy"))
			}
			world[value] = concat(parents[len(parents)-1], local[value])
			last = world[value]
		}
	}
	return world
}

type tri [3]vec3

// modelTriangles returns the rigid model's triangles in model space, wound
// so cross(v1-v0, v2-v0) points out of the front face.
func modelTriangles(b []byte) []tri {
	blk := j3dBlocks(b)
	vtx := blockAt(blk, "VTX1")
	p := vtx + u32(b, vtx+8)
	comp, frac, found := 0, 0, false
	for {
		attr := u32(b, p)
		c, fr := u32(b, p+8), u8(b, p+12)
		if attr == 0xFF {
			break
		}
		if attr == 9 {
			comp, frac, found = c, fr, true
		}
		p += 16
	}
	if !found {
		panic(errors.New("model has no position format"))
	}
	posBase := vtx + u32(b, vtx+0x0C)
	width := map[int]int{0: 1, 1: 1, 2: 2, 3: 2, 4: 4}[comp]
	if width == 0 {
		panic(errors.New("unknown position format"))
	}
	scale := 1.0
	if comp != 4 {
		scale = 1.0 / float64(int(1)<<uint(frac))
	}
	position := func(i int) vec3 {
		var v vec3
		for k := 0; k < 3; k++ {
			o := posBase + (i*3+k)*width
			var x float64
			switch comp {
			case 0:
				x = float64(u8(b, o))
			case 1:
				x = float64(int8(u8(b, o)))
			case 2:
				x = float64(u16(b, o))
			case 3:
				x = float64(s16(b, o))
			case 4:
				x = bef32(b, o)
			}
			v[k] = mul(x, scale)
		}
		return v
	}
	world := jointWorld(b, blk)
	drw := blockAt(blk, "DRW1")
	drwFlag, drwIndex := u32(b, drw+0x0C), u32(b, drw+0x10)
	shp := blockAt(blk, "SHP1")
	count := u16(b, shp+8)
	oInit, oDesc, oTable, oDL, oMtx, oDraw := u32(b, shp+0x0C), u32(b, shp+0x18), u32(b, shp+0x1C), u32(b, shp+0x20), u32(b, shp+0x24), u32(b, shp+0x28)
	var tris []tri
	for s := 0; s < count; s++ {
		rec := shp + oInit + 0x28*s
		groups, desc, mtxFirst, drawFirst := u16(b, rec+2), u16(b, rec+4), u16(b, rec+6), u16(b, rec+8)
		type ak struct{ attr, kind int }
		var attrs []ak
		q := shp + oDesc + desc
		for {
			attr, kind := u32(b, q), u32(b, q+4)
			q += 8
			if attr == 0xFF {
				break
			}
			attrs = append(attrs, ak{attr, kind})
		}
		for g := 0; g < groups; g++ {
			m := shp + oMtx + 8*(mtxFirst+g)
			use, useCount, first := u16(b, m), u16(b, m+2), u32(b, m+4)
			dr := shp + oDraw + 8*(drawFirst+g)
			size, start := u32(b, dr), u32(b, dr+4)
			table := make([]int, useCount)
			for k := range table {
				table[k] = u16(b, shp+oTable+2*(first+k))
			}
			matrix := func(slot int) mat34 {
				d := use
				if slot < len(table) && table[slot] != 0xFFFF {
					d = table[slot]
				}
				if u8(b, drw+drwFlag+d) != 0 {
					panic(errors.New("skinned shape"))
				}
				wi := u16(b, drw+drwIndex+2*d)
				if wi >= len(world) {
					panic(errors.New("bad joint index"))
				}
				return world[wi]
			}
			d := shp + oDL + start
			end := d + size
			for d < end {
				cmd := u8(b, d)
				d++
				if cmd == 0 {
					continue
				}
				prim := cmd & 0xF8
				if prim != 0x90 && prim != 0x98 && prim != 0xA0 {
					break
				}
				n := u16(b, d)
				d += 2
				verts := make([]vec3, 0, n)
				for v := 0; v < n; v++ {
					slot, index := 0, 0
					for _, a := range attrs {
						if a.attr <= 8 {
							if a.attr == 0 {
								slot = u8(b, d) / 3
							}
							d++
							continue
						}
						var val int
						switch a.kind {
						case 2:
							val = u8(b, d)
							d++
						case 3:
							val = u16(b, d)
							d += 2
						case 1:
							panic(errors.New("direct vertex attribute"))
						default:
							continue
						}
						if a.attr == 9 {
							index = val
						}
					}
					verts = append(verts, apply(matrix(slot), position(index)))
				}
				var faces [][3]int
				switch prim {
				case 0x90:
					for k := 0; k < n-2; k += 3 {
						faces = append(faces, [3]int{k, k + 1, k + 2})
					}
				case 0x98:
					for k := 0; k < n-2; k++ {
						if k&1 != 0 {
							faces = append(faces, [3]int{k + 1, k, k + 2})
						} else {
							faces = append(faces, [3]int{k, k + 1, k + 2})
						}
					}
				default:
					for k := 1; k < n-1; k++ {
						faces = append(faces, [3]int{0, k, k + 1})
					}
				}
				for _, fc := range faces {
					tris = append(tris, tri{verts[fc[0]], verts[fc[2]], verts[fc[1]]})
				}
			}
		}
	}
	return tris
}

type kclTri struct {
	t   tri
	row int
}

// kclTriangles rebuilds every prism of a big-endian KCL as the game does.
func kclTriangles(b []byte) []kclTri {
	posOff, nrmOff, prismOff, octreeOff := u32(b, 0), u32(b, 4), u32(b, 8), u32(b, 12)
	vec := func(off, i int) vec3 { return vec3{bef32(b, off+12*i), bef32(b, off+12*i+4), bef32(b, off+12*i+8)} }
	var out []kclTri
	for i := 1; i < floorDiv(octreeOff-prismOff, 16); i++ {
		r := prismOff + 16*i
		height := bef32(b, r)
		p, n, e0, e1, e2, attribute := u16(b, r+4), u16(b, r+6), u16(b, r+8), u16(b, r+10), u16(b, r+12), u16(b, r+14)
		a, fn := vec(posOff, p), vec(nrmOff, n)
		en0, en1, en2 := vec(nrmOff, e0), vec(nrmOff, e1), vec(nrmOff, e2)
		d1, d2 := cross3(fn, en1), cross3(en0, fn)
		q1, q2 := dot3(d1, en2), dot3(d2, en2)
		v1, v2 := a, a
		if q1 != 0 && q2 != 0 {
			for k := 0; k < 3; k++ {
				v1[k] = f(a[k] + f(mul(d1[k], height)/q1))
				v2[k] = f(a[k] + f(mul(d2[k], height)/q2))
			}
		}
		out = append(out, kclTri{tri{a, v1, v2}, attribute})
	}
	return out
}

func prismBox(t tri, normal vec3, depth float64, center, half vec3) bool {
	var pts [6]vec3
	for i := 0; i < 3; i++ {
		pts[i] = sub3(t[i], center)
	}
	for i := 0; i < 3; i++ {
		p := pts[i]
		pts[3+i] = vec3{p[0] - mul(normal[0], depth), p[1] - mul(normal[1], depth), p[2] - mul(normal[2], depth)}
	}
	edges := [4]vec3{sub3(t[1], t[0]), sub3(t[2], t[1]), sub3(t[0], t[2]), normal}
	axes := make([]vec3, 0, 19)
	axes = append(axes, vec3{1, 0, 0}, vec3{0, 1, 0}, vec3{0, 0, 1}, normal)
	for _, e := range edges[:3] {
		axes = append(axes, cross3(e, normal))
	}
	for _, e := range edges {
		axes = append(axes, vec3{0, -e[2], e[1]}, vec3{e[2], 0, -e[0]}, vec3{-e[1], e[0], 0})
	}
	for _, ax := range axes {
		r := f(f(mul(half[0], math.Abs(ax[0]))+mul(half[1], math.Abs(ax[1]))) + mul(half[2], math.Abs(ax[2])))
		lo, hi := math.Inf(1), math.Inf(-1)
		for _, p := range pts {
			d := dot3(p, ax)
			if d < lo {
				lo = d
			}
			if d > hi {
				hi = d
			}
		}
		if lo > r || hi < -r {
			return false
		}
	}
	return true
}

func f32key(v vec3) [3]uint32 {
	return [3]uint32{math.Float32bits(float32(v[0])), math.Float32bits(float32(v[1])), math.Float32bits(float32(v[2]))}
}

// buildKCL writes a big-endian KCL for triangles facing along cross(b-a, c-a).
func buildKCL(tris []tri, thickness float64) ([]byte, []int) {
	const maxPerLeaf, minWidth = 16, 32
	var positions, normals []vec3
	posIndex, nrmIndex := map[[3]uint32]int{}, map[[3]uint32]int{}
	intern := func(table *[]vec3, index map[[3]uint32]int, v vec3) int {
		k := f32key(v)
		if i, ok := index[k]; ok {
			return i
		}
		index[k] = len(*table)
		*table = append(*table, v)
		return index[k]
	}
	type prism struct {
		height           float64
		p, n, e0, e1, e2 int
	}
	var prisms []prism
	var kept []int
	var shapes []tri
	var normalsOf []vec3
	for i, t := range tris {
		a, b, c := f32v(t[0]), f32v(t[1]), f32v(t[2])
		n := cross3(sub3(b, a), sub3(c, a))
		if dot3(n, n) < 1e-6 {
			continue
		}
		n = normalize3(n)
		e0 := normalize3(cross3(n, sub3(c, a)))
		e1 := normalize3(cross3(sub3(b, a), n))
		e2 := normalize3(cross3(sub3(c, b), n))
		height := dot3(sub3(b, a), e2)
		if height < 0.01 {
			continue
		}
		pi := intern(&positions, posIndex, a)
		ni := intern(&normals, nrmIndex, n)
		e0i := intern(&normals, nrmIndex, e0)
		e1i := intern(&normals, nrmIndex, e1)
		e2i := intern(&normals, nrmIndex, e2)
		prisms = append(prisms, prism{height, pi, ni, e0i, e1i, e2i})
		kept = append(kept, i)
		shapes = append(shapes, tri{a, b, c})
		normalsOf = append(normalsOf, n)
	}
	if len(positions) > 0xFFFF || len(normals) > 0xFFFF || len(prisms) > 0xFFFE {
		panic(errors.New("too many triangles for a KCL"))
	}
	lo := [3]float64{math.Inf(1), math.Inf(1), math.Inf(1)}
	hi := [3]float64{math.Inf(-1), math.Inf(-1), math.Inf(-1)}
	for si, t := range shapes {
		n := normalsOf[si]
		var pts []vec3
		for _, q := range t {
			pts = append(pts, q)
		}
		for _, q := range t {
			pts = append(pts, vec3{q[0] - mul(n[0], thickness), q[1] - mul(n[1], thickness), q[2] - mul(n[2], thickness)})
		}
		for _, p := range pts {
			for k := 0; k < 3; k++ {
				if p[k] < lo[k] {
					lo[k] = p[k]
				}
				if p[k] > hi[k] {
					hi[k] = p[k]
				}
			}
		}
	}
	var origin, bits [3]int
	for k := 0; k < 3; k++ {
		lo[k] -= 8.0
		hi[k] += 8.0
		origin[k] = int(math.Floor(lo[k]))
		span := hi[k] - float64(origin[k])
		if span < 1.0 {
			span = 1.0
		}
		bits[k] = int(math.Ceil(math.Log2(span)))
		if bits[k] < 1 {
			bits[k] = 1
		}
	}
	minBits := bits[0]
	for _, v := range bits {
		if v < minBits {
			minBits = v
		}
	}
	shift := 5 // log2(minWidth)
	if minBits-2 > shift {
		shift = minBits - 2
	}
	if minBits < shift {
		shift = minBits
	}
	var counts [3]int
	for k := 0; k < 3; k++ {
		counts[k] = 1 << uint(bits[k]-shift)
	}

	type word struct {
		node   bool
		target int
	}
	var blocks [][]word
	var lists [][]int
	listIndex := map[string]int{}
	leaf := func(members []int) word {
		key := make([]byte, 0, 4*len(members))
		for _, m := range members {
			key = append(key, byte(m), byte(m>>8), byte(m>>16), byte(m>>24))
		}
		if i, ok := listIndex[string(key)]; ok {
			return word{false, i}
		}
		listIndex[string(key)] = len(lists)
		lists = append(lists, members)
		return word{false, len(lists) - 1}
	}
	var cell func(x, y, z, width int, members []int) word
	cell = func(x, y, z, width int, members []int) word {
		half := float64(width)/2.0 + 1.0
		center := vec3{float64(origin[0]+x) + float64(width)/2.0, float64(origin[1]+y) + float64(width)/2.0, float64(origin[2]+z) + float64(width)/2.0}
		var inside []int
		for _, m := range members {
			if prismBox(shapes[m], normalsOf[m], thickness, center, vec3{half, half, half}) {
				inside = append(inside, m)
			}
		}
		if len(inside) <= maxPerLeaf || width <= minWidth {
			return leaf(inside)
		}
		blk := len(blocks)
		blocks = append(blocks, nil)
		w := width / 2
		children := make([]word, 8)
		for i := 0; i < 8; i++ {
			children[i] = cell(x+(i&1)*w, y+((i>>1)&1)*w, z+((i>>2)&1)*w, w, inside)
		}
		blocks[blk] = children
		return word{true, blk}
	}
	everyone := make([]int, len(shapes))
	for i := range everyone {
		everyone[i] = i
	}
	width := 1 << uint(shift)
	blocks = append(blocks, nil)
	var root []word
	for iz := 0; iz < counts[2]; iz++ {
		for iy := 0; iy < counts[1]; iy++ {
			for ix := 0; ix < counts[0]; ix++ {
				root = append(root, cell(ix*width, iy*width, iz*width, width, everyone))
			}
		}
	}
	blocks[0] = root

	offsets := make([]int, len(blocks))
	at := 0
	for i, ws := range blocks {
		offsets[i] = at
		at += 4 * len(ws)
	}
	listOffsets := make([]int, len(lists))
	for i, m := range lists {
		listOffsets[i] = at
		at += 2 * (len(m) + 2)
	}
	octree := make([]byte, at)
	for bi, ws := range blocks {
		base := offsets[bi]
		for wi, wd := range ws {
			var v uint32
			if wd.node {
				v = uint32(offsets[wd.target] - base)
			} else {
				v = 0x80000000 | uint32(listOffsets[wd.target]-base)
			}
			be.PutUint32(octree[base+4*wi:], v)
		}
	}
	for li, m := range lists {
		o := listOffsets[li]
		be.PutUint16(octree[o:], 0)
		for k, x := range m {
			be.PutUint16(octree[o+2+2*k:], uint16(x+1))
		}
		be.PutUint16(octree[o+2+2*len(m):], 0)
	}
	posOff := kclHeader
	nrmOff := posOff + 12*len(positions)
	prismOff := nrmOff + 12*len(normals) - 0x10
	octreeOff := prismOff + 0x10 + 16*len(prisms)
	out := make([]byte, octreeOff)
	put32 := func(o int, v uint32) { be.PutUint32(out[o:], v) }
	putf := func(o int, v float64) { put32(o, math.Float32bits(float32(v))) }
	put32(0, uint32(posOff))
	put32(4, uint32(nrmOff))
	put32(8, uint32(prismOff))
	put32(12, uint32(octreeOff))
	putf(16, thickness)
	putf(20, float64(origin[0]))
	putf(24, float64(origin[1]))
	putf(28, float64(origin[2]))
	for k := 0; k < 3; k++ {
		put32(32+4*k, ^uint32((uint64(1)<<uint(bits[k]))-1))
	}
	put32(44, uint32(shift))
	xshift := bits[0] - shift
	put32(48, uint32(xshift))
	put32(52, uint32(xshift+bits[1]-shift))
	for i, p := range positions {
		for k := 0; k < 3; k++ {
			putf(posOff+12*i+4*k, p[k])
		}
	}
	for i, n := range normals {
		for k := 0; k < 3; k++ {
			putf(nrmOff+12*i+4*k, n[k])
		}
	}
	for i, p := range prisms {
		o := prismOff + 16*(i+1)
		putf(o, p.height)
		for k, v := range []int{p.p, p.n, p.e0, p.e1, p.e2, i} {
			be.PutUint16(out[o+4+2*k:], uint16(v))
		}
	}
	return append(out, octree...), kept
}

// buildPA gives the attribute table one row per new triangle, copied from
// the old triangle whose centre is nearest.
func buildPA(oldPA []byte, oldTris []kclTri, newTris []tri) []byte {
	n, dataOff, entry := s32(oldPA, 0), s32(oldPA, 8), u32(oldPA, 12)
	if dataOff+n*entry > len(oldPA) || dataOff < 0 || n < 0 {
		panic(errors.New("bad .pa table"))
	}
	strs := oldPA[dataOff+n*entry:]
	table := make([][]byte, n)
	for i := range table {
		table[i] = oldPA[dataOff+i*entry : dataOff+(i+1)*entry]
	}
	rows := make([][]byte, len(oldTris))
	for i, t := range oldTris {
		if t.row >= n {
			panic(errors.New("KCL attribute beyond the .pa rows"))
		}
		rows[i] = table[t.row]
	}
	centre := func(t tri) vec3 {
		var c vec3
		for k := 0; k < 3; k++ {
			c[k] = f(f(t[0][k]+t[1][k])+t[2][k]) / 3.0
		}
		return c
	}
	const cellSize = 256.0
	type gkey [3]int
	type gitem struct {
		c vec3
		i int
	}
	grid := map[gkey][]gitem{}
	keyOf := func(c vec3) gkey {
		return gkey{int(math.Floor(c[0] / cellSize)), int(math.Floor(c[1] / cellSize)), int(math.Floor(c[2] / cellSize))}
	}
	for i, t := range oldTris {
		c := centre(t.t)
		k := keyOf(c)
		grid[k] = append(grid[k], gitem{c, i})
	}
	reach := 0
	if len(grid) > 0 {
		m := 0
		for k := range grid {
			for _, v := range k {
				if v < 0 {
					v = -v
				}
				if v > m {
					m = v
				}
			}
		}
		reach = m*2 + 2
	}
	abs := func(v int) int {
		if v < 0 {
			return -v
		}
		return v
	}
	nearest := func(c vec3) int {
		home := keyOf(c)
		best, bestD := 0, math.Inf(1)
		for ring := 0; ring <= reach; ring++ {
			for dx := -ring; dx <= ring; dx++ {
				for dy := -ring; dy <= ring; dy++ {
					for dz := -ring; dz <= ring; dz++ {
						m := abs(dx)
						if abs(dy) > m {
							m = abs(dy)
						}
						if abs(dz) > m {
							m = abs(dz)
						}
						if m != ring {
							continue
						}
						for _, it := range grid[gkey{home[0] + dx, home[1] + dy, home[2] + dz}] {
							d0, d1, d2 := it.c[0]-c[0], it.c[1]-c[1], it.c[2]-c[2]
							dd := f(f(mul(d0, d0)+mul(d1, d1)) + mul(d2, d2))
							if dd < bestD {
								best, bestD = it.i, dd
							}
						}
					}
				}
			}
			rr := float64(ring) * cellSize
			if bestD <= mul(rr, rr) {
				break
			}
		}
		return best
	}
	uniform := true
	for _, r := range rows {
		if string(r) != string(rows[0]) {
			uniform = false
			break
		}
	}
	out := append([]byte(nil), oldPA[:dataOff]...)
	be.PutUint32(out[0:], uint32(int32(len(newTris))))
	for _, t := range newTris {
		if uniform {
			out = append(out, rows[0]...)
		} else {
			out = append(out, rows[nearest(centre(t))]...)
		}
	}
	return append(out, strs...)
}

// rebuildCollision returns the new KCL and .pa (big-endian) for a model.
func rebuildCollision(files map[string][]byte, model string) map[string][]byte {
	prefix := model + "/" + model
	kclPath, paPath := prefix+".kcl", prefix+".pa"
	modelPath := ""
	for _, ext := range []string{".bdl", ".bmd"} {
		if _, ok := files[prefix+ext]; ok {
			modelPath = prefix + ext
			break
		}
	}
	if modelPath == "" || files[kclPath] == nil || files[paPath] == nil {
		panic(errors.New(model + ": model, KCL or .pa missing"))
	}
	oldKCL := files[kclPath]
	thickness := bef32(oldKCL, 0x10)
	tris := modelTriangles(files[modelPath])
	kcl, kept := buildKCL(tris, thickness)
	newTris := make([]tri, len(kept))
	for i, k := range kept {
		newTris[i] = tris[k]
	}
	pa := buildPA(files[paPath], kclTriangles(oldKCL), newTris)
	return map[string][]byte{kclPath: kcl, paPath: pa}
}
