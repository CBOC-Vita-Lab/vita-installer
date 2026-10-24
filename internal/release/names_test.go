package release

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/CBOC-Vita-Lab/vita-installer/internal/fakevita"
)

// Release files are recognised by what they contain, not by their names.
func TestRenamedFilesAreFound(t *testing.T) {
	cbocJSON := func(vpkName, sha string) string {
		return fmt.Sprintf(`{"schemaVersion":1,"titleId":"MSCN00003","displayName":"Mario Strikers Charged","releaseState":"ready",
		"package":{"path":%q,"sha256":%q},"requiredInputs":[{"id":"game","kind":"strikers-disc"}],
		"destinations":{"data":"ux0:data/mscharged/game"}}`, vpkName, sha)
	}
	mk := func(t *testing.T) (string, string) {
		dir := t.TempDir()
		vpk := filepath.Join(dir, "x.vpk")
		if err := fakevita.MakeVPK(vpk, "MSCN00003", "Mario Strikers Charged", nil); err != nil {
			t.Fatal(err)
		}
		sum, _ := sha256File(vpk)
		return dir, sum
	}
	wantDisc := func(t *testing.T, rs []*Release, err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
		if len(rs) != 1 || rs[0].TitleID != "MSCN00003" || len(rs[0].Inputs) == 0 || rs[0].Inputs[0].Type != NeedWiiDisc {
			t.Fatalf("release not read from its release file: %+v", rs)
		}
	}

	t.Run("app renamed after its release file was written", func(t *testing.T) {
		dir, sum := mk(t)
		os.Rename(filepath.Join(dir, "x.vpk"), filepath.Join(dir, "Strikers-Charged-Vita.vpk"))
		os.WriteFile(filepath.Join(dir, "cboc-release.json"), []byte(cbocJSON("Strikers-settings-20261019.vpk", sum)), 0o644)
		rs, err := Load(dir, t.TempDir())
		wantDisc(t, rs, err)
	})
	t.Run("app with no .vpk extension", func(t *testing.T) {
		dir, sum := mk(t)
		os.Rename(filepath.Join(dir, "x.vpk"), filepath.Join(dir, "game.bin"))
		os.WriteFile(filepath.Join(dir, "cboc-release.json"), []byte(cbocJSON("old.vpk", sum)), 0o644)
		rs, err := Load(dir, t.TempDir())
		wantDisc(t, rs, err)
	})
	t.Run("release file renamed", func(t *testing.T) {
		dir, sum := mk(t)
		os.WriteFile(filepath.Join(dir, "strikers-release.json"), []byte(cbocJSON("x.vpk", sum)), 0o644)
		rs, err := Load(dir, t.TempDir())
		wantDisc(t, rs, err)
	})
	t.Run("a record file next to the release file is not mistaken for it", func(t *testing.T) {
		dir, sum := mk(t)
		os.WriteFile(filepath.Join(dir, "cboc-release.json"), []byte(cbocJSON("x.vpk", sum)), 0o644)
		os.WriteFile(filepath.Join(dir, "RELEASE.json"), []byte(`{"schemaVersion":1,"titleId":"MSCN00003","package":{"path":"x.vpk"}}`), 0o644)
		rs, err := Load(dir, t.TempDir())
		wantDisc(t, rs, err)
	})
	t.Run("a VPK downloaded under an odd name", func(t *testing.T) {
		dir, _ := mk(t)
		odd := filepath.Join(dir, "download")
		os.Rename(filepath.Join(dir, "x.vpk"), odd)
		rs, err := Load(odd, t.TempDir())
		if err != nil || len(rs) != 1 || rs[0].TitleID != "MSCN00003" {
			t.Fatalf("%v %+v", err, rs)
		}
	})
	t.Run("a wrong app is refused", func(t *testing.T) {
		dir, _ := mk(t)
		os.WriteFile(filepath.Join(dir, "cboc-release.json"), []byte(cbocJSON("x.vpk", "00"+fmt.Sprintf("%062d", 0))), 0o644)
		if _, err := Load(dir, t.TempDir()); err == nil {
			t.Fatal("an app with the wrong checksum was accepted")
		}
	})
}
