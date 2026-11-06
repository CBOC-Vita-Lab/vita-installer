package install

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CBOC-Vita-Lab/vita-installer/internal/release"
)

// TestRealReleasesLoad reads real game packages and lists what each asks
// for, without installing. VITA_LOAD=<path>[;<path>...] (files, folders, ZIPs).
func TestRealReleasesLoad(t *testing.T) {
	paths := os.Getenv("VITA_LOAD")
	if paths == "" {
		t.Skip("set VITA_LOAD")
	}
	for _, p := range strings.Split(paths, ";") {
		rs, err := release.Load(p, t.TempDir())
		if err != nil {
			t.Errorf("%s: %v", filepath.Base(p), err)
			continue
		}
		for _, r := range rs {
			app := "(built on the computer)"
			if r.App != nil {
				app = r.App.TitleID + " " + r.App.Version + " " + r.App.Title
			}
			t.Logf("%s: %s [%s] app=%s held=%q warn=%q block=%q", filepath.Base(p), r.Name, r.TitleID, app, r.Held, r.Warning, r.BuildBlock)
			for _, in := range r.Inputs {
				t.Logf("    %-8s %-15s opt=%-5v to=%s ext=%v magic=%v prepare=%d %s", in.ID, in.Type, in.Optional, in.To, in.Ext, in.Magic != "", len(in.Prepare), in.Label)
			}
		}
	}
}
