package install

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CBOC-Vita-Lab/vita-installer/internal/fakevita"
	"github.com/CBOC-Vita-Lab/vita-installer/internal/release"
)

const bootFile = "PSP_GAME/SYSDIR/BOOT.BIN"

var goodBoot = []byte("boot file of the US 1.02 version")

func sumOf(b []byte) string { s := sha256.Sum256(b); return hex.EncodeToString(s[:]) }

func isoWithBoot(boot []byte, marker string) []byte {
	return fakevita.PSPISOFiles("ULUS-10054", marker, map[string][]byte{bootFile: boot, "PSP_GAME/PARAM.SFO": []byte("sfo")})
}

func checkedNeed() *release.Input {
	return &release.Input{Need: release.Need{
		ID: "disc", Type: release.NeedPSPISO, GameID: "ULUS-10054",
		Check: &release.ISOCheck{File: bootFile, SHA256: sumOf(goodBoot), Message: "This is not the US 1.02 version of Infected (ULUS-10054)."},
	}}
}

func writeTemp(t *testing.T, name string, data []byte) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// The disc is checked the moment it is chosen, so the row says at once
// whether it is the right version.
func TestDiscVersionCheck(t *testing.T) {
	good := isoWithBoot(goodBoot, "")
	cases := []struct {
		name  string
		data  []byte
		state string
		say   string
	}{
		{"right version", good, "ok", "found and checked"},
		{"other version", isoWithBoot([]byte("the EU version"), ""), "error", "This is not the US 1.02 version of Infected (ULUS-10054)."},
		{"no boot file", fakevita.PSPISOFiles("ULUS-10054", "", map[string][]byte{"PSP_GAME/PARAM.SFO": []byte("x")}), "error", "This is not the US 1.02 version"},
		{"cut short", good[:len(good)-4096], "error", "damaged or incomplete"},
		{"cso", append([]byte("CISO"), make([]byte, 50000)...), "error", "CSO and PBP files are not supported"},
		{"pbp", append([]byte("\x00PBP"), make([]byte, 50000)...), "error", "CSO and PBP files are not supported"},
		{"other game", fakevita.PSPISOFiles("ULES-00000", "", map[string][]byte{bootFile: goodBoot}), "error", "This is ULES-00000, but this release needs ULUS-10054."},
	}
	for _, c := range cases {
		in := checkedNeed()
		CheckInput(in, writeTemp(t, "g.iso", c.data))
		if in.State != c.state || !strings.Contains(in.Message, c.say) {
			t.Errorf("%s: %s %q", c.name, in.State, in.Message)
		}
	}
	// Without a check in the release, any version of the right game is fine.
	in := checkedNeed()
	in.Check = nil
	CheckInput(in, writeTemp(t, "g.iso", isoWithBoot([]byte("anything"), "")))
	if in.State != "ok" {
		t.Errorf("no check: %s %q", in.State, in.Message)
	}
	// A release that gives no message of its own still says something.
	in = checkedNeed()
	in.Check.Message = ""
	CheckInput(in, writeTemp(t, "g.iso", isoWithBoot([]byte("EU"), "")))
	if in.State != "error" || in.Message == "" {
		t.Errorf("default message: %s %q", in.State, in.Message)
	}
}

