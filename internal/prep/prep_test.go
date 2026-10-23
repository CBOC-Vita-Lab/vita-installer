package prep

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

func TestIdentifyImages(t *testing.T) {
	dir := t.TempDir()
	iso := make([]byte, 0x400)
	copy(iso, "R4QE01\x00\x01")
	binary.BigEndian.PutUint32(iso[0x18:], 0x5D1C9EA3)
	os.WriteFile(filepath.Join(dir, "a.iso"), iso, 0o644)
	rvz := make([]byte, 0x400)
	copy(rvz, "RVZ\x01")
	copy(rvz[0x58:], "RMGE01\x00\x00")
	os.WriteFile(filepath.Join(dir, "b.rvz"), rvz, 0o644)
	for name, want := range map[string]string{"a.iso": "R4QE01", "b.rvz": "RMGE01"} {
		d, err := IdentifyDisc(filepath.Join(dir, name))
		if err != nil || d.GameID != want {
			t.Fatalf("%s: %v %+v", name, err, d)
		}
	}
	rev := 1
	d, _ := IdentifyDisc(filepath.Join(dir, "a.iso"))
	if CheckDisc(d, "R4QE01", &rev) != nil {
		t.Fatal("rev 1 rejected")
	}
	rev = 0
	if CheckDisc(d, "R4QE01", &rev) == nil {
		t.Fatal("wrong revision accepted")
	}
	os.WriteFile(filepath.Join(dir, "junk.iso"), make([]byte, 0x400), 0o644)
	if _, err := IdentifyDisc(filepath.Join(dir, "junk.iso")); err == nil {
		t.Fatal("junk accepted")
	}
}

func TestPSPGameID(t *testing.T) {
	const s = 2048
	img := make([]byte, 24*s)
	pvd := img[16*s:]
	pvd[0] = 1
	copy(pvd[1:], "CD001")
	root := pvd[156:]
	root[0] = 34
	binary.LittleEndian.PutUint32(root[2:], 20)
	binary.LittleEndian.PutUint32(root[10:], s)
	dir := img[20*s:]
	rec := dir[0:]
	name := "UMD_DATA.BIN;1"
	rec[0] = byte(33 + len(name) + 1)
	binary.LittleEndian.PutUint32(rec[2:], 22)
	rec[32] = byte(len(name))
	copy(rec[33:], name)
	copy(img[22*s:], "ULUS-10054|0000000000000001|0001|G")
	p := filepath.Join(t.TempDir(), "g.iso")
	os.WriteFile(p, img, 0o644)
	id, err := PSPGameID(p)
	if err != nil || id != "ULUS-10054" {
		t.Fatalf("%q %v", id, err)
	}
}
