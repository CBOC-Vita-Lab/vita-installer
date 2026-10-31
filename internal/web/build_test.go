package web

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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
	"testing"
	"time"

	"github.com/CBOC-Vita-Lab/vita-installer/internal/device"
	"github.com/CBOC-Vita-Lab/vita-installer/internal/fakevita"
	"github.com/CBOC-Vita-Lab/vita-installer/internal/install"
)

const buildManifestJSON = `{
 "name": "Infected", "titleId": "INFC00001",
 "build": {
  "from": "disc",
  "kit": {
   "windows-amd64": {"file": "Infected-Vita-builder-windows.zip", "sha256": "%[1]s"},
   "darwin-amd64":  {"file": "Infected-Vita-builder-mac.zip",     "sha256": "%[1]s"},
   "linux-amd64":   {"file": "Infected-Vita-builder-linux.zip",   "sha256": "%[1]s"}
  },
  "run": "infected-build", "args": ["--progress", "--out", "{out}", "{in}"], "app": "infected.vpk", "minutes": 10
 },
 "needs": [
  {"id": "disc", "type": "psp-iso", "gameId": "ULUS-10054", "to": "ux0:data/infected/Infected.iso", "label": "Your Infected UMD backup (PSP, USA, ISO)"},
  {"id": "shader", "type": "shader-compiler", "optional": true}
 ]
}`

// A release that builds its app is added from its release file alone: the
// big build tools are never downloaded just to look at the game.
func TestBuildReleaseDownloadsNoKitWhenAdded(t *testing.T) {
	sum := strings.Repeat("a", 64)
	manifest := []byte(strings.Replace(buildManifestJSON, "%[1]s", sum, -1))
	var zbuf bytes.Buffer
	zw := zip.NewWriter(&zbuf)
	w, _ := zw.Create("vita-install.json")
	w.Write(manifest)
	zw.Close()
	junk := bytes.Repeat([]byte("kit"), 1000)

	cases := []struct {
		name  string
		files map[string][]byte
		list  []string
	}{
		{"loose release file and kits",
			map[string][]byte{"vita-install.json": manifest, "Infected-Vita-builder-windows.zip": junk, "Infected-Vita-builder-mac.zip": junk, "Infected-Vita-builder-linux.zip": junk},
			[]string{"Infected-Vita-builder-windows.zip", "vita-install.json", "Infected-Vita-builder-mac.zip", "Infected-Vita-builder-linux.zip"}},
		{"release file under another name",
			map[string][]byte{"info.json": manifest, "Infected-Vita-builder-windows.zip": junk, "Infected-Vita-builder-mac.zip": junk, "Infected-Vita-builder-linux.zip": junk},
			[]string{"Infected-Vita-builder-windows.zip", "info.json", "Infected-Vita-builder-mac.zip", "Infected-Vita-builder-linux.zip"}},
		{"release file inside a ZIP, kits known by name",
			map[string][]byte{"Infected-Vita-install.zip": zbuf.Bytes(), "Infected-Vita-builder-windows.zip": junk, "Infected-Vita-builder-mac.zip": junk, "Infected-Vita-builder-linux.zip": junk},
			[]string{"Infected-Vita-builder-windows.zip", "Infected-Vita-builder-mac.zip", "Infected-Vita-builder-linux.zip", "Infected-Vita-install.zip"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var mu sync.Mutex
			var asked []string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				asked = append(asked, r.URL.Path[1:])
				mu.Unlock()
				d, ok := c.files[r.URL.Path[1:]]
				if !ok {
					http.NotFound(w, r)
					return
				}
				w.Write(d)
			}))
			defer srv.Close()
			var assets []asset
			for _, n := range c.list {
				assets = append(assets, asset{Name: n, URL: srv.URL + "/" + n, Size: int64(len(c.files[n]))})
			}
			app := NewApp("1.8", t.TempDir(), t.TempDir())
			if err := app.addRelease(context.Background(), "CBOC-Vita-Lab/infected-vita", ghRelease{Tag: "v1.0", Assets: assets}); err != nil {
				t.Fatal(err)
			}
			for _, a := range asked {
				if strings.Contains(a, "builder") {
					t.Errorf("downloaded %s just to add the game", a)
				}
			}
			if len(app.st.Items) != 1 {
				t.Fatalf("%d items", len(app.st.Items))
			}
			it := app.st.Items[0]
			if it.TitleID != "INFC00001" || it.Build == nil || it.Build.Minutes != 10 || it.Repo != "CBOC-Vita-Lab/infected-vita" || it.Tag != "v1.0" {
				t.Fatalf("release: %+v", it.Release)
			}
			// What the page gets to draw.
			var state struct {
				Items []struct {
					Build struct {
						Minutes int
						From    string
					}
					Art struct{ Preview bool }
				}
			}
			if err := json.Unmarshal(app.Snapshot(), &state); err != nil || len(state.Items) != 1 || state.Items[0].Build.Minutes != 10 || state.Items[0].Build.From != "disc" {
				t.Fatalf("state: %v %s", err, app.Snapshot())
			}
			if state.Items[0].Art.Preview {
				t.Error("there are no pictures to preview before the game is built")
			}
			if _, err := app.ArtPreview(it.ID, "icon"); err == nil {
				t.Error("artwork preview of an unbuilt game")
			}
		})
	}
}