// With a wrong disc nothing slow starts: no build tools are downloaded or
// unpacked, and the builder never runs. The same holds when the disc is
// swapped after it was chosen.
func TestWrongDiscStartsNothing(t *testing.T) {
	f := newBuildFixture(t, true, "MODE:OK")
	m, _ := os.ReadFile(filepath.Join(f.release, release.ManifestName))
	var mm map[string]interface{}
	json.Unmarshal(m, &mm)
	mm["needs"].([]interface{})[0].(map[string]interface{})["check"] = map[string]string{"file": bootFile, "sha256": sumOf(goodBoot), "message": "This is not the US 1.02 version of Infected (ULUS-10054)."}
	m, _ = json.Marshal(mm)
	os.WriteFile(filepath.Join(f.release, release.ManifestName), m, 0o644)
	_, vita := startVita(t, f.dir)

	choose := func(r *release.Release, data []byte) *release.Input {
		os.WriteFile(f.disc, data, 0o644)
		for _, in := range r.Inputs {
			switch in.ID {
			case "disc":
				CheckInput(in, f.disc)
			case release.BuildConsentID:
				CheckInput(in, "yes")
			}
		}
		return find(r, "disc")
	}
	noKit := func() {
		t.Helper()
		if _, err := os.Stat(filepath.Join(f.cache, "kits")); err == nil {
			t.Error("the build tools were unpacked for a wrong disc")
		}
		if n := f.runCount(t); n != 0 {
			t.Errorf("the builder ran %d times", n)
		}
	}

	r := load(t, f.release)
	if in := choose(r, isoWithBoot([]byte("EU"), "MODE:OK")); in.State != "error" || !strings.Contains(in.Message, "US 1.02") {
		t.Fatalf("wrong disc: %s %q", in.State, in.Message)
	}
	if _, err := Run(context.Background(), r, Options{Vita: vita, Cache: f.cache}); err == nil || !strings.Contains(err.Error(), "still needed") {
		t.Fatalf("a wrong disc was accepted: %v", err)
	}
	noKit()

	// The right disc is chosen, then replaced by another version.
	r = load(t, f.release)
	if in := choose(r, isoWithBoot(goodBoot, "MODE:OK")); in.State != "ok" {
		t.Fatalf("right disc: %s %q", in.State, in.Message)
	}
	os.WriteFile(f.disc, isoWithBoot([]byte("EU"), "MODE:OK"), 0o644)
	if _, err := Run(context.Background(), r, Options{Vita: vita, Cache: f.cache}); err == nil || !strings.Contains(err.Error(), "US 1.02") {
		t.Fatalf("a swapped disc was accepted: %v", err)
	}
	noKit()

	// And the right disc builds as usual.
	os.WriteFile(f.disc, isoWithBoot(goodBoot, "MODE:OK"), 0o644)
	if res, err := Run(context.Background(), r, Options{Vita: vita, Cache: f.cache}); err != nil || !res.Installed {
		t.Fatalf("right disc: %v %+v", err, res)
	}
	if n := f.runCount(t); n != 1 {
		t.Errorf("the builder ran %d times", n)
	}
}

func find(r *release.Release, id string) *release.Input {
	for _, in := range r.Inputs {
		if in.ID == id {
			return in
		}
	}
	return nil
}

func setHost(t *testing.T, h release.Host) {
	t.Helper()
	old := release.CurrentHost
	release.CurrentHost = func() release.Host { return h }
	t.Cleanup(func() { release.CurrentHost = old })
}

func builtApp(t *testing.T, titleID string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "infected.vpk")
	if err := fakevita.MakeVPK(p, titleID, "Infected", map[string][]byte{"module/infected_core3.skprx": []byte("core")}); err != nil {
		t.Fatal(err)
	}
	return p
}

// On a computer with no build tools for it, the game says so first, asks for
// no build and no download, and installs an app built elsewhere.
func TestBuiltAppOnAComputerWithoutKit(t *testing.T) {
	for name, h := range map[string]release.Host{
		"windows 32-bit": {OS: "windows", Arch: "386"},
		"linux arm":      {OS: "linux", Arch: "arm64"},
	} {
		t.Run(name, func(t *testing.T) {
			f := newBuildFixture(t, false, "MODE:OK")
			// This computer has a kit entry in the fixture only for itself.
			setHost(t, h)
			r := load(t, f.release)
			if !strings.HasPrefix(r.BuildBlock, "Infected can't be built on this computer yet. It can be built on ") {
				t.Fatalf("message: %q", r.BuildBlock)
			}
			if find(r, release.BuildConsentID) != nil {
				t.Error("asks to allow a build that can't happen")
			}
			CheckInput(find(r, "disc"), f.disc)
			if u := r.Unmet(); u == nil || u.ID != release.BuiltAppID {
				t.Fatalf("waiting for %+v", u)
			}
			v, vita := startVita(t, f.dir)
			if _, err := Run(context.Background(), r, Options{Vita: vita, Cache: f.cache}); err == nil {
				t.Fatal("installed without an app")
			}

			// A file that is not the right app is refused on the spot.
			in := find(r, release.BuiltAppID)
			CheckInput(in, builtApp(t, "ABCD00001"))
			if in.State != "error" || !strings.Contains(in.Message, "ABCD00001") {
				t.Errorf("wrong app: %s %q", in.State, in.Message)
			}
			CheckInput(in, writeTemp(t, "infected.vpk", []byte("not a zip")))
			if in.State != "error" {
				t.Errorf("junk accepted: %s", in.State)
			}
			CheckInput(in, t.TempDir())
			if in.State != "error" {
				t.Errorf("a folder accepted")
			}

			CheckInput(in, builtApp(t, "INFC00001"))
			if in.State != "ok" {
				t.Fatalf("right app: %s %q", in.State, in.Message)
			}
			if u := r.Unmet(); u != nil {
				t.Fatalf("still waiting for %s", u.ID)
			}
			res, err := Run(context.Background(), r, Options{Vita: vita, Cache: f.cache})
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
				t.Error("the disc was not copied to the Vita")
			}
			if _, err := os.Stat(filepath.Join(f.cache, "kits")); err == nil {
				t.Error("build tools were fetched")
			}
		})
	}
}

