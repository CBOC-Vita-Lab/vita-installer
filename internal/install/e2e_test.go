package install

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/CBOC-Vita-Lab/vita-installer/internal/device"
	"github.com/CBOC-Vita-Lab/vita-installer/internal/fakevita"
)

// TestRealRelease installs a real release and game folder into a fake Vita.
// Run with VITA_E2E_RELEASE=<release folder or zip> VITA_E2E_DISC=<disc>.
func TestRealRelease(t *testing.T) {
	relPath, disc := os.Getenv("VITA_E2E_RELEASE"), os.Getenv("VITA_E2E_DISC")
	if relPath == "" {
		t.Skip("set VITA_E2E_RELEASE to run")
	}
	r := load(t, relPath)
	for _, in := range r.Inputs {
		switch in.Type {
		case "wii-disc", "gc-disc", "psp-iso":
			CheckInput(in, disc)
		case "shader-compiler":
			CheckInput(in, os.Getenv("VITA_E2E_SHADER"))
		}
		t.Logf("input %s: %s %s", in.ID, in.State, in.Message)
	}
	root := filepath.Join(os.Getenv("VITA_E2E_ROOT"), "vita")
	if os.Getenv("VITA_E2E_ROOT") == "" {
		root = filepath.Join(t.TempDir(), "vita")
	}
	defer os.RemoveAll(root)
	v, err := fakevita.Start(root, true, false)
	if err != nil {
		t.Fatal(err)
	}
	defer v.Close()
	start := time.Now()
	res, err := Run(context.Background(), r, Options{Vita: &device.Vita{Host: "127.0.0.1", Port: v.Port()}, Cache: filepath.Join(filepath.Dir(root), "cache")})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("installed=%v copied=%d skipped=%d bytes=%d in %s (max %d connections)", res.Installed, res.Copied, res.Skipped, res.Bytes, time.Since(start), atomic.LoadInt32(&v.MaxClients))
	start = time.Now()
	res, err = Run(context.Background(), r, Options{Vita: &device.Vita{Host: "127.0.0.1", Port: v.Port()}, Cache: filepath.Join(filepath.Dir(root), "cache")})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("re-run: copied=%d skipped=%d in %s", res.Copied, res.Skipped, time.Since(start))
	p, err := Check(context.Background(), &device.Vita{Host: "127.0.0.1", Port: v.Port()}, "", r)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("already on the Vita: installed=%v inputs=%v", p.Installed, p.Inputs)
	for id, ok := range p.Inputs {
		if !ok {
			t.Errorf("input %s not recognised on the Vita", id)
		}
	}
}
