package install

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/CBOC-Vita-Lab/vita-installer/internal/device"
	"github.com/CBOC-Vita-Lab/vita-installer/internal/fakevita"
	"github.com/CBOC-Vita-Lab/vita-installer/internal/release"
)

var (
	fakeBuilderOnce sync.Once
	fakeBuilderExe  string
	fakeBuilderErr  error
	fakeBuilderDir  string
)

func TestMain(m *testing.M) {
	code := m.Run()
	if fakeBuilderDir != "" {
		os.RemoveAll(fakeBuilderDir)
	}
	os.Exit(code)
}

// fakeBuilder compiles the stand-in build program once for all tests.
func fakeBuilder(t *testing.T) string {
	t.Helper()
	fakeBuilderOnce.Do(func() {
		fakeBuilderDir, fakeBuilderErr = os.MkdirTemp("", "fakebuilder")
		if fakeBuilderErr != nil {
			return
		}
		fakeBuilderExe = filepath.Join(fakeBuilderDir, "infected-build")
		if runtime.GOOS == "windows" {
			fakeBuilderExe += ".exe"
		}
		if out, err := exec.Command("go", "build", "-o", fakeBuilderExe, "./testdata/fakebuilder").CombinedOutput(); err != nil {
			fakeBuilderErr = fmt.Errorf("%v: %s", err, out)
		}
	})
	if fakeBuilderErr != nil {
		t.Fatal(fakeBuilderErr)
	}
	return fakeBuilderExe
}