// A Mac older than the kits need gets the same answer and no download, even
// though a kit exists for its kind of Mac.
func TestOldMacBuildsNothing(t *testing.T) {
	f := newBuildFixture(t, false, "MODE:OK")
	m, _ := os.ReadFile(filepath.Join(f.release, release.ManifestName))
	var mm map[string]interface{}
	json.Unmarshal(m, &mm)
	key := "darwin-arm64"
	kits := mm["build"].(map[string]interface{})["kit"].(map[string]interface{})
	for k, v := range kits {
		delete(kits, k)
		kits[key] = v
	}
	mm["build"].(map[string]interface{})["requires"] = map[string]interface{}{"macos": 14}
	m, _ = json.Marshal(mm)
	os.WriteFile(filepath.Join(f.release, release.ManifestName), m, 0o644)

	setHost(t, release.Host{OS: "darwin", Arch: "arm64", MacMajor: 13})
	r := load(t, f.release)
	if want := "Infected can't be built on this computer yet. It can be built on macOS 14 or newer."; r.BuildBlock != want {
		t.Fatalf("message %q", r.BuildBlock)
	}
	// Even if the page allowed it, building is refused before any download.
	os.WriteFile(filepath.Join(f.release, "x"), nil, 0o644)
	for _, in := range r.Inputs {
		if in.ID == "disc" {
			CheckInput(in, f.disc)
		}
	}
	r.Inputs = append(r.Inputs, &release.Input{Need: release.Need{ID: release.BuildConsentID, Type: release.NeedConsent}, State: "ok", Path: "yes"})
	find(r, release.BuiltAppID).Optional = true
	_, vita := startVita(t, f.dir)
	_, err := Run(context.Background(), r, Options{Vita: vita, Cache: f.cache})
	if err == nil || !strings.Contains(err.Error(), "can't be built on this computer yet") {
		t.Fatalf("an old Mac built: %v", err)
	}
	if _, err := os.Stat(filepath.Join(f.cache, "kits")); err == nil {
		t.Error("build tools were fetched for a Mac that can't run them")
	}

	// macOS 14 is fine.
	setHost(t, release.Host{OS: "darwin", Arch: "arm64", MacMajor: 14})
	if r := load(t, f.release); r.BuildBlock != "" {
		t.Errorf("macOS 14 blocked: %q", r.BuildBlock)
	}
}

// A built app brought by the player skips the build even where kits exist, and
// is told apart from the release's own build when updating.
func TestBuiltAppSkipsTheBuild(t *testing.T) {
	f := newBuildFixture(t, true, "MODE:OK")
	r := f.load(t, false) // the build is not allowed, and does not need to be
	in := find(r, release.BuiltAppID)
	if in == nil || !in.Optional {
		t.Fatalf("built-app: %+v", in)
	}
	if r.Unmet() == nil {
		t.Fatal("ready without permission or app")
	}
	CheckInput(in, builtApp(t, "INFC00001"))
	if r.Unmet() != nil {
		t.Fatal("not ready with an app of their own")
	}
	v, vita := startVita(t, f.dir)
	res, err := Run(context.Background(), r, Options{Vita: vita, Cache: f.cache})
	if err != nil || !res.Installed {
		t.Fatalf("%v %+v", err, res)
	}
	if n := f.runCount(t); n != 0 {
		t.Errorf("the builder ran %d times", n)
	}
	if _, err := os.Stat(filepath.Join(f.cache, "kits")); err == nil {
		t.Error("the build tools were unpacked")
	}
	mustFile(t, v.Root, "ux0/app/INFC00001/eboot.bin")
	// This is not the same as the release's own build.
	p, err := Check(context.Background(), vita, "", load(t, f.release))
	if err != nil || !p.Installed || p.Same {
		t.Errorf("an app built elsewhere counted as this release's build: %v %+v", err, p)
	}
}

// ---- the shader compiler ----

var shaderBytes = bytes.Repeat([]byte{7}, 200<<10)

