package release

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const bootSum = "952bb2d4f99c72ee3d6b0db16ce76dc4716f1d6ef2e9fccf9e65a4acba9af000"

func withCheck(m map[string]interface{}, c interface{}) map[string]interface{} {
	m["needs"].([]interface{})[0].(map[string]interface{})["check"] = c
	return m
}

func TestISOCheckParses(t *testing.T) {
	m, err := parse(t, withCheck(buildManifest(), map[string]string{
		"file": "PSP_GAME/SYSDIR/BOOT.BIN", "sha256": strings.ToUpper(bootSum), "message": "This is not the US 1.02 version of Infected (ULUS-10054).",
	}))
	if err != nil {
		t.Fatal(err)
	}
	c := m.Needs[0].Check
	if c == nil || c.File != "PSP_GAME/SYSDIR/BOOT.BIN" || c.SHA256 != bootSum || !strings.Contains(c.Message, "US 1.02") {
		t.Fatalf("check: %+v", c)
	}
}

// A check this installer can't fully understand must not be skipped: it
// could be what keeps the player from building from the wrong disc.
func TestISOCheckUnknownKeyNeedsNewerInstaller(t *testing.T) {
	_, err := parse(t, withCheck(buildManifest(), map[string]string{"file": "A/B", "sha256": bootSum, "algorithm": "blake3"}))
	if err == nil || !strings.Contains(err.Error(), "a newer installer may be required") {
		t.Fatalf("an unknown key in check: %v", err)
	}
	if strings.Contains(err.Error(), "not valid JSON") {
		t.Errorf("should read as a newer-installer message: %v", err)
	}
}

