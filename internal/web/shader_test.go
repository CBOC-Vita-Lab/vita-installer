package web

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/CBOC-Vita-Lab/vita-installer/internal/device"
	"github.com/CBOC-Vita-Lab/vita-installer/internal/fakevita"
	"github.com/CBOC-Vita-Lab/vita-installer/internal/release"
)

// fakeGitHub serves a ShaRKBR33D release the way GitHub's API describes one.
type fakeGitHub struct {
	srv      *httptest.Server
	vpk      []byte
	digest   string // what the API says ("" = none, as for old releases)
	requests int32
}

func newFakeGitHub(t *testing.T, titleID string) *fakeGitHub {
	t.Helper()
	p := filepath.Join(t.TempDir(), "s.vpk")
	if err := fakevita.MakeVPK(p, titleID, "ShaRKBR33D", map[string][]byte{"vitadb.skprx": []byte("k")}); err != nil {
		t.Fatal(err)
	}
	g := &fakeGitHub{}
	g.vpk, _ = os.ReadFile(p)
	sum := sha256.Sum256(g.vpk)
	g.digest = "sha256:" + hex.EncodeToString(sum[:])
	g.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&g.requests, 1)
		switch r.URL.Path {
		case "/latest":
			a := map[string]interface{}{"name": "ShaRKBR33D.vpk", "browser_download_url": g.srv.URL + "/ShaRKBR33D.vpk", "size": len(g.vpk)}
			if g.digest != "" {
				a["digest"] = g.digest
			}
			json.NewEncoder(w).Encode(map[string]interface{}{"tag_name": "v.1.2", "assets": []interface{}{
				map[string]interface{}{"name": "notes.txt", "browser_download_url": g.srv.URL + "/notes.txt"}, a}})
		case "/ShaRKBR33D.vpk":
			w.Write(g.vpk)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(g.srv.Close)
	old := sharkLatestURL
	sharkLatestURL = g.srv.URL + "/latest"
	t.Cleanup(func() { sharkLatestURL = old })
	return g
}