func zipOf(t *testing.T, files map[string][]byte) string {
	t.Helper()
	var b bytes.Buffer
	z := zip.NewWriter(&b)
	for n, d := range files {
		w, _ := z.Create(n)
		w.Write(d)
	}
	z.Close()
	return writeTemp(t, "download.zip", b.Bytes())
}

func TestShaderCompilerSources(t *testing.T) {
	dir := t.TempDir()
	plain := filepath.Join(dir, "libshacccg.suprx")
	os.WriteFile(plain, shaderBytes, 0o644)
	nested := filepath.Join(dir, "folder", "sub", "libshacccg.suprx")
	os.MkdirAll(filepath.Dir(nested), 0o755)
	os.WriteFile(nested, shaderBytes, 0o644)
	other := filepath.Join(dir, "other.suprx")
	os.WriteFile(other, shaderBytes, 0o644)
	tiny := filepath.Join(dir, "tiny", "libshacccg.suprx")
	os.MkdirAll(filepath.Dir(tiny), 0o755)
	os.WriteFile(tiny, []byte("tiny"), 0o644)
	empty := filepath.Join(dir, "empty")
	os.MkdirAll(empty, 0o755)

	for name, tc := range map[string]struct {
		path string
		ok   bool
	}{
		"the file":                  {plain, true},
		"a folder that has it":      {filepath.Join(dir, "folder"), true},
		"a ZIP that has it":         {zipOf(t, map[string][]byte{"readme.txt": []byte("hi"), "data/ur0/data/libshacccg.suprx": shaderBytes}), true},
		"a ZIP, name in any case":   {zipOf(t, map[string][]byte{"LibShaCCCg.SUPRX": shaderBytes}), true},
		"a ZIP without it":          {zipOf(t, map[string][]byte{"other.suprx": shaderBytes, "libshacccg.txt": shaderBytes}), false},
		"another .suprx":            {other, false},
		"a file that is too small":  {filepath.Join(dir, "tiny", "libshacccg.suprx"), false},
		"a folder that lacks it":    {empty, false},
		"a file that is not there":  {filepath.Join(dir, "nope"), false},
		"a folder with a tiny copy": {filepath.Join(dir, "tiny"), false},
	} {
		in := &release.Input{Need: release.Need{Type: release.NeedShader}}
		CheckInput(in, tc.path)
		if (in.State == "ok") != tc.ok {
			t.Errorf("%s: %s %q", name, in.State, in.Message)
		}
		if !tc.ok && !strings.Contains(in.Message, "libshacccg.suprx") && in.State == "error" && !strings.Contains(in.Message, "cannot be opened") {
			t.Errorf("%s: unhelpful message %q", name, in.Message)
		}
	}
}

func shaderRelease(t *testing.T) (*release.Release, string) {
	t.Helper()
	dir := t.TempDir()
	rel := filepath.Join(dir, "Game")
	os.MkdirAll(rel, 0o755)
	if err := fakevita.MakeVPK(filepath.Join(rel, "Game.vpk"), "GAME00001", "Game", nil); err != nil {
		t.Fatal(err)
	}
	m, _ := json.Marshal(map[string]interface{}{"needs": []map[string]interface{}{{"id": "shader", "type": "shader-compiler", "optional": true}}})
	os.WriteFile(filepath.Join(rel, release.ManifestName), m, 0o644)
	return load(t, rel), dir
}

// The compiler is taken out of a ZIP and put where games look for it, and
// the installer then knows the Vita has it.
func TestShaderCompilerFromAZip(t *testing.T) {
	for name, pack := range map[string]func() string{
		"zip": func() string { return zipOf(t, map[string][]byte{"extra/libshacccg.suprx": shaderBytes}) },
		"folder": func() string {
			d := t.TempDir()
			os.WriteFile(filepath.Join(d, "libshacccg.suprx"), shaderBytes, 0o644)
			return d
		},
	} {
		t.Run(name, func(t *testing.T) {
			r, dir := shaderRelease(t)
			if r.Unmet() != nil {
				t.Fatal("an optional shader compiler blocks the install")
			}
			CheckInput(find(r, "shader"), pack())
			v, vita := startVita(t, dir)
			cache := filepath.Join(dir, "cache")
			if _, err := Run(context.Background(), r, Options{Vita: vita, Cache: cache}); err != nil {
				t.Fatal(err)
			}
			got, err := os.ReadFile(filepath.Join(v.Root, "ur0/data/libshacccg.suprx"))
			if err != nil || !bytes.Equal(got, shaderBytes) {
				t.Fatalf("shader compiler on the Vita: %v (%d bytes)", err, len(got))
			}
			if left, _ := filepath.Glob(filepath.Join(cache, "inputs", "*")); len(left) > 0 {
				t.Errorf("unpacked copies left behind: %v", left)
			}
			if st := CheckShader(context.Background(), vita, ""); !st.Known || !st.Present {
				t.Errorf("status %+v", st)
			}
		})
	}
}

