package galaxy

import (
	"bytes"
	"encoding/binary"
	"strings"
)

// The port's executable leaves out three pieces of data that sit in the
// original main.dol: the error message archive and the StoryEvent and
// GalaxyID tables. They are taken from the player's own main.dol.

const errorWindow = "layoutdata/errormessagewindow.arc"

type dolTable struct {
	name    string
	columns int
	wanted  []string
}

var dolTables = []dolTable{
	{"StoryEvent", 2, []string{"name", "progress"}},
	{"GalaxyID", 7, []string{"name", "MapPaneName", "PowerStarNum"}},
}

func jmapHash(name string) uint32 {
	var h uint32
	for _, c := range []byte(name) {
		v := int32(c)
		if c >= 128 {
			v -= 256
		}
		h = uint32(v) + h*31
	}
	return h
}

func safe(fn func()) (ok bool) {
	defer func() {
		if recover() != nil {
			ok = false
		}
	}()
	fn()
	return true
}

func findErrorArchive(dol []byte) []byte {
	pos := bytes.Index(dol, []byte("RARC"))
	for pos >= 0 {
		if pos+8 <= len(dol) {
			size := int(be.Uint32(dol[pos+4:]))
			if size >= 0x40 && size <= len(dol)-pos {
				var names []string
				if !safe(func() {
					paths, _ := rarcFiles(dol[pos : pos+size])
					for _, p := range paths {
						p = strings.ToLower(p)
						if i := strings.IndexByte(p, '/'); i >= 0 {
							p = p[i+1:]
						}
						names = append(names, p)
					}
				}) {
					names = nil
				}
				for _, n := range names {
					if n == errorWindow {
						return append([]byte(nil), dol[pos:pos+size]...)
					}
				}
			}
		}
		next := bytes.Index(dol[pos+4:], []byte("RARC"))
		if next < 0 {
			break
		}
		pos += 4 + next
	}
	return nil
}

func tableAt(dol []byte, pos int, t dolTable, wanted map[uint32]bool) []byte {
	if pos < 0 || pos+16+12*t.columns > len(dol) {
		return nil
	}
	rows, fields, data, entry := int(int32(be.Uint32(dol[pos:]))), int(int32(be.Uint32(dol[pos+4:]))), int(int32(be.Uint32(dol[pos+8:]))), int(int32(be.Uint32(dol[pos+12:])))
	if fields != t.columns || data != 16+12*fields || !(rows > 0 && rows < 4096) || !(entry > 0 && entry <= 1024) {
		return nil
	}
	type desc struct {
		hash   uint32
		offset int
		kind   byte
	}
	descs := make([]desc, fields)
	have := map[uint32]bool{}
	for i := range descs {
		p := pos + 16 + 12*i
		descs[i] = desc{be.Uint32(dol[p:]), int(be.Uint16(dol[p+8:])), dol[p+11]}
		have[descs[i].hash] = true
	}
	for h := range wanted {
		if !have[h] {
			return nil
		}
	}
	strs := pos + data + rows*entry
	end := strs
	var result []byte
	ok := safe(func() {
		for row := 0; row < rows; row++ {
			for _, d := range descs {
				if d.kind == 6 {
					at := strs + u32(dol, pos+data+row*entry+d.offset)
					if at < 0 || at > len(dol) {
						panic(oob{at, 1, len(dol)})
					}
					z := bytes.IndexByte(dol[at:], 0)
					if z < 0 {
						panic(oob{at, 1, len(dol)})
					}
					if at+z+1 > end {
						end = at + z + 1
					}
				}
			}
		}
		e := pos + ((end - pos + 31) &^ 31)
		if e > len(dol) {
			e = len(dol)
		}
		result = append([]byte(nil), dol[pos:e]...)
	})
	if !ok {
		panic(oob{pos, 0, len(dol)}) // the reference fails the whole step here too
	}
	return result
}

func findTable(dol []byte, t dolTable) []byte {
	wanted := map[uint32]bool{}
	for _, n := range t.wanted {
		wanted[jmapHash(n)] = true
	}
	var key [4]byte
	binary.BigEndian.PutUint32(key[:], jmapHash(t.wanted[0]))
	hit := bytes.Index(dol, key[:])
	for hit >= 0 {
		for i := 0; i < t.columns; i++ {
			if tb := tableAt(dol, hit-16-12*i, t, wanted); tb != nil {
				return tb
			}
		}
		next := bytes.Index(dol[hit+1:], key[:])
		if next < 0 {
			break
		}
		hit += 1 + next
	}
	return nil
}
