package release

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"

	"github.com/CBOC-Vita-Lab/vita-installer/internal/fakevita"
)

func TestLoadZipWithSeveralApps(t *testing.T) {
	dir := t.TempDir()
	a, b := filepath.Join(dir, "a.vpk"), filepath.Join(dir, "b.vpk")
	fakevita.MakeVPK(a, "AAAA00001", "Game A", nil)
	fakevita.MakeVPK(b, "BBBB00001", "Game B", nil)
	zp := filepath.Join(dir, "pack.zip")
	f, _ := os.Create(zp)
	z := zip.NewWriter(f)
	for name, src := range map[string]string{"pack/a.vpk": a, "pack/b.vpk": b} {
		w, _ := z.Create(name)
		data, _ := os.ReadFile(src)
		w.Write(data)
	}
	w, _ := z.Create("pack/data/gamea/config.txt")
	w.Write([]byte("x"))
	w, _ = z.Create("pack/ux0/tai/config.txt")
	w.Write([]byte("never copied"))
	z.Close()
	f.Close()
	rs, err := Load(zp, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if len(rs) != 2 {
		t.Fatalf("got %d", len(rs))
	}
	if len(rs[0].Copies) != 1 || rs[0].Copies[0].Remote != "ux0:data/gamea/config.txt" {
		t.Fatalf("copies %+v", rs[0].Copies)
	}
}

func TestVPKNamedZip(t *testing.T) {
	p := filepath.Join(t.TempDir(), "game.zip")
	fakevita.MakeVPK(p, "ZIPV00001", "Zipped", nil)
	rs, err := Load(p, t.TempDir())
	if err != nil || len(rs) != 1 || rs[0].TitleID != "ZIPV00001" {
		t.Fatalf("%v %+v", err, rs)
	}
}

func TestBadManifest(t *testing.T) {
	dir := t.TempDir()
	fakevita.MakeVPK(filepath.Join(dir, "a.vpk"), "AAAA00001", "A", nil)
	os.WriteFile(filepath.Join(dir, ManifestName), []byte(`{"needs":[{"type":"file","to":"ux0:../../x"}]}`), 0o644)
	if _, err := Load(dir, t.TempDir()); err == nil {
		t.Fatal("unsafe manifest accepted")
	}
}
