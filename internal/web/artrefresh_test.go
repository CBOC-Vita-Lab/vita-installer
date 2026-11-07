package web

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/CBOC-Vita-Lab/vita-installer/internal/art"
	"github.com/CBOC-Vita-Lab/vita-installer/internal/device"
	"github.com/CBOC-Vita-Lab/vita-installer/internal/fakevita"
	"github.com/CBOC-Vita-Lab/vita-installer/internal/install"
)

const (
	galaxyRepo = "CBOC-Vita-Lab/galaxy-vita"
	galaxyID   = "SMGV00001"
)

type artEnv struct {
	t     *testing.T
	v     *fakevita.Vita
	app_  *App
	cfg   string
	cache string
	when  time.Time
}

func (e *artEnv) file(rel string) string { return filepath.Join(e.v.Root, filepath.FromSlash(rel)) }

func (e *artEnv) read(rel string) []byte {
	b, err := os.ReadFile(e.file(rel))
	if err != nil {
		e.t.Fatalf("%s: %v", rel, err)
	}
	return b
}

func (e *artEnv) write(rel string, b []byte) {
	p := e.file(rel)
	os.MkdirAll(filepath.Dir(p), 0o755)
	if err := os.WriteFile(p, b, 0o644); err != nil {
		e.t.Fatal(err)
	}
}

// newArtEnv has a fake Vita with an installed game (the plain star) and a
// receipt from this installer.
func newArtEnv(t *testing.T, title, repo string) *artEnv {
	dir := t.TempDir()
	v, err := fakevita.Start(filepath.Join(dir, "vita"), true, false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(v.Close)
	e := &artEnv{t: t, v: v, cfg: t.TempDir(), cache: t.TempDir(), when: time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)}
	app := "ux0/app/" + title
	e.write(app+"/eboot.bin", []byte("game"))
	e.write(app+"/sce_sys/icon0.png", []byte("star-icon"))
	e.write(app+"/sce_sys/pic0.png", []byte("star-pic0"))
	e.write(app+"/sce_sys/livearea/contents/template.xml", []byte("<livearea/>"))
	e.write(app+"/sce_sys/livearea/contents/bg.png", []byte("star-bg"))
	e.write(app+"/sce_sys/livearea/contents/startup.png", []byte("star-startup"))
	e.write("ur0/appmeta/"+title+"/icon0.png", []byte("star-icon"))
	e.write("ux0/data/smg/save.bin", []byte("my save"))
	e.receipt(title, repo)
	e.app(true)
	return e
}

func (e *artEnv) receipt(title, repo string) {
	rc := &install.Receipt{Name: "Game " + title, TitleID: title, Repo: repo, Release: "v1", InstalledAt: e.when, Files: map[string]string{}, Icon: sha256Hex([]byte("star-icon"))}
	b, _ := json.MarshalIndent(rc, "", " ")
	e.write("ux0/data/cboc-installer/installed/"+title+".json", b)
}

// app starts the installer (as if reopened) connected to the Vita.
func (e *artEnv) app(connect bool) {
	app := NewApp("2.0", e.cache, e.cfg)
	app.tags = map[string]latestTag{galaxyRepo: {"v1", time.Now()}, "CBOC-Vita-Lab/strikers-charged-vita": {"v1", time.Now()}, "other/thing": {"v1", time.Now()}}
	app.st.Vita = &device.Vita{Host: "127.0.0.1", Port: e.v.Port()}
	e.app_ = app
	_ = connect
}

// connect is what happens when the Vita is found.
func (e *artEnv) connect() { e.app_.refreshLibrary() }

func (e *artEnv) stores() int32 { return e.v.Stores }

func (e *artEnv) count(cmds ...string) int {
	n := 0
	for _, c := range e.v.Commands() {
		for _, w := range cmds {
			if c == w {
				n++
			}
		}
	}
	return n
}

func capture(seed byte) []byte {
	b := make([]byte, 960*544*4)
	for i := 0; i < len(b); i += 4 {
		x, y := (i/4)%960, (i/4)/960
		b[i], b[i+1], b[i+2] = byte(x/4)+seed, byte(y/2)^seed, seed*3+byte(x*y)
	}
	return b
}

func isVitaPNG(t *testing.T, b []byte, w, h int) {
	t.Helper()
	img, err := png.Decode(bytes.NewReader(b))
	if err != nil {
		t.Fatalf("not a PNG: %v", err)
	}
	p, ok := img.(*image.Paletted)
	if !ok || len(p.Palette) != 256 || img.Bounds().Dx() != w || img.Bounds().Dy() != h {
		t.Fatalf("not an 8-bit palettized %dx%d PNG (%T %v)", w, h, img, img.Bounds())
	}
}

