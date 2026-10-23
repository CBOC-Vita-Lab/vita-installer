package main

import (
	"bytes"
	"debug/macho"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// The re-signing code must produce exactly what the Go linker produces, or
// the Apple Silicon app would not start after LC_UUID is added.
func TestUUIDAndSignature(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "main.go")
	os.WriteFile(src, []byte("package main\nfunc main(){}\n"), 0o644)
	bin := filepath.Join(dir, "bin")
	cmd := exec.Command("go", "build", "-o", bin, src)
	cmd.Env = append(os.Environ(), "GOOS=darwin", "GOARCH=arm64", "CGO_ENABLED=0")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("cannot build a darwin binary here: %v %s", err, out)
	}
	orig, _ := os.ReadFile(bin)
	f, err := macho.Open(bin)
	if err != nil {
		t.Fatal(err)
	}
	var sigOff, sigSize uint32
	for _, l := range f.Loads {
		b := l.Raw()
		if f.ByteOrder.Uint32(b) == lcCodeSignature {
			sigOff, sigSize = f.ByteOrder.Uint32(b[8:]), f.ByteOrder.Uint32(b[12:])
		}
	}
	text := f.Segment("__TEXT")
	f.Close()
	sig := make([]byte, sigSize)
	Sign(sig, bytes.NewReader(orig[:sigOff]), "a.out", int64(sigOff), int64(text.Offset), int64(text.Filesz), true)
	if !bytes.Equal(sig, orig[sigOff:sigOff+sigSize]) {
		t.Fatal("signature differs from the Go linker's")
	}
	if err := addUUID(bin); err != nil {
		t.Fatal(err)
	}
	f, err = macho.Open(bin)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	found := false
	for _, l := range f.Loads {
		if f.ByteOrder.Uint32(l.Raw()) == lcUUID {
			found = true
		}
	}
	if !found {
		t.Fatal("no LC_UUID")
	}
}