// The disc a game is built from is needed on this computer every time, even
// when a copy is already on the Vita; other files already on the Vita are not.
func TestBuildDiscIsNeverSkipped(t *testing.T) {
	dir := t.TempDir()
	rel := filepath.Join(dir, "Infected-Vita")
	os.MkdirAll(rel, 0o755)
	os.WriteFile(filepath.Join(rel, "vita-install.json"), []byte(strings.Replace(buildManifestJSON, "%[1]s", strings.Repeat("a", 64), -1)), 0o644)
	vroot := filepath.Join(dir, "vita")
	v, err := fakevita.Start(vroot, true, false)
	if err != nil {
		t.Fatal(err)
	}
	defer v.Close()
	os.MkdirAll(filepath.Join(vroot, "ux0/data/infected"), 0o755)
	os.WriteFile(filepath.Join(vroot, "ux0/data/infected/Infected.iso"), []byte("disc"), 0o644)
	os.WriteFile(filepath.Join(vroot, "ur0/data/libshacccg.suprx"), []byte("shader"), 0o644)

	app := NewApp("1.8", t.TempDir(), t.TempDir())
	if err := app.Add(rel); err != nil {
		t.Fatal(err)
	}
	app.mu.Lock()
	app.st.Vita = &device.Vita{Host: "127.0.0.1", Port: v.Port()}
	it := app.st.Items[0]
	for _, in := range it.Inputs {
		if in.ID == "disc" {
			in.State, in.Message = "skip", "Already on your Vita."
		}
	}
	app.mu.Unlock()
	app.refreshItems()
	app.mu.Lock()
	defer app.mu.Unlock()
	for _, in := range it.Inputs {
		switch in.ID {
		case "disc":
			if in.State != "" {
				t.Errorf("the disc is %q, but a build needs it from this computer", in.State)
			}
		case "shader":
			if in.State != "skip" {
				t.Errorf("the shader compiler already on the Vita should not be asked for (%q)", in.State)
			}
		}
	}
	if it.Status != "needs" {
		t.Errorf("status %q: the game cannot be installed before the disc is chosen and the build allowed", it.Status)
	}
}

