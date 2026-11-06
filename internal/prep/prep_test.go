package prep

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/CBOC-Vita-Lab/vita-installer/internal/fakevita"
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

const bootPath = "PSP_GAME/SYSDIR/BOOT.BIN"

func writeISO(t *testing.T, data []byte) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "g.iso")
	if err := os.WriteFile(p, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// A file deep inside the image is found by its path and hashed.
func TestPSPFileSHA256(t *testing.T) {
	boot := []byte("the boot file of the US 1.02 version")
	p := writeISO(t, fakevita.PSPISOFiles("ULUS-10054", "", map[string][]byte{
		bootPath:                  boot,
		"PSP_GAME/PARAM.SFO":      []byte("sfo"),
		"PSP_GAME/USRDIR/EMPTY.X": nil,
	}))
	if id, err := PSPGameID(p); err != nil || id != "ULUS-10054" {
		t.Fatalf("game id: %q %v", id, err)
	}
	sum := sha256.Sum256(boot)
	got, err := PSPFileSHA256(p, bootPath)
	if err != nil || got != hex.EncodeToString(sum[:]) {
		t.Fatalf("hash: %q %v", got, err)
	}
	if got, err := PSPFileSHA256(p, "psp_game/sysdir/boot.bin"); err != nil || got == "" {
		t.Errorf("names are not case sensitive: %q %v", got, err)
	}
	if _, err := PSPFileSHA256(p, "PSP_GAME/SYSDIR/NOPE.BIN"); !errors.Is(err, ErrNotInISO) {
		t.Errorf("a missing file: %v", err)
	}
	if _, err := PSPFileSHA256(p, "PSP_GAME/SYSDIR"); !errors.Is(err, ErrNotInISO) {
		t.Errorf("a folder is not a file: %v", err)
	}
}

// A cut-off image, an image with a file past its end, and files that are not
// PSP images each give their own answer.
func TestPSPFileSHA256BadImages(t *testing.T) {
	img := fakevita.PSPISOFiles("ULUS-10054", "", map[string][]byte{bootPath: bytes.Repeat([]byte("B"), 5000)})
	// Cut short: the image says it is longer than the file.
	if _, err := PSPFileSHA256(writeISO(t, img[:len(img)-4096]), bootPath); err == nil || errors.Is(err, ErrNotPSPISO) {
		t.Errorf("a truncated image: %v", err)
	}
	if _, err := PSPGameID(writeISO(t, img[:len(img)-4096])); err == nil {
		t.Error("a truncated image gave a game ID")
	}
	// The directory is intact but the file's data is missing and the image
	// does not know its own size.
	blind := append([]byte(nil), img...)
	binary.LittleEndian.PutUint32(blind[16*2048+80:], 0)
	if _, err := PSPFileSHA256(writeISO(t, blind[:len(blind)-3*2048]), bootPath); !errors.Is(err, ErrDamagedISO) {
		t.Errorf("a file past the end: %v", err)
	}
	for name, data := range map[string][]byte{
		"cso": append([]byte("CISO"), make([]byte, 100000)...),
		"pbp": append([]byte("\x00PBP"), make([]byte, 100000)...),
	} {
		if _, err := PSPFileSHA256(writeISO(t, data), bootPath); !errors.Is(err, ErrNotPSPISO) {
			t.Errorf("%s: %v", name, err)
		}
	}
	if _, err := PSPFileSHA256(filepath.Join(t.TempDir(), "missing.iso"), bootPath); err == nil {
		t.Error("a missing file")
	}
}