func TestArtRefreshOnConnect(t *testing.T) {
	e := newArtEnv(t, galaxyID, galaxyRepo)
	e.write("ux0/data/smg/art/title.rgba", capture(1))
	icon := "ux0/app/" + galaxyID + "/sce_sys/icon0.png"
	meta := "ur0/appmeta/" + galaxyID + "/icon0.png"
	bg := "ux0/app/" + galaxyID + "/sce_sys/livearea/contents/bg.png"
	marker := "ux0/data/cboc-installer/art/" + galaxyID + ".json"

	// 1. First connect: the pictures are written and the marker is kept.
	e.connect()
	isVitaPNG(t, e.read(icon), art.IconW, art.IconH)
	isVitaPNG(t, e.read(bg), art.BGW, art.BGH)
	isVitaPNG(t, e.read("ux0/app/"+galaxyID+"/sce_sys/livearea/contents/startup.png"), art.StartupW, art.StartupH)
	isVitaPNG(t, e.read("ux0/app/"+galaxyID+"/sce_sys/pic0.png"), art.Pic0W, art.Pic0H)
	if !bytes.Equal(e.read(icon), e.read(meta)) {
		t.Fatal("the system's copy of the icon is not the same picture")
	}
	if _, err := os.Stat(e.file("ur0/appmeta/" + galaxyID + "/livearea")); err == nil {
		t.Fatal("a livearea folder was made in appmeta")
	}
	m := string(e.read(marker))
	for _, want := range []string{"capture title.rgba", `"sha256"`, `"install"`} {
		if !strings.Contains(m, want) {
			t.Fatalf("marker lacks %q: %s", want, m)
		}
	}
	// Only picture files were touched; the old ones were kept once.
	if string(e.read("ux0/app/"+galaxyID+"/eboot.bin")) != "game" || string(e.read("ux0/data/smg/save.bin")) != "my save" {
		t.Fatal("game files changed")
	}
	bk := "ux0/data/cboc-installer/art-backup/" + galaxyID + "/"
	if string(e.read(bk+"0-icon0.png")) != "star-icon" || string(e.read(bk+"1-bg.png")) != "star-bg" {
		t.Fatal("previous pictures were not kept")
	}
	e.read(bk + "backup.json")
	filepath.Walk(e.v.Root, func(p string, info os.FileInfo, err error) error {
		if err == nil && strings.HasSuffix(p, ".cboc-new") {
			t.Errorf("temporary file left: %s", p)
		}
		return nil
	})
	if !strings.Contains(e.app_.st.Notice, "restart your Vita") {
		t.Fatalf("no notice about restarting: %q", e.app_.st.Notice)
	}
	firstStores := e.stores()
	if firstStores < 6 { // 5 pictures + marker, plus the backup
		t.Fatalf("only %d uploads", firstStores)
	}
	firstIcon := append([]byte(nil), e.read(icon)...)

	// 2. Second connect, nothing changed: no uploads at all, only reading.
	before := len(e.v.Commands())
	e.app_.st.Notice = ""
	e.connect()
	if e.stores() != firstStores {
		t.Fatal("pictures were sent again")
	}
	ops := e.v.Commands()[before:]
	for _, c := range ops {
		switch c {
		case "STOR", "DELE", "RNFR", "RNTO", "MKD", "RMD", "PROM":
			t.Fatalf("unchanged Vita but the installer sent %s (%v)", c, ops)
		}
	}
	t.Logf("FTP commands for a connect with nothing changed: %v", ops)
	if e.app_.st.Notice != "" {
		t.Fatal("told the player about a refresh that did not happen")
	}

	// 3. The game saved a new title picture: one refresh, backup not redone.
	e.write("ux0/data/smg/art/title.rgba", capture(90))
	future := time.Now().Add(3 * time.Hour)
	os.Chtimes(e.file("ux0/data/smg/art/title.rgba"), future, future)
	e.connect()
	if e.stores() == firstStores {
		t.Fatal("a changed picture was not sent")
	}
	second := e.stores()
	if bytes.Equal(e.read(icon), firstIcon) {
		t.Fatal("the bubble did not change")
	}
	if !bytes.Equal(e.read(icon), e.read(meta)) {
		t.Fatal("copies differ")
	}
	if string(e.read(bk+"0-icon0.png")) != "star-icon" {
		t.Fatal("the backup was overwritten")
	}
	e.connect()
	if e.stores() != second {
		t.Fatal("sent again after the refresh")
	}

	// 4. The game was installed again (a new receipt): the pictures go back on.
	e.when = e.when.Add(time.Hour)
	e.write(icon, []byte("star-icon"))
	e.receipt(galaxyID, galaxyRepo)
	e.connect()
	isVitaPNG(t, e.read(icon), art.IconW, art.IconH)
	if string(e.read(bk+"0-icon0.png")) != "star-icon" {
		t.Fatal("backup changed")
	}
}

