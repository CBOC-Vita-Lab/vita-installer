package install

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/CBOC-Vita-Lab/vita-installer/internal/device"
	"github.com/CBOC-Vita-Lab/vita-installer/internal/fakevita"
	"github.com/CBOC-Vita-Lab/vita-installer/internal/release"
)

// makeRelease builds a release folder: VPK, bundled data and a manifest that
// asks for a Wii disc (as an extracted folder) and keeps a save folder.
func makeRelease(t *testing.T, withManifest bool) (string, string) {
	dir := t.TempDir()
	rel := filepath.Join(dir, "MyGame-1.0")
	os.MkdirAll(filepath.Join(rel, "data", "mygame", "save"), 0o755)
	if err := fakevita.MakeVPK(filepath.Join(rel, "MyGame.vpk"), "MYGM00001", "My Game", map[string][]byte{"module/libshacccg-loader.txt": []byte("loads ur0:data/libshacccg.suprx")}); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(rel, "data", "mygame", "config.ini"), []byte("speed=fast"), 0o644)
	os.WriteFile(filepath.Join(rel, "data", "mygame", "big.bin"), bytes.Repeat([]byte("x"), 3<<20), 0o644)
	os.WriteFile(filepath.Join(rel, "data", "mygame", "save", "slot1.sav"), []byte("bundled save must not be copied"), 0o644)
	os.WriteFile(filepath.Join(rel, "README.txt"), []byte("docs are not copied"), 0o644)
	if withManifest {
		m := map[string]interface{}{
			"name": "My Game",
			"needs": []map[string]interface{}{
				{"id": "disc", "type": "wii-disc", "gameId": "RMYE01", "revision": 1, "to": "ux0:data/mygame/game", "label": "Your My Game disc"},
				{"type": "shader-compiler"},
			},
			"keep": []string{"ux0:data/mygame/save"},
		}
		b, _ := json.Marshal(m)
		os.WriteFile(filepath.Join(rel, release.ManifestName), b, 0o644)
	}
	// The player's extracted disc, next to the release.
	disc := filepath.Join(dir, "RMYE01")
	os.MkdirAll(filepath.Join(disc, "DATA", "sys"), 0o755)
	os.MkdirAll(filepath.Join(disc, "DATA", "files", "stage"), 0o755)
	boot := append([]byte("RMYE01\x00\x00"), make([]byte, 0x438)...)
	os.WriteFile(filepath.Join(disc, "DATA", "sys", "boot.bin"), boot, 0o644)
	os.MkdirAll(filepath.Join(disc, "DATA", "disc"), 0o755)
	os.WriteFile(filepath.Join(disc, "DATA", "disc", "header.bin"), append([]byte("RMYE01\x00\x01"), make([]byte, 0x100)...), 0o644)
	os.WriteFile(filepath.Join(disc, "DATA", "sys", "main.dol"), []byte("dol"), 0o644)
	os.WriteFile(filepath.Join(disc, "DATA", "files", "stage", "level 1.arc"), bytes.Repeat([]byte("L"), 70000), 0o644)
	shader := filepath.Join(dir, "libshacccg.suprx")
	os.WriteFile(shader, bytes.Repeat([]byte{1}, 200<<10), 0o644)
	return rel, dir
}