// The page's whole path for a game built from the disc: add it, choose the
// disc, allow the build, install; afterwards the app and its artwork are known.
func TestBuildThroughTheApp(t *testing.T) {
	defer install.FakeFreeSpace(100 << 30)() // not a test of this computer's disk
	dir := t.TempDir()
	exe := filepath.Join(dir, "infected-build")
	if runtime.GOOS == "windows" {
		exe += ".exe"
	}
	if out, err := exec.Command("go", "build", "-o", exe, "../install/testdata/fakebuilder").CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	prog, _ := os.ReadFile(exe)
	var kit bytes.Buffer
	zw := zip.NewWriter(&kit)
	h := &zip.FileHeader{Name: filepath.Base(exe), Method: zip.Deflate}
	h.SetMode(0o755)
	w, _ := zw.CreateHeader(h)
	w.Write(prog)
	zw.Close()
	sum := sha256.Sum256(kit.Bytes())

	rel := filepath.Join(dir, "Infected-Vita")
	os.MkdirAll(rel, 0o755)
	key := runtime.GOOS + "-" + runtime.GOARCH
	m, _ := json.Marshal(map[string]interface{}{
		"name": "Infected", "titleId": "INFC00001",
		"build": map[string]interface{}{
			"from": "disc", "kit": map[string]interface{}{key: map[string]interface{}{"file": "kit.zip", "sha256": hex.EncodeToString(sum[:])}},
			"run": "infected-build", "args": []string{"--progress", "--out", "{out}", "{in}"}, "app": "infected.vpk", "minutes": 10,
		},
		"needs": []map[string]interface{}{{"id": "disc", "type": "psp-iso", "gameId": "ULUS-10054", "to": "ux0:data/infected/Infected.iso"}},
	})
	os.WriteFile(filepath.Join(rel, "vita-install.json"), m, 0o644)
	os.WriteFile(filepath.Join(rel, "kit.zip"), kit.Bytes(), 0o644)
	disc := filepath.Join(dir, "Infected.iso")
	os.WriteFile(disc, fakevita.PSPISO("ULUS-10054", "MODE:OK"), 0o644)

	v, err := fakevita.Start(filepath.Join(dir, "vita"), true, false)
	if err != nil {
		t.Fatal(err)
	}
	defer v.Close()
	app := NewApp("1.8", t.TempDir(), t.TempDir())
	app.st.Vita = &device.Vita{Host: "127.0.0.1", Port: v.Port()}
	if err := app.Add(rel); err != nil {
		t.Fatal(err)
	}
	it := app.st.Items[0]
	if err := app.SetInput(it.ID, "disc", disc); err != nil {
		t.Fatal(err)
	}
	if it.Status != "needs" {
		t.Fatalf("status %q before the build is allowed", it.Status)
	}
	if err := app.SetInput(it.ID, "allow-build", "yes"); err != nil {
		t.Fatal(err)
	}
	if it.Status != "ready" {
		t.Fatalf("status %q with everything chosen", it.Status)
	}
	if err := app.Install(it.ID); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 600; i++ {
		app.mu.Lock()
		st := it.Status
		app.mu.Unlock()
		if st == "done" || st == "failed" {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	app.mu.Lock()
	status, errText := it.Status, it.Error
	app.mu.Unlock()
	if status != "done" {
		t.Fatalf("status %q: %s", status, errText)
	}
	if _, err := os.Stat(filepath.Join(v.Root, "ux0/app/INFC00001/eboot.bin")); err != nil {
		t.Fatal("the built app is not on the Vita")
	}
	// Now the app is known: icon, version and artwork preview are available.
	app.mu.Lock()
	known := it.VPK != "" && it.App.Icon != nil && it.Art != nil && it.Art.Preview
	app.mu.Unlock()
	if !known {
		t.Fatalf("the built app was not recorded: %+v %+v", it.Release.App, it.Art)
	}
	// The player can now pick pictures and see them before installing again.
	pic := filepath.Join(dir, "cover.png")
	img := image.NewNRGBA(image.Rect(0, 0, 200, 200))
	for i := range img.Pix {
		img.Pix[i] = uint8(i)
	}
	pf, _ := os.Create(pic)
	png.Encode(pf, img)
	pf.Close()
	if err := app.SetArt(context.Background(), it.ID, "icon", pic, "", ""); err != nil {
		t.Fatal(err)
	}
	if b, err := app.ArtPreview(it.ID, "icon"); err != nil || len(b) == 0 {
		t.Errorf("artwork preview after the build: %v", err)
	}
}