func TestArtRefreshSetting(t *testing.T) {
	e := newArtEnv(t, galaxyID, galaxyRepo)
	e.write("ux0/data/smg/art/title.rgba", capture(1))
	e.app_.SetArtAuto(false)
	e.app_.bg.Wait()
	e.connect()
	if e.stores() != 0 || e.count("STOR", "DELE", "MKD") != 0 {
		t.Fatal("did something with the setting off")
	}
	// The choice is kept for next time.
	e.app(true)
	if e.app_.st.ArtAuto {
		t.Fatal("setting was not remembered")
	}
	e.connect()
	if e.stores() != 0 {
		t.Fatal("did something with the setting off")
	}
	// The button works anyway, and turning it on again refreshes.
	if err := e.app_.RefreshArt(context.Background()); err != nil {
		t.Fatal(err)
	}
	if e.stores() == 0 {
		t.Fatal("manual refresh did nothing")
	}
	n := e.stores()
	if err := e.app_.RefreshArt(context.Background()); err != nil {
		t.Fatal(err)
	}
	if e.stores() <= n {
		t.Fatal("manual refresh should always write")
	}
	e.app_.SetArtAuto(true)
	e.app_.bg.Wait()
	e.app(true)
	if !e.app_.st.ArtAuto {
		t.Fatal("setting was not remembered")
	}
}

func TestArtRefreshKeepsPlayersChoice(t *testing.T) {
	e := newArtEnv(t, galaxyID, galaxyRepo)
	e.write("ux0/data/smg/art/title.rgba", capture(1))
	pic := filepath.Join(t.TempDir(), "mine.png")
	img := image.NewNRGBA(image.Rect(0, 0, 64, 64))
	img.Set(3, 3, color.White)
	var b bytes.Buffer
	png.Encode(&b, img)
	os.WriteFile(pic, b.Bytes(), 0o644)
	e.app_.saveArt(galaxyID, savedArt{Choice: art.Choice{Icon: pic}, Labels: map[string]string{}})
	e.connect()
	if err := e.app_.RefreshArt(context.Background()); err != nil {
		t.Fatal(err)
	}
	if e.stores() != 0 {
		t.Fatal("replaced art the player chose")
	}
}

// Other games: screenshots are the source; games the installer did not put
// there, or does not manage, are left alone.
func TestArtRefreshScreenshotsAndOthers(t *testing.T) {
	const id = "MSCE00001"
	e := newArtEnv(t, id, "CBOC-Vita-Lab/strikers-charged-vita")
	e.receipt("OTHR00001", "other/thing")
	e.write("ux0/app/OTHR00001/sce_sys/icon0.png", []byte("x"))
	var jb bytes.Buffer
	png.Encode(&jb, image.NewNRGBA(image.Rect(0, 0, 960, 544)))
	e.write("ux0/picture/SCREENSHOT/"+id+"/2026-11-05-100000.png", jb.Bytes())
	e.write("ux0/picture/SCREENSHOT/OTHR00001/2026-11-05-100000.png", jb.Bytes())
	e.connect()
	isVitaPNG(t, e.read("ux0/app/"+id+"/sce_sys/icon0.png"), art.IconW, art.IconH)
	if string(e.read("ux0/app/OTHR00001/sce_sys/icon0.png")) != "x" {
		t.Fatal("touched a game that is not managed")
	}
	if !strings.Contains(string(e.read("ux0/data/cboc-installer/art/"+id+".json")), "shot 2026-11-05-100000.png") {
		t.Fatal("marker does not name the screenshot")
	}
	// A newer screenshot is picked up.
	n := e.stores()
	e.write("ux0/picture/SCREENSHOT/"+id+"/2026-11-06-090000.png", jb.Bytes())
	e.connect()
	if e.stores() == n {
		t.Fatal("newer screenshot not used")
	}
	// A game with no pictures at all is left alone, with one listing.
	e2 := newArtEnv(t, id, "CBOC-Vita-Lab/strikers-charged-vita")
	e2.connect()
	if e2.stores() != 0 {
		t.Fatal("wrote without a picture")
	}
}

// A bubble the player changed by hand (VitaShell, another tool, their own
// art) is never replaced automatically; pressing Refresh artwork still does.
func TestArtRefreshKeepsHandMadeArt(t *testing.T) {
	e := newArtEnv(t, galaxyID, galaxyRepo)
	e.write("ux0/data/smg/art/title.rgba", capture(1))
	icon := "ux0/app/" + galaxyID + "/sce_sys/icon0.png"
	e.write(icon, []byte("my-own-icon"))
	e.connect()
	if got := string(e.read(icon)); got != "my-own-icon" {
		t.Fatalf("hand-made bubble replaced on connect")
	}
	// After a refresh, a later change by hand is kept too.
	e.write(icon, []byte("star-icon"))
	e.connect()
	isVitaPNG(t, e.read(icon), art.IconW, art.IconH)
	e.write(icon, []byte("changed-again"))
	e.write("ux0/data/smg/art/title.rgba", capture(9))
	e.app(true)
	e.connect()
	if got := string(e.read(icon)); got != "changed-again" {
		t.Fatalf("bubble changed by hand after a refresh was replaced")
	}
	if err := e.app_.RefreshArt(context.Background()); err != nil {
		t.Fatal(err)
	}
	isVitaPNG(t, e.read(icon), art.IconW, art.IconH)
}