func TestISOCheckIsValidated(t *testing.T) {
	for name, c := range map[string]map[string]string{
		"no file":        {"sha256": bootSum},
		"unsafe path":    {"file": "../BOOT.BIN", "sha256": bootSum},
		"absolute path":  {"file": "/PSP_GAME/SYSDIR/BOOT.BIN", "sha256": bootSum},
		"short hash":     {"file": "A/B", "sha256": "abc"},
		"missing hash":   {"file": "A/B"},
		"endless excuse": {"file": "A/B", "sha256": bootSum, "message": strings.Repeat("x", 400)},
	} {
		if _, err := parse(t, withCheck(buildManifest(), c)); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	// Only a PSP image can be checked this way.
	m := buildManifest()
	n := m["needs"].([]interface{})[1].(map[string]interface{})
	n["check"] = map[string]string{"file": "A/B", "sha256": bootSum}
	if _, err := parse(t, m); err == nil {
		t.Error("a check on a shader need was accepted")
	}
}

func TestBuildRequiresIsStrict(t *testing.T) {
	m := buildManifest()
	m["build"].(map[string]interface{})["requires"] = map[string]interface{}{"macos": 14}
	mf, err := parse(t, m)
	if err != nil || mf.Build.Require == nil || mf.Build.Require.MacOS != 14 {
		t.Fatalf("requires: %v %+v", err, mf)
	}
	m["build"].(map[string]interface{})["requires"] = map[string]interface{}{"macos": 14, "gpu": "yes"}
	if _, err := parse(t, m); err == nil || !strings.Contains(err.Error(), "a newer installer may be required") {
		t.Errorf("unknown requirement: %v", err)
	}
}

func TestWhereTheGameCanBeBuilt(t *testing.T) {
	kit := func(keys ...string) *Build {
		b := &Build{Kit: map[string]KitFile{}}
		for _, k := range keys {
			b.Kit[k] = KitFile{File: k + ".tar.gz", SHA256: sum1}
		}
		return b
	}
	infected := kit("windows-amd64", "darwin-arm64", "darwin-amd64", "linux-amd64")
	infected.Require = &Requires{MacOS: 14}
	const want = "Infected can't be built on this computer yet. It can be built on Windows (64-bit Intel/AMD), macOS 14 or newer, or Linux (64-bit Intel/AMD)."
	for name, tc := range map[string]struct {
		b    *Build
		host Host
		msg  string
	}{
		"windows 64":        {infected, Host{"windows", "amd64", 0}, ""},
		"windows 32":        {infected, Host{"windows", "386", 0}, want},
		"windows arm":       {infected, Host{"windows", "arm64", 0}, want},
		"linux arm":         {infected, Host{"linux", "arm64", 0}, want},
		"linux 64":          {infected, Host{"linux", "amd64", 0}, ""},
		"mac 14":            {infected, Host{"darwin", "arm64", 14}, ""},
		"mac 15 intel":      {infected, Host{"darwin", "amd64", 15}, ""},
		"mac 13":            {infected, Host{"darwin", "arm64", 13}, want},
		"mac 10.15":         {infected, Host{"darwin", "amd64", 10}, want},
		"mac, unknown":      {infected, Host{"darwin", "arm64", 0}, ""},
		"windows 32 allows": {kit("windows-386", "windows-amd64"), Host{"windows", "386", 0}, ""},
	} {
		if got := tc.b.Blocked("Infected", tc.host); got != tc.msg {
			t.Errorf("%s: %q", name, got)
		}
	}
	// A Mac with only one kind of kit still builds on the other kind.
	if got := kit("darwin-amd64").Blocked("Infected", Host{"darwin", "arm64", 14}); got != "" {
		t.Errorf("Apple silicon with an Intel kit: %q", got)
	}
	if got := kit("windows-amd64", "windows-386", "windows-arm64").cannotBuild("Game"); got != "Game can't be built on this computer yet. It can be built on Windows (32-bit), Windows (64-bit Intel/AMD), or Windows (ARM)." {
		t.Errorf("%q", got)
	}
	if got := kit("linux-amd64", "windows-amd64").cannotBuild(""); got != "This game can't be built on this computer yet. It can be built on Windows (64-bit Intel/AMD) or Linux (64-bit Intel/AMD)." {
		t.Errorf("%q", got)
	}
}

func loadWithHost(t *testing.T, h Host, requires bool) *Release {
	t.Helper()
	old := CurrentHost
	CurrentHost = func() Host { return h }
	defer func() { CurrentHost = old }()
	m := buildManifest()
	if requires {
		m["build"].(map[string]interface{})["requires"] = map[string]interface{}{"macos": 14}
	}
	dir := t.TempDir()
	b, _ := json.Marshal(m)
	os.WriteFile(filepath.Join(dir, ManifestName), b, 0o644)
	rs, err := Load(dir, t.TempDir())
	if err != nil || len(rs) != 1 {
		t.Fatalf("%v", err)
	}
	return rs[0]
}

func find(r *Release, id string) *Input {
	for _, in := range r.Inputs {
		if in.ID == id {
			return in
		}
	}
	return nil
}

// A computer that can build asks for the permission and offers, but does not
// require, an app built elsewhere.
func TestReleaseOnAComputerThatBuilds(t *testing.T) {
	r := loadWithHost(t, Host{"windows", "amd64", 0}, true)
	if r.BuildBlock != "" {
		t.Fatalf("blocked: %q", r.BuildBlock)
	}
	built := find(r, BuiltAppID)
	if built == nil || !built.Optional || built.Type != NeedBuiltApp || built.GameID != "INFC00001" || built.Label != "Already built it? Choose the infected.vpk" {
		t.Fatalf("built-app: %+v", built)
	}
	if find(r, BuildConsentID) == nil {
		t.Fatal("no permission to ask for")
	}
	// With an app of their own, the permission is not needed any more.
	find(r, "disc").State = "ok"
	if u := r.Unmet(); u == nil || u.ID != BuildConsentID {
		t.Fatalf("unmet: %+v", u)
	}
	built.State, built.Path = "ok", "/some/infected.vpk"
	if u := r.Unmet(); u != nil {
		t.Fatalf("still waiting for %s", u.ID)
	}
	if r.UsingBuiltApp() != built {
		t.Error("the chosen app is not the one in use")
	}
}

func TestReleaseOnAComputerWithoutKit(t *testing.T) {
	for name, h := range map[string]Host{
		"windows 32":   {"windows", "386", 0},
		"linux arm":    {"linux", "arm64", 0},
		"old mac":      {"darwin", "amd64", 13},
		"windows arm":  {"windows", "arm64", 0},
		"other system": {"freebsd", "amd64", 0},
	} {
		r := loadWithHost(t, h, true)
		const prefix = "Infected can't be built on this computer yet. It can be built on Windows (64-bit Intel/AMD), macOS 14 or newer, or Linux (64-bit Intel/AMD)."
		if r.BuildBlock != prefix {
			t.Errorf("%s: %q", name, r.BuildBlock)
		}
		if find(r, BuildConsentID) != nil {
			t.Errorf("%s: asks permission to build what can't be built", name)
		}
		built := find(r, BuiltAppID)
		if built == nil || built.Optional || !strings.Contains(built.Label, "built on another computer") {
			t.Errorf("%s: the built app must be required: %+v", name, built)
		}
		find(r, "disc").State = "ok"
		if u := r.Unmet(); u == nil || u.ID != BuiltAppID {
			t.Errorf("%s: waiting for %+v", name, u)
		}
	}
}

// The wording about the shader compiler says what the player loses.
func TestOptionalShaderCompilerSaysWhatIsLost(t *testing.T) {
	m, err := parse(t, buildManifest())
	if err != nil {
		t.Fatal(err)
	}
	h := m.Needs[1].Help
	for _, want := range []string{"Recommended", "rarely used effects", "skipped"} {
		if !strings.Contains(h, want) {
			t.Errorf("help %q lacks %q", h, want)
		}
	}
	mm := buildManifest()
	mm["needs"].([]interface{})[1].(map[string]interface{})["help"] = "own words"
	if m, _ := parse(t, mm); m.Needs[1].Help != "own words" {
		t.Errorf("the release's own help was replaced: %q", m.Needs[1].Help)
	}
}
