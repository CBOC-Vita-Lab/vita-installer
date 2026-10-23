package install

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/CBOC-Vita-Lab/vita-installer/internal/device"
	"github.com/CBOC-Vita-Lab/vita-installer/internal/fakevita"
)

// An update that changes a bundled file without changing its size must still
// send it, and the receipt must say which release is installed.
func TestUpdateSendsSameSizeChanges(t *testing.T) {
	rel, dir := makeRelease(t, false)
	vroot := filepath.Join(dir, "vita")
	v, err := fakevita.Start(vroot, true, false)
	if err != nil {
		t.Fatal(err)
	}
	defer v.Close()
	vita := &device.Vita{Host: "127.0.0.1", Port: v.Port()}
	run := func() *Result {
		r := load(t, rel)
		r.Repo, r.Tag = "CBOC-Vita-Lab/my-game", "v1"
		for _, in := range r.Inputs {
			in.State = "skip"
		}
		res, err := Run(context.Background(), r, Options{Vita: vita, Cache: t.TempDir()})
		if err != nil {
			t.Fatal(err)
		}
		return res
	}
	run()
	cfg := "ux0/data/mygame/config.ini"
	if b, _ := os.ReadFile(filepath.Join(vroot, cfg)); string(b) != "speed=fast" {
		t.Fatalf("first install: %q", b)
	}
	// Nothing changed: the bundled files are skipped (only the app is staged again).
	res := run()
	if res.Skipped < 2 {
		t.Errorf("unchanged files were sent again: %+v", res)
	}
	// Same size, new content.
	os.WriteFile(filepath.Join(rel, "data", "mygame", "config.ini"), []byte("speed=slow"), 0o644)
	run()
	if b, _ := os.ReadFile(filepath.Join(vroot, cfg)); string(b) != "speed=slow" {
		t.Fatalf("same-size change was not sent: %q", b)
	}
	got, err := Installed(context.Background(), vita, "")
	if err != nil || len(got) != 1 || got[0].TitleID != "MYGM00001" || got[0].Release != "v1" || got[0].Repo != "CBOC-Vita-Lab/my-game" {
		t.Fatalf("receipt: %v %+v", err, got)
	}
	if got[0].Files["ux0:data/mygame/config.ini"] == "" {
		t.Errorf("receipt has no entry for config.ini: %v", got[0].Files)
	}
	for p := range got[0].Files {
		if filepath.Dir(p) == Staging || len(p) > len(Staging) && p[:len(Staging)+1] == Staging+"/" {
			t.Errorf("staging path in receipt: %s", p)
		}
	}
	// Check sees the exact build as installed.
	r := load(t, rel)
	p, err := Check(context.Background(), vita, "", r)
	if err != nil || !p.Same || p.Receipt == nil {
		t.Fatalf("check: %v %+v", err, p)
	}
	os.WriteFile(filepath.Join(rel, "data", "mygame", "config.ini"), []byte("speed=mild"), 0o644)
	if p, _ := Check(context.Background(), vita, "", load(t, rel)); p.Same {
		t.Error("a changed release is reported as the same")
	}
}