func TestShaderStatus(t *testing.T) {
	dir := t.TempDir()
	v, vita := startVita(t, dir)
	if st := CheckShader(context.Background(), vita, ""); !st.Known || st.Present || st.Helper {
		t.Fatalf("a bare Vita: %+v", st)
	}
	os.MkdirAll(filepath.Join(v.Root, "ux0/app/SHRKBR33D"), 0o755)
	os.WriteFile(filepath.Join(v.Root, "ux0/app/SHRKBR33D/eboot.bin"), []byte("x"), 0o644)
	if st := CheckShader(context.Background(), vita, ""); !st.Known || st.Present || !st.Helper {
		t.Fatalf("with ShaRKBR33D: %+v", st)
	}
	os.MkdirAll(filepath.Join(v.Root, "ur0/data"), 0o755)
	os.WriteFile(filepath.Join(v.Root, "ur0/data/libshacccg.suprx"), shaderBytes, 0o644)
	if st := CheckShader(context.Background(), vita, ""); !st.Present {
		t.Fatalf("with the compiler: %+v", st)
	}
	// A drive only shows ux0:, so the compiler can't be seen there.
	if st := CheckShader(context.Background(), nil, filepath.Join(v.Root, "ux0")); st.Known || st.Present {
		t.Errorf("drive: %+v", st)
	}
	if st := CheckShader(context.Background(), nil, ""); st.Known {
		t.Errorf("no Vita: %+v", st)
	}
}

// A kit that says it needs a newer macOS than this Mac has is not used, even
// if the release did not say so.
func TestKitCanDemandANewerMac(t *testing.T) {
	r := &release.Release{Name: "Infected", Build: &release.Build{Kit: map[string]release.KitFile{"darwin-arm64": {File: "k", SHA256: strings.Repeat("a", 64)}}}}
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "kit.json"), []byte(`{"requires":{"macos":14}}`), 0o644)
	setHost(t, release.Host{OS: "darwin", Arch: "arm64", MacMajor: 13})
	if msg := kitRequirement(dir, r); msg != "Infected can't be built on this computer yet. It can be built on macOS 14 or newer." {
		t.Errorf("macOS 13: %q", msg)
	}
	setHost(t, release.Host{OS: "darwin", Arch: "arm64", MacMajor: 15})
	if msg := kitRequirement(dir, r); msg != "" {
		t.Errorf("macOS 15: %q", msg)
	}
	setHost(t, release.Host{OS: "linux", Arch: "amd64"})
	if msg := kitRequirement(dir, r); msg != "" && !strings.Contains(msg, "can't be built") {
		t.Errorf("%q", msg)
	}
	if msg := kitRequirement(t.TempDir(), r); msg != "" {
		t.Errorf("a kit without kit.json: %q", msg)
	}
}

// The games also load the compiler from ur0:data/external/, and a file cut
// short is reported as damaged instead of present.
func TestShaderStatusPlaces(t *testing.T) {
	dir := t.TempDir()
	v, vita := startVita(t, dir)
	real := bytes.Repeat([]byte{0x7f}, shaderMinBytes+1)
	os.MkdirAll(filepath.Join(v.Root, "ur0/data/external"), 0o755)
	os.WriteFile(filepath.Join(v.Root, "ur0/data/external/libshacccg.suprx"), real, 0o644)
	if st := CheckShader(context.Background(), vita, ""); !st.Present || st.Damaged {
		t.Fatalf("in external/: %+v", st)
	}
	os.Remove(filepath.Join(v.Root, "ur0/data/external/libshacccg.suprx"))
	os.WriteFile(filepath.Join(v.Root, "ur0/data/libshacccg.suprx"), nil, 0o644)
	if st := CheckShader(context.Background(), vita, ""); st.Present || !st.Damaged {
		t.Fatalf("an empty file: %+v", st)
	}
	os.WriteFile(filepath.Join(v.Root, "ur0/data/libshacccg.suprx"), real, 0o644)
	if st := CheckShader(context.Background(), vita, ""); !st.Present || st.Damaged {
		t.Fatalf("a whole file: %+v", st)
	}
}