// kitZip packs the fake builder the way a kit is packed: the program with its
// Unix permissions, a recipe and the licenses.
func kitZip(t *testing.T, extra map[string][]byte) []byte {
	t.Helper()
	exe, err := os.ReadFile(fakeBuilder(t))
	if err != nil {
		t.Fatal(err)
	}
	name := "infected-build"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	var buf bytes.Buffer
	z := zip.NewWriter(&buf)
	add := func(name string, data []byte, mode os.FileMode) {
		h := &zip.FileHeader{Name: name, Method: zip.Deflate}
		h.SetMode(mode)
		w, err := z.CreateHeader(h)
		if err != nil {
			t.Fatal(err)
		}
		w.Write(data)
	}
	add(name, exe, 0o755)
	add("kit.json", []byte(`{"recipe":"fake"}`), 0o644)
	add("licenses/SOURCES.txt", []byte("sources"), 0o644)
	for n, d := range extra {
		add(n, d, 0o644)
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// pspISO is a tiny valid PSP disc image for Infected; marker is anything
// that tells the fake builder what to do.
func pspISO(marker string) []byte { return fakevita.PSPISO("ULUS-10054", marker) }

type buildFixture struct {
	dir     string // everything lives here
	release string // the release folder
	disc    string
	kit     []byte
	kitSum  string
	kitFile string
	cache   string
}

// newBuildFixture makes a release folder with a build, the kit next to it
// (when withKit) and the player's disc.
func newBuildFixture(t *testing.T, withKit bool, marker string) *buildFixture {
	t.Helper()
	// The tests must not depend on how much room this computer has.
	t.Cleanup(FakeFreeSpace(100 << 30))
	f := &buildFixture{dir: t.TempDir()}
	f.release = filepath.Join(f.dir, "Infected-Vita")
	os.MkdirAll(f.release, 0o755)
	f.kit = kitZip(t, nil)
	sum := sha256.Sum256(f.kit)
	f.kitSum = hex.EncodeToString(sum[:])
	f.kitFile = "Infected-Vita-builder-test.zip"
	if withKit {
		os.WriteFile(filepath.Join(f.release, f.kitFile), f.kit, 0o644)
	}
	f.writeManifest(t, f.kitSum)
	f.disc = filepath.Join(f.dir, "Infected.iso")
	os.WriteFile(f.disc, pspISO(marker), 0o644)
	f.cache = filepath.Join(f.dir, "cache")
	return f
}

func (f *buildFixture) writeManifest(t *testing.T, kitSum string) {
	t.Helper()
	key := runtime.GOOS + "-" + runtime.GOARCH
	m := map[string]interface{}{
		"name":    "Infected",
		"titleId": "INFC00001",
		"build": map[string]interface{}{
			"from": "disc",
			"kit": map[string]interface{}{
				key:             map[string]interface{}{"file": f.kitFile, "sha256": kitSum, "size": len(f.kit)},
				"someos-sparc":  nil,
				"linux-riscv64": map[string]interface{}{"file": "other.zip", "sha256": strings.Repeat("c", 64)},
			},
			"run":     "infected-build",
			"args":    []string{"--progress", "--out", "{out}", "{in}"},
			"app":     "infected.vpk",
			"label":   "Building Infected from your disc",
			"minutes": 10,
		},
		"needs": []map[string]interface{}{
			{"id": "disc", "type": "psp-iso", "gameId": "ULUS-10054", "to": "ux0:data/infected/Infected.iso", "label": "Your Infected UMD backup (PSP, USA, ISO)", "prepare": []map[string]string{{"step": "build"}}},
			{"id": "shader", "type": "shader-compiler", "optional": true},
		},
	}
	delete(m["build"].(map[string]interface{})["kit"].(map[string]interface{}), "someos-sparc")
	b, _ := json.Marshal(m)
	if err := os.WriteFile(filepath.Join(f.release, release.ManifestName), b, 0o644); err != nil {
		t.Fatal(err)
	}
}

// load reads the release, chooses the disc and (if allow) allows the build.
func (f *buildFixture) load(t *testing.T, allow bool) *release.Release {
	t.Helper()
	r := load(t, f.release)
	for _, in := range r.Inputs {
		switch in.ID {
		case "disc":
			CheckInput(in, f.disc)
			if in.State != "ok" {
				t.Fatalf("disc: %s %s", in.State, in.Message)
			}
		case release.BuildConsentID:
			if allow {
				CheckInput(in, "yes")
			}
		}
	}
	return r
}

func (f *buildFixture) runCount(t *testing.T) int {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(f.cache, "kits", f.kitSum[:16], "ran.log"))
	if err != nil {
		return 0
	}
	return strings.Count(string(b), "\n")
}

func startVita(t *testing.T, dir string) (*fakevita.Vita, *device.Vita) {
	t.Helper()
	v, err := fakevita.Start(filepath.Join(dir, "vita"), true, false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(v.Close)
	return v, &device.Vita{Host: "127.0.0.1", Port: v.Port()}
}

// The whole path: a release that builds its app from the disc is installed on
// the Vita, with progress, and the second time nothing is built again.
func TestBuildAndInstallEndToEnd(t *testing.T) {
	f := newBuildFixture(t, true, "MODE:OK")
	r := f.load(t, true)
	if r.Build == nil || r.VPK != "" {
		t.Fatalf("release: %+v", r)
	}
	v, vita := startVita(t, f.dir)

	var mu sync.Mutex
	var steps []string
	var sawBar bool
	var phases = map[string]bool{}
	report := func(p Progress) {
		mu.Lock()
		defer mu.Unlock()
		phases[p.Phase] = true
		if p.Phase == "prepare" && p.Text != "" && (len(steps) == 0 || steps[len(steps)-1] != p.Text) {
			steps = append(steps, p.Text)
		}
		if p.Phase == "prepare" && p.Total == 4 && p.Done > 0 {
			sawBar = true
			if p.Unit != "count" || p.Done > p.Total {
				t.Errorf("progress %+v", p)
			}
		}
	}
	res, err := Run(context.Background(), r, Options{Vita: vita, Cache: f.cache, OnProgress: report})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Installed || res.Built == nil || res.Built.TitleID != "INFC00001" {
		t.Fatalf("result: %+v", res)
	}
	mustFile(t, v.Root, "ux0/app/INFC00001/eboot.bin")
	mustFile(t, v.Root, "ux0/app/INFC00001/module/infected_core3.skprx")
	disc, _ := os.ReadFile(filepath.Join(v.Root, "ux0/data/infected/Infected.iso"))
	if !bytes.Equal(disc, pspISO("MODE:OK")) {
		t.Error("the disc was not copied to the Vita as usual")
	}
	// What the player saw, in order.
	want := []string{"Building Infected from your disc", "Reading your disc", "Compiling (this is the long part)", "Packaging"}
	mu.Lock()
	got := strings.Join(steps, "|")
	mu.Unlock()
	last := -1
	for _, w := range want {
		i := strings.Index(got, w)
		if i < 0 || i < last {
			t.Errorf("step %q missing or out of order in %q", w, got)
		}
		last = i
	}
	if !sawBar || !phases["send"] || !phases["install"] || !phases["done"] {
		t.Errorf("progress incomplete: bar=%v phases=%v", sawBar, phases)
	}
	if n := f.runCount(t); n != 1 {
		t.Fatalf("the builder ran %d times", n)
	}
	// Scratch files went where the room was checked, and nothing is left behind.
	if b, _ := os.ReadFile(filepath.Join(f.cache, "kits", f.kitSum[:16], "tmpdir.txt")); !strings.Contains(string(b), filepath.Join(f.cache, "apps")) {
		t.Errorf("the builder's scratch folder was %q", b)
	}
	left, _ := filepath.Glob(filepath.Join(f.cache, "apps", ".build-*"))
	if len(left) > 0 {
		t.Errorf("work folders left behind: %v", left)
	}
	if built, _ := filepath.Glob(filepath.Join(f.cache, "apps", "INFC00001-build-*.vpk")); len(built) != 1 {
		t.Errorf("the built app should stay cached for a retry: %v", built)
	}

	// The receipt, update detection and the page's check agree.
	p, err := Check(context.Background(), vita, "", r)
	if err != nil || !p.Installed || !p.Same || p.Receipt == nil {
		t.Fatalf("check after install: %v %+v", err, p)
	}
	if !p.Inputs["disc"] {
		t.Error("the disc should be known to be on the Vita")
	}
	// Reinstalling builds nothing and sends nothing.
	res, err = Run(context.Background(), f.load(t, true), Options{Vita: vita, Cache: f.cache})
	if err != nil {
		t.Fatal(err)
	}
	if n := f.runCount(t); n != 1 {
		t.Fatalf("a reinstall built again (%d runs)", n)
	}
	if res.Copied > 1 { // the app itself is sent again, as for any app
		t.Errorf("a reinstall sent %d files", res.Copied)
	}
	// A different disc file is built again.
	os.WriteFile(f.disc, pspISO("MODE:OK and different"), 0o644)
	if _, err := Run(context.Background(), f.load(t, true), Options{Vita: vita, Cache: f.cache}); err != nil {
		t.Fatal(err)
	}
	if n := f.runCount(t); n != 2 {
		t.Fatalf("a changed disc should be built again (%d runs)", n)
	}
	if built, _ := filepath.Glob(filepath.Join(f.cache, "apps", "INFC00001-build-*.vpk")); len(built) != 1 {
		t.Errorf("old builds should be cleaned up: %v", built)
	}
}

// The player's own artwork goes on the built app like on any other.
func TestBuiltAppGetsArtwork(t *testing.T) {
	f := newBuildFixture(t, true, "MODE:OK")
	r := f.load(t, true)
	pic := filepath.Join(f.dir, "cover.png")
	img := image.NewNRGBA(image.Rect(0, 0, 300, 400))
	for i := range img.Pix {
		img.Pix[i] = uint8(i)
	}
	pf, _ := os.Create(pic)
	png.Encode(pf, img)
	pf.Close()
	r.Art.Icon = pic
	v, vita := startVita(t, f.dir)
	if _, err := Run(context.Background(), r, Options{Vita: vita, Cache: f.cache}); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(v.Root, "ux0/app/INFC00001/sce_sys/icon0.png"))
	if err != nil {
		t.Fatal(err)
	}
	got, err := png.Decode(bytes.NewReader(b))
	if err != nil || got.Bounds().Dx() != 128 {
		t.Fatalf("installed icon is not the player's picture: %v", err)
	}
	if p, err := Check(context.Background(), vita, "", r); err != nil || !p.Same {
		t.Fatalf("%v %+v", err, p)
	}
}

func TestBuildNeedsPermissionAndDisc(t *testing.T) {
	f := newBuildFixture(t, true, "MODE:OK")
	_, vita := startVita(t, f.dir)
	if _, err := Run(context.Background(), f.load(t, false), Options{Vita: vita, Cache: f.cache}); err == nil || f.runCount(t) != 0 {
		t.Fatalf("the build ran without permission (%v)", err)
	}
	// Even if the page says the disc is "already on the Vita", a build needs it here.
	r := f.load(t, true)
	for _, in := range r.Inputs {
		if in.ID == "disc" {
			in.State, in.Path = "skip", ""
		}
	}
	if _, err := Run(context.Background(), r, Options{Vita: vita, Cache: f.cache}); err == nil || !strings.Contains(err.Error(), "disc") {
		t.Fatalf("expected a request for the disc, got %v", err)
	}
}

func TestBuildFailureShowsTheBuildersMessage(t *testing.T) {
	f := newBuildFixture(t, true, "MODE:BAD")
	_, vita := startVita(t, f.dir)
	_, err := Run(context.Background(), f.load(t, true), Options{Vita: vita, Cache: f.cache})
	if err == nil || !strings.Contains(err.Error(), "This is not the US version of Infected (ULUS-10054, 1.02).") {
		t.Fatalf("the builder's own message should reach the player, got %v", err)
	}
	if built, _ := filepath.Glob(filepath.Join(f.cache, "apps", "*")); len(built) > 0 {
		t.Errorf("a failed build left files behind: %v", built)
	}
	// Trying again with the right disc works, with the kit already unpacked.
	os.WriteFile(f.disc, pspISO("MODE:OK"), 0o644)
	if _, err := Run(context.Background(), f.load(t, true), Options{Vita: vita, Cache: f.cache}); err != nil {
		t.Fatal(err)
	}
	if n := f.runCount(t); n != 2 {
		t.Fatalf("builder ran %d times", n)
	}
}

func TestBuildMustMakeTheRightApp(t *testing.T) {
	f := newBuildFixture(t, true, "MODE:WRONG")
	_, vita := startVita(t, f.dir)
	_, err := Run(context.Background(), f.load(t, true), Options{Vita: vita, Cache: f.cache})
	if err == nil || !strings.Contains(err.Error(), "WRNG00001") {
		t.Fatalf("a build that makes another app was accepted: %v", err)
	}
}

func TestBuildCanBeStopped(t *testing.T) {
	f := newBuildFixture(t, true, "MODE:SLOW")
	_, vita := startVita(t, f.dir)
	ctx, cancel := context.WithCancel(context.Background())
	began := make(chan struct{}, 1)
	done := make(chan error, 1)
	go func() {
		_, err := Run(ctx, f.load(t, true), Options{Vita: vita, Cache: f.cache, OnProgress: func(p Progress) {
			if strings.HasPrefix(p.Text, "Compiling") {
				select {
				case began <- struct{}{}:
				default:
				}
			}
		}})
		done <- err
	}()
	select {
	case <-began:
	case <-time.After(30 * time.Second):
		t.Fatal("the build never started")
	}
	cancel()
	select {
	case err := <-done:
		if err == nil || ctx.Err() == nil {
			t.Fatalf("stopped build returned %v", err)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("stopping the build took too long")
	}
}

func TestBuildChecksFreeSpace(t *testing.T) {
	f := newBuildFixture(t, true, "MODE:OK")
	_, vita := startVita(t, f.dir)
	freeSpace = func(string) int64 { return 100 << 20 }
	_, err := Run(context.Background(), f.load(t, true), Options{Vita: vita, Cache: f.cache})
	if err == nil || !strings.Contains(err.Error(), "free") || f.runCount(t) != 0 {
		t.Fatalf("a full disk should stop the build before it starts: %v", err)
	}
}

// A kit that is not the one the release promised never runs.
func TestKitMustMatchItsChecksum(t *testing.T) {
	f := newBuildFixture(t, true, "MODE:OK")
	_, vita := startVita(t, f.dir)
	f.writeManifest(t, strings.Repeat("d", 64))
	_, err := Run(context.Background(), f.load(t, true), Options{Vita: vita, Cache: f.cache})
	if err == nil || !strings.Contains(err.Error(), "integrity") {
		t.Fatalf("a kit with the wrong checksum was accepted: %v", err)
	}
	if _, err := os.Stat(filepath.Join(f.cache, "kits", strings.Repeat("d", 16))); err == nil {
		t.Fatal("a kit with the wrong checksum was unpacked")
	}
	if f.runCount(t) != 0 {
		t.Fatal("it ran")
	}
	// A kit missing from the download, with nowhere to get it from.
	g := newBuildFixture(t, false, "MODE:OK")
	_, err = Run(context.Background(), g.load(t, true), Options{Vita: vita, Cache: g.cache})
	if err == nil || !strings.Contains(err.Error(), "not in this download") {
		t.Fatalf("expected a hint about the missing build tools: %v", err)
	}
}

// The kit is downloaded from the release only when the build runs, checked,
// unpacked once and reused.
func TestKitDownload(t *testing.T) {
	f := newBuildFixture(t, false, "MODE:OK")
	var hits int32
	body := f.kit
	status := 200
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		if r.URL.Path != "/CBOC-Vita-Lab/infected-vita/releases/download/v1.0/"+f.kitFile {
			http.NotFound(w, r)
			return
		}
		if status != 200 {
			http.Error(w, "no", status)
			return
		}
		w.Header().Set("Content-Length", fmt.Sprint(len(body)))
		w.Write(body)
	}))
	defer srv.Close()
	old := kitDownloadURL
	kitDownloadURL = func(repo, tag, file string) string {
		return srv.URL + "/" + repo + "/releases/download/" + tag + "/" + file
	}
	defer func() { kitDownloadURL = old }()
	_, vita := startVita(t, f.dir)

	fromGitHub := func() *release.Release {
		r := f.load(t, true)
		r.Repo, r.Tag = "CBOC-Vita-Lab/infected-vita", "v1.0"
		return r
	}
	if n := atomic.LoadInt32(&hits); n != 0 {
		t.Fatalf("%d downloads before anything was built", n)
	}

	// A damaged download is refused and nothing is unpacked.
	body = append(append([]byte(nil), f.kit...), 0)
	if _, err := Run(context.Background(), fromGitHub(), Options{Vita: vita, Cache: f.cache}); err == nil || !strings.Contains(err.Error(), "damaged") {
		t.Fatalf("a damaged kit was accepted: %v", err)
	}
	if _, err := os.Stat(filepath.Join(f.cache, "kits", f.kitSum[:16])); err == nil {
		t.Fatal("a damaged kit was unpacked")
	}
	if left, _ := filepath.Glob(filepath.Join(f.cache, "kits", "*")); len(left) > 0 {
		t.Fatalf("download leftovers: %v", left)
	}
	// A missing kit says so.
	status = 404
	if _, err := Run(context.Background(), fromGitHub(), Options{Vita: vita, Cache: f.cache}); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("a missing kit: %v", err)
	}
	status, body = 200, f.kit

	// The real one, with progress.
	var mu sync.Mutex
	var got []Progress
	res, err := Run(context.Background(), fromGitHub(), Options{Vita: vita, Cache: f.cache, OnProgress: func(p Progress) {
		mu.Lock()
		got = append(got, p)
		mu.Unlock()
	}})
	if err != nil || !res.Installed {
		t.Fatal(err)
	}
	var dl, unpack, finished bool
	for _, p := range got {
		dl = dl || p.Text == "Downloading the build tools"
		unpack = unpack || p.Text == "Unpacking the build tools"
		finished = finished || (p.Text == "Downloading the build tools" && p.Done == p.Total && p.Total > 0) || (p.Text == "Unpacking the build tools" && p.Done == p.Total && p.Total > 0)
	}
	if !dl || !unpack || !finished {
		t.Errorf("download %v unpack %v finished %v", dl, unpack, finished)
	}
	if runtime.GOOS != "windows" {
		st, err := os.Stat(filepath.Join(f.cache, "kits", f.kitSum[:16], "infected-build"))
		if err != nil || st.Mode()&0o111 == 0 {
			t.Errorf("the builder's executable bit was not restored: %v %v", err, st)
		}
	}
	if left, _ := filepath.Glob(filepath.Join(f.cache, "kits", "*.*")); len(left) > 0 {
		t.Errorf("download leftovers: %v", left)
	}
	n := atomic.LoadInt32(&hits)
	// Another disc, another build, but the same kit: no new download.
	os.WriteFile(f.disc, pspISO("MODE:OK again"), 0o644)
	if _, err := Run(context.Background(), fromGitHub(), Options{Vita: vita, Cache: f.cache}); err != nil {
		t.Fatal(err)
	}
	if atomic.LoadInt32(&hits) != n {
		t.Error("the kit was downloaded again")
	}
}