func load(t *testing.T, rel string) *release.Release {
	rs, err := release.Load(rel, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if len(rs) != 1 {
		t.Fatalf("got %d releases", len(rs))
	}
	return rs[0]
}

func TestInstallOverVitaShell(t *testing.T) {
	rel, dir := makeRelease(t, true)
	r := load(t, rel)
	AutoFind(r)
	for _, in := range r.Inputs {
		if in.State != "ok" {
			t.Fatalf("input %s not found automatically: %q %q", in.ID, in.State, in.Message)
		}
	}
	vroot := filepath.Join(dir, "vita")
	v, err := fakevita.Start(vroot, true, false)
	if err != nil {
		t.Fatal(err)
	}
	defer v.Close()
	// An existing save on the Vita must survive.
	os.MkdirAll(filepath.Join(vroot, "ux0/data/mygame/save"), 0o755)
	os.WriteFile(filepath.Join(vroot, "ux0/data/mygame/save/slot1.sav"), []byte("player save"), 0o644)

	vita := &device.Vita{Host: "127.0.0.1", Port: v.Port()}
	var phases []string
	res, err := Run(context.Background(), r, Options{Vita: vita, Cache: t.TempDir(), OnProgress: func(p Progress) {
		if len(phases) == 0 || phases[len(phases)-1] != p.Phase {
			phases = append(phases, p.Phase)
		}
	}})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Installed {
		t.Fatalf("not installed: %+v", res)
	}
	mustFile(t, vroot, "ux0/app/MYGM00001/eboot.bin")
	mustFile(t, vroot, "ux0/data/mygame/config.ini")
	mustFile(t, vroot, "ux0/data/mygame/game/sys/boot.bin")
	mustFile(t, vroot, "ux0/data/mygame/game/files/stage/level 1.arc")
	mustFile(t, vroot, "ur0/data/libshacccg.suprx")
	if b, _ := os.ReadFile(filepath.Join(vroot, "ux0/data/mygame/save/slot1.sav")); string(b) != "player save" {
		t.Fatalf("save was overwritten: %q", b)
	}
	if _, err := os.Stat(filepath.Join(vroot, "ux0/data/cboc-installer/MYGM00001.vpk")); err == nil {
		t.Fatal("staged VPK was not cleaned up")
	}
	if _, err := os.Stat(filepath.Join(vroot, "ux0/README.txt")); err == nil {
		t.Fatal("docs copied")
	}
	if atomic.LoadInt32(&v.MaxClients) < 2 {
		t.Errorf("expected parallel connections, max was %d", atomic.LoadInt32(&v.MaxClients))
	}
	if strings.Join(phases, ",") != "prepare,check,send,install,done" {
		t.Errorf("phases %v", phases)
	}

	// A second run only re-sends the app.
	before := v.Stores
	res, err = Run(context.Background(), r, Options{Vita: vita, Cache: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if got := v.Stores - before; got != 1 {
		t.Errorf("second run sent %d files, want 1 (the app)", got)
	}
	if res.Skipped < 4 {
		t.Errorf("skipped %d", res.Skipped)
	}
}

func TestInstallWithCompanionAndResume(t *testing.T) {
	rel, dir := makeRelease(t, false)
	r := load(t, rel)
	vroot := filepath.Join(dir, "vita")
	v, err := fakevita.Start(vroot, false, true)
	if err != nil {
		t.Fatal(err)
	}
	defer v.Close()
	old := device.CompanionPort
	device.CompanionPort = v.CompanionPort()
	defer func() { device.CompanionPort = old }()
	vita, err := device.Probe(context.Background(), "127.0.0.1", v.Port())
	if err != nil || !vita.Companion {
		t.Fatalf("probe: %v %+v", err, vita)
	}
	// Generic release: data/ folder copied, nothing else needed except the auto shader check.
	for _, in := range r.Inputs {
		if in.Type == release.NeedShader {
			in.State = "skip"
		}
	}
	v.FailStoreAfter = 2 // the connection drops during the second file
	res, err := Run(context.Background(), r, Options{Vita: vita, Cache: t.TempDir(), Connections: 1})
	if err != nil {
		t.Fatalf("transfer should recover from a dropped connection: %v", err)
	}
	if !res.Installed || !res.CanLaunch {
		t.Fatalf("%+v", res)
	}
	mustFile(t, vroot, "ux0/app/MYGM00001/sce_sys/param.sfo")
	mustFile(t, vroot, "ux0/data/mygame/big.bin")
	if st, _ := os.Stat(filepath.Join(vroot, "ux0/data/mygame/big.bin")); st.Size() != 3<<20 {
		t.Fatalf("big.bin size %d", st.Size())
	}
	if _, err := os.Stat(filepath.Join(vroot, "ux0/data/cboc-installer/MYGM00001")); err == nil {
		t.Error("staging folder left behind")
	}
	if err := Launch(context.Background(), vita, "MYGM00001"); err != nil || len(v.Launched) != 1 {
		t.Fatalf("launch: %v %v", err, v.Launched)
	}
}

func TestInstallToDrive(t *testing.T) {
	rel, dir := makeRelease(t, true)
	r := load(t, rel)
	AutoFind(r)
	drive := filepath.Join(dir, "card")
	for _, d := range []string{"app", "data", "license"} {
		os.MkdirAll(filepath.Join(drive, d), 0o755)
	}
	if !device.LooksLikeVita(drive) {
		t.Fatal("drive not recognised")
	}
	res, err := Run(context.Background(), r, Options{Drive: drive, Cache: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if res.Installed || len(res.Steps) < 2 {
		t.Fatalf("drive install should leave manual steps: %+v", res)
	}
	mustFile(t, drive, "VPK/My-Game.vpk")
	mustFile(t, drive, "VPK/libshacccg.suprx")
	mustFile(t, drive, "data/mygame/game/sys/main.dol")
	mustFile(t, drive, "data/mygame/config.ini")
}

func TestWrongDiscRejected(t *testing.T) {
	rel, dir := makeRelease(t, true)
	r := load(t, rel)
	bad := filepath.Join(dir, "other")
	os.MkdirAll(filepath.Join(bad, "sys"), 0o755)
	os.MkdirAll(filepath.Join(bad, "files"), 0o755)
	os.WriteFile(filepath.Join(bad, "sys", "boot.bin"), append([]byte("RMYP01\x00\x01"), make([]byte, 64)...), 0o644)
	for _, in := range r.Inputs {
		if in.Type == release.NeedWiiDisc {
			CheckInput(in, bad)
			if in.State != "error" || !strings.Contains(in.Message, "Europe") {
				t.Fatalf("%q %q", in.State, in.Message)
			}
		}
	}
	if _, err := Run(context.Background(), r, Options{Drive: dir, Cache: t.TempDir()}); err == nil {
		t.Fatal("install ran with a wrong disc")
	}
}

func mustFile(t *testing.T, root, rel string) {
	t.Helper()
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel))); err != nil {
		t.Fatalf("missing %s", rel)
	}
}

func TestVitaThatAcceptsOneConnection(t *testing.T) {
	rel, dir := makeRelease(t, false)
	r := load(t, rel)
	for _, in := range r.Inputs {
		in.State = "skip"
	}
	v, err := fakevita.Start(filepath.Join(dir, "vita"), true, false)
	if err != nil {
		t.Fatal(err)
	}
	defer v.Close()
	atomic.StoreInt32(&v.Limit, 1)
	res, err := Run(context.Background(), r, Options{Vita: &device.Vita{Host: "127.0.0.1", Port: v.Port()}, Cache: t.TempDir(), Connections: 4})
	if err != nil || !res.Installed {
		t.Fatalf("%v %+v", err, res)
	}
}

func TestCheckFindsGameAlreadyOnVita(t *testing.T) {
	rel, dir := makeRelease(t, true)
	r := load(t, rel)
	AutoFind(r)
	vroot := filepath.Join(dir, "vita")
	v, err := fakevita.Start(vroot, true, false)
	if err != nil {
		t.Fatal(err)
	}
	defer v.Close()
	vita := &device.Vita{Host: "127.0.0.1", Port: v.Port()}
	p, err := Check(context.Background(), vita, "", r)
	if err != nil || p.Installed || p.Inputs["disc"] {
		t.Fatalf("before install: %v %+v", err, p)
	}
	if _, err := Run(context.Background(), r, Options{Vita: vita, Cache: t.TempDir()}); err != nil {
		t.Fatal(err)
	}
	p, err = Check(context.Background(), vita, "", r)
	if err != nil || !p.Installed || !p.Inputs["disc"] || !p.Inputs["need2"] {
		t.Fatalf("after install: %v %+v", err, p)
	}
	// A different game in the folder is not mistaken for this one.
	os.WriteFile(filepath.Join(vroot, "ux0/data/mygame/game/sys/boot.bin"), []byte("RXXE01\x00\x00"), 0o644)
	if p, _ := Check(context.Background(), vita, "", r); p.Inputs["disc"] {
		t.Fatal("wrong game accepted")
	}
}