// shaderApp is an app with a Vita connected and a game that needs the shader
// compiler in its list.
func shaderApp(t *testing.T, optional bool) (*App, *fakevita.Vita, *Item) {
	t.Helper()
	dir := t.TempDir()
	rel := filepath.Join(dir, "Game")
	os.MkdirAll(rel, 0o755)
	if err := fakevita.MakeVPK(filepath.Join(rel, "Game.vpk"), "GAME00001", "Game", nil); err != nil {
		t.Fatal(err)
	}
	m, _ := json.Marshal(map[string]interface{}{"needs": []map[string]interface{}{{"id": "shader", "type": "shader-compiler", "optional": optional}}})
	os.WriteFile(filepath.Join(rel, "vita-install.json"), m, 0o644)
	v, err := fakevita.Start(filepath.Join(dir, "vita"), true, false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(v.Close)
	app := NewApp("1.9", t.TempDir(), t.TempDir())
	app.st.Vita = &device.Vita{Host: "127.0.0.1", Port: v.Port()}
	if err := app.Add(rel); err != nil {
		t.Fatal(err)
	}
	app.bg.Wait()
	app.refreshShader(context.Background())
	return app, v, app.st.Items[0]
}

func waitJob(t *testing.T, app *App) {
	t.Helper()
	for i := 0; i < 400; i++ {
		app.mu.Lock()
		done := app.st.Job == nil
		app.mu.Unlock()
		if done {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatal("the job never finished")
}

func (a *App) shaderState() ShaderState {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.st.Shader == nil {
		return ShaderState{}
	}
	return *a.st.Shader
}

func (a *App) noticeText() (string, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.st.Notice, a.st.NoticeBad
}

// The whole "Prepare your Vita" path: the Vita has no compiler, the installer
// puts ShaRKBR33D on it, tells the player what to do there, and "Check again"
// notices the compiler once ShaRKBR33D has made it.
func TestPrepareYourVita(t *testing.T) {
	g := newFakeGitHub(t, "SHRKBR33D")
	app, v, it := shaderApp(t, false)

	if st := app.shaderState(); !st.Known || st.Present || st.Helper {
		t.Fatalf("before: %+v", st)
	}
	if it.Status != "needs" {
		t.Fatalf("a required compiler the Vita lacks: status %q", it.Status)
	}
	if err := app.PrepareShader(); err != nil {
		t.Fatal(err)
	}
	waitJob(t, app)
	if _, err := os.Stat(filepath.Join(v.Root, "ux0/app/SHRKBR33D/eboot.bin")); err != nil {
		note, _ := app.noticeText()
		t.Fatalf("ShaRKBR33D is not on the Vita: %s", note)
	}
	st := app.shaderState()
	if !st.Helper || st.Present {
		t.Fatalf("after: %+v", st)
	}
	for _, want := range []string{"On your Vita, open ShaRKBR33D once and follow it (it needs Wi-Fi); then come back"} {
		if !strings.Contains(st.Message, want) {
			t.Errorf("message %q lacks %q", st.Message, want)
		}
		if note, bad := app.noticeText(); !strings.Contains(note, want) || bad {
			t.Errorf("notice %q (bad=%v)", note, bad)
		}
	}
	if _, err := os.Stat(filepath.Join(v.Root, "ux0/data/cboc-installer/installed/SHRKBR33D.json")); err != nil {
		t.Error("the install was not recorded like any other app")
	}

	// Pressing "Check again" too early says it is not there yet.
	if err := app.CheckShader(context.Background()); err != nil {
		t.Fatal(err)
	}
	if note, _ := app.noticeText(); !strings.Contains(note, "Not there yet") {
		t.Errorf("notice %q", note)
	}
	if it.Status != "needs" {
		t.Errorf("status %q before the compiler exists", it.Status)
	}

	// ShaRKBR33D does its work on the Vita; now the installer sees it.
	os.MkdirAll(filepath.Join(v.Root, "ur0/data"), 0o755)
	os.WriteFile(filepath.Join(v.Root, "ur0/data/libshacccg.suprx"), make([]byte, 300<<10), 0o644)
	if err := app.CheckShader(context.Background()); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 100 && it.Status != "ready"; i++ {
		time.Sleep(20 * time.Millisecond)
	}
	app.mu.Lock()
	row := it.Inputs[0]
	state, status := row.State, it.Status
	app.mu.Unlock()
	if state != "skip" || status != "ready" {
		t.Errorf("after the check: row %q, game %q", state, status)
	}
	if st := app.shaderState(); !st.Present || st.Message != "" {
		t.Errorf("state %+v", st)
	}
	if note, bad := app.noticeText(); !strings.Contains(note, "has the shader compiler now") || bad {
		t.Errorf("notice %q", note)
	}
	if atomic.LoadInt32(&g.requests) == 0 {
		t.Error("nothing was downloaded?")
	}
}

// An optional compiler never blocks the game.
func TestOptionalShaderCompilerDoesNotBlock(t *testing.T) {
	app, _, it := shaderApp(t, true)
	if st := app.shaderState(); !st.Known || st.Present {
		t.Fatalf("state %+v", st)
	}
	if it.Status != "ready" {
		t.Errorf("status %q", it.Status)
	}
	if !strings.Contains(it.Inputs[0].Help, "skipped") {
		t.Errorf("help %q", it.Inputs[0].Help)
	}
}

// The download must be what GitHub says it is.
func TestPrepareYourVitaChecksTheDownload(t *testing.T) {
	cases := []struct {
		name   string
		title  string
		digest func(g *fakeGitHub) string
		known  bool
		ok     bool
		say    string
	}{
		{"matching digest", "SHRKBR33D", func(g *fakeGitHub) string { return g.digest }, false, true, ""},
		{"wrong digest", "SHRKBR33D", func(g *fakeGitHub) string { return "sha256:" + strings.Repeat("0", 64) }, false, false, "damaged"},
		{"no digest, unknown file", "SHRKBR33D", func(g *fakeGitHub) string { return "" }, false, false, "could not be verified"},
		{"no digest, file the installer knows", "SHRKBR33D", func(g *fakeGitHub) string { return "" }, true, true, ""},
		{"another app", "OTHR00001", func(g *fakeGitHub) string { return g.digest }, false, false, "not the ShaRKBR33D app"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g := newFakeGitHub(t, c.title)
			g.digest = c.digest(g)
			if c.known {
				sum := sha256.Sum256(g.vpk)
				old := sharkKnown
				sharkKnown = map[string]string{hex.EncodeToString(sum[:]): "test"}
				defer func() { sharkKnown = old }()
			}
			app, v, _ := shaderApp(t, false)
			if err := app.PrepareShader(); err != nil {
				t.Fatal(err)
			}
			waitJob(t, app)
			_, err := os.Stat(filepath.Join(v.Root, "ux0/app/"+c.title+"/eboot.bin"))
			if c.ok != (err == nil) {
				note, _ := app.noticeText()
				t.Fatalf("installed=%v, notice %q", err == nil, note)
			}
			note, bad := app.noticeText()
			if !c.ok && (!bad || !strings.Contains(note, c.say)) {
				t.Errorf("notice %q (bad=%v)", note, bad)
			}
			if !c.ok {
				if _, err := os.Stat(filepath.Join(v.Root, "ux0/app/SHRKBR33D")); err == nil {
					t.Error("a download that failed its check reached the Vita")
				}
			}
		})
	}
}

func TestPrepareYourVitaWithoutNetwork(t *testing.T) {
	old := sharkLatestURL
	sharkLatestURL = "http://127.0.0.1:1/latest"
	defer func() { sharkLatestURL = old }()
	app, _, _ := shaderApp(t, false)
	if err := app.PrepareShader(); err != nil {
		t.Fatal(err)
	}
	waitJob(t, app)
	if note, bad := app.noticeText(); !bad || !strings.Contains(note, "ould not find ShaRKBR33D") {
		t.Errorf("notice %q", note)
	}
}

func TestPrepareYourVitaNeedsAVita(t *testing.T) {
	app := NewApp("1.9", t.TempDir(), t.TempDir())
	if err := app.PrepareShader(); err == nil {
		t.Error("prepared nothing")
	}
	if err := app.CheckShader(context.Background()); err == nil {
		t.Error("checked nothing")
	}
}

// The shipped list of trusted downloads holds what the installer expects.
func TestSharkKnownIsWellFormed(t *testing.T) {
	for sum := range sharkKnown {
		if len(sum) != 64 || strings.ToLower(sum) != sum {
			t.Errorf("bad checksum %q", sum)
		}
	}
	if !strings.Contains(sharkLatestURL, "Rinnegatamante/ShaRKBR33D") {
		t.Error(sharkLatestURL)
	}
}

// ---- computers that can't build ----

const noKitManifest = `{
 "name": "Infected", "titleId": "INFC00001",
 "build": {"from": "disc", "kit": {
   "windows-amd64": {"file": "w.tar.gz", "sha256": "%[1]s"},
   "darwin-arm64": {"file": "m.tar.gz", "sha256": "%[1]s"},
   "linux-amd64": {"file": "l.tar.gz", "sha256": "%[1]s"}},
  "requires": {"macos": 14}, "run": "infected-build", "app": "infected.vpk", "minutes": 10},
 "needs": [{"id": "disc", "type": "psp-iso", "gameId": "ULUS-10054", "to": "ux0:data/infected/Infected.iso"}]
}`

// A computer that can't build the game is told so on the game, before
// anything else, and nothing is downloaded. An app built elsewhere installs.
func TestNoKitThroughTheApp(t *testing.T) {
	old := release.CurrentHost
	release.CurrentHost = func() release.Host { return release.Host{OS: "darwin", Arch: "amd64", MacMajor: 12} }
	defer func() { release.CurrentHost = old }()
	var asked int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&asked, 1)
		http.NotFound(w, r)
	}))
	defer srv.Close()

	dir := t.TempDir()
	rel := filepath.Join(dir, "Infected-Vita")
	os.MkdirAll(rel, 0o755)
	os.WriteFile(filepath.Join(rel, "vita-install.json"), []byte(strings.Replace(noKitManifest, "%[1]s", strings.Repeat("a", 64), -1)), 0o644)
	disc := filepath.Join(dir, "Infected.iso")
	os.WriteFile(disc, fakevita.PSPISO("ULUS-10054", "x"), 0o644)
	built := filepath.Join(dir, "infected.vpk")
	if err := fakevita.MakeVPK(built, "INFC00001", "Infected", nil); err != nil {
		t.Fatal(err)
	}
	v, err := fakevita.Start(filepath.Join(dir, "vita"), true, false)
	if err != nil {
		t.Fatal(err)
	}
	defer v.Close()
	app := NewApp("1.9", t.TempDir(), t.TempDir())
	app.st.Vita = &device.Vita{Host: "127.0.0.1", Port: v.Port()}
	if err := app.Add(rel); err != nil {
		t.Fatal(err)
	}
	app.bg.Wait()
	it := app.st.Items[0]

	var state struct {
		Items []struct {
			BuildBlock string
			Inputs     []struct{ ID, Type, Label string }
		}
	}
	json.Unmarshal(app.Snapshot(), &state)
	const want = "Infected can't be built on this computer yet. It can be built on Windows (64-bit Intel/AMD), macOS 14 or newer, or Linux (64-bit Intel/AMD)."
	if len(state.Items) != 1 || state.Items[0].BuildBlock != want {
		t.Fatalf("the page is told %+v", state.Items)
	}
	for _, in := range state.Items[0].Inputs {
		if in.Type == "consent" {
			t.Error("asks to allow a build")
		}
	}
	app.SetInput(it.ID, "disc", disc)
	app.bg.Wait()
	if it.Status != "needs" {
		t.Fatalf("status %q with a disc but no app", it.Status)
	}
	if err := app.SetInput(it.ID, release.BuiltAppID, built); err != nil {
		t.Fatal(err)
	}
	app.bg.Wait()
	if it.Status != "ready" {
		t.Fatalf("status %q with the disc and the app", it.Status)
	}
	if err := app.Install(it.ID); err != nil {
		t.Fatal(err)
	}
	waitJob(t, app)
	app.mu.Lock()
	status, msg := it.Status, it.Error
	app.mu.Unlock()
	if status != "done" {
		t.Fatalf("status %q: %s", status, msg)
	}
	for _, p := range []string{"ux0/app/INFC00001/eboot.bin", "ux0/data/infected/Infected.iso"} {
		if _, err := os.Stat(filepath.Join(v.Root, p)); err != nil {
			t.Errorf("%s is not on the Vita", p)
		}
	}
	if n := atomic.LoadInt32(&asked); n != 0 {
		t.Errorf("%d downloads", n)
	}
}