func TestUnpackKitIsSafe(t *testing.T) {
	zipOf := func(entries func(z *zip.Writer)) string {
		p := filepath.Join(t.TempDir(), "kit.zip")
		fh, _ := os.Create(p)
		z := zip.NewWriter(fh)
		entries(z)
		z.Close()
		fh.Close()
		return p
	}
	for _, name := range []string{"../evil.txt", "a/../../evil.txt", "/abs/evil.txt", `..\evil.txt`, "C:/evil.txt"} {
		name := name
		p := zipOf(func(z *zip.Writer) { w, _ := z.Create(name); w.Write([]byte("x")) })
		dir := filepath.Join(t.TempDir(), "kit")
		if err := unpackKit(context.Background(), p, dir, func(int64, int64) {}); err == nil {
			t.Errorf("%q was unpacked", name)
		}
		if _, err := os.Stat(filepath.Join(filepath.Dir(dir), "evil.txt")); err == nil {
			t.Errorf("%q escaped the kit folder", name)
		}
	}
	if runtime.GOOS == "windows" {
		return
	}
	// Programs keep their executable bit; links stay inside the kit.
	p := zipOf(func(z *zip.Writer) {
		h := &zip.FileHeader{Name: "bin/tool", Method: zip.Deflate}
		h.SetMode(0o755)
		w, _ := z.CreateHeader(h)
		w.Write([]byte("#!/bin/sh\n"))
		h = &zip.FileHeader{Name: "bin/alias"}
		h.SetMode(os.ModeSymlink | 0o777)
		w, _ = z.CreateHeader(h)
		w.Write([]byte("tool"))
		w, _ = z.Create("data.txt")
		w.Write([]byte("d"))
	})
	dir := filepath.Join(t.TempDir(), "kit")
	if err := unpackKit(context.Background(), p, dir, func(int64, int64) {}); err != nil {
		t.Fatal(err)
	}
	if st, err := os.Stat(filepath.Join(dir, "bin/alias")); err != nil || st.Mode()&0o111 == 0 {
		t.Errorf("link to the tool: %v %v", st, err)
	}
	if st, _ := os.Stat(filepath.Join(dir, "data.txt")); st.Mode()&0o111 != 0 {
		t.Error("a data file became executable")
	}
	bad := zipOf(func(z *zip.Writer) {
		h := &zip.FileHeader{Name: "link"}
		h.SetMode(os.ModeSymlink | 0o777)
		w, _ := z.CreateHeader(h)
		w.Write([]byte("../../etc/passwd"))
	})
	if err := unpackKit(context.Background(), bad, filepath.Join(t.TempDir(), "kit"), func(int64, int64) {}); err == nil {
		t.Error("a link out of the kit folder was unpacked")
	}
}

func TestBuilderOutputLines(t *testing.T) {
	var got []string
	w := &lineWriter{fn: func(s string) { got = append(got, s) }}
	w.Write([]byte("@step One\r\n@progr"))
	w.Write([]byte("ess 1 2\n\n@done /x"))
	w.flush()
	if strings.Join(got, "|") != "@step One|@progress 1 2||@done /x" {
		t.Fatalf("%q", got)
	}
	tail := &tailWriter{}
	tail.Write([]byte("first\n\nsecond\nthird\nfourth\n"))
	if s := tail.last(2); s != "third fourth" {
		t.Fatalf("%q", s)
	}
}
