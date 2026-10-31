package release

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const sum1 = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
const sum2 = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"

// buildManifest returns a valid build-only release file as a map, so a test
// can break one thing at a time.
func buildManifest() map[string]interface{} {
	return map[string]interface{}{
		"name":    "Infected",
		"titleId": "INFC00001",
		"build": map[string]interface{}{
			"from": "disc",
			"kit": map[string]interface{}{
				"windows-amd64": map[string]interface{}{"file": "Infected-Vita-builder-windows.zip", "sha256": sum1, "size": 300 << 20},
				"darwin-amd64":  map[string]interface{}{"file": "Infected-Vita-builder-mac.zip", "sha256": sum2, "size": 250 << 20},
				"linux-amd64":   map[string]interface{}{"file": "Infected-Vita-builder-linux.zip", "sha256": sum1, "size": 280 << 20},
			},
			"run":     "infected-build",
			"args":    []string{"--progress", "--out", "{out}", "{in}"},
			"app":     "infected.vpk",
			"label":   "Building Infected from your disc",
			"minutes": 10,
		},
		"needs": []interface{}{
			map[string]interface{}{"id": "disc", "type": "psp-iso", "gameId": "ULUS-10054", "to": "ux0:data/infected/Infected.iso", "label": "Your Infected UMD backup (PSP, USA, ISO)"},
			map[string]interface{}{"id": "shader", "type": "shader-compiler", "optional": true},
		},
	}
}

func parse(t *testing.T, m map[string]interface{}) (*Manifest, error) {
	t.Helper()
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	return ParseManifest(b)
}

func TestBuildManifestParses(t *testing.T) {
	m, err := parse(t, buildManifest())
	if err != nil {
		t.Fatal(err)
	}
	b := m.Build
	if b == nil || b.From != "disc" || b.Run != "infected-build" || b.App != "infected.vpk" || b.Minutes != 10 || len(b.Kit) != 3 || len(b.Args) != 4 {
		t.Fatalf("build not read: %+v", b)
	}
	if m.VPK != "" {
		t.Fatal("a build release needs no vpk")
	}
	// The kit for a computer, with a Mac falling back to the other Mac kit.
	if k, f, ok := b.KitFor("windows", "amd64"); !ok || k != "windows-amd64" || f.File != "Infected-Vita-builder-windows.zip" {
		t.Fatalf("windows kit: %v %v %v", k, f, ok)
	}
	if _, f, ok := b.KitFor("darwin", "arm64"); !ok || f.File != "Infected-Vita-builder-mac.zip" {
		t.Fatalf("an Apple Silicon Mac should use the Mac kit: %v %v", f, ok)
	}
	if _, _, ok := b.KitFor("windows", "386"); ok {
		t.Fatal("no kit for 32-bit Windows was listed")
	}
	// A recipe fingerprint that does not depend on the order of the kit list.
	m2, _ := parse(t, buildManifest())
	if b.Fingerprint() != m2.Build.Fingerprint() || len(b.Fingerprint()) != 16 {
		t.Fatal("fingerprint is not stable")
	}
	other := buildManifest()
	other["build"].(map[string]interface{})["run"] = "other"
	m3, _ := parse(t, other)
	if m3.Build.Fingerprint() == b.Fingerprint() {
		t.Fatal("a different recipe has the same fingerprint")
	}
}

func TestBuildManifestIsChecked(t *testing.T) {
	set := func(f func(b, root map[string]interface{})) map[string]interface{} {
		m := buildManifest()
		f(m["build"].(map[string]interface{}), m)
		return m
	}
	kit := func(b map[string]interface{}) map[string]interface{} {
		return b["kit"].(map[string]interface{})["windows-amd64"].(map[string]interface{})
	}
	cases := []struct {
		name string
		m    map[string]interface{}
		want string
	}{
		{"unknown setting", set(func(b, _ map[string]interface{}) { b["sandbox"] = true }), "newer installer"},
		{"unknown kit setting", set(func(b, _ map[string]interface{}) { kit(b)["signature"] = "x" }), "newer installer"},
		{"no from", set(func(b, _ map[string]interface{}) { delete(b, "from") }), "from"},
		{"from is not a need", set(func(b, _ map[string]interface{}) { b["from"] = "cartridge" }), "not one of the release's needs"},
		{"from is optional", set(func(b, _ map[string]interface{}) { b["from"] = "shader" }), "optional"},
		{"no kit", set(func(b, _ map[string]interface{}) { b["kit"] = map[string]interface{}{} }), "kit"},
		{"bad kit name", set(func(b, _ map[string]interface{}) {
			b["kit"] = map[string]interface{}{"beos": map[string]interface{}{"file": "a.zip", "sha256": sum1}}
		}), "<os>-<arch>"},
		{"short sha", set(func(b, _ map[string]interface{}) { kit(b)["sha256"] = "abc" }), "SHA-256"},
		{"non-hex sha", set(func(b, _ map[string]interface{}) { kit(b)["sha256"] = strings.Repeat("z", 64) }), "SHA-256"},
		{"kit file in a folder", set(func(b, _ map[string]interface{}) { kit(b)["file"] = "../kit.zip" }), "plain file name"},
		{"kit file with backslash", set(func(b, _ map[string]interface{}) { kit(b)["file"] = `x\kit.zip` }), "plain file name"},
		{"run escapes the kit", set(func(b, _ map[string]interface{}) { b["run"] = "../infected-build" }), "relative path"},
		{"run is absolute", set(func(b, _ map[string]interface{}) { b["run"] = "/bin/sh" }), "relative path"},
		{"no run", set(func(b, _ map[string]interface{}) { delete(b, "run") }), "relative path"},
		{"args without out", set(func(b, _ map[string]interface{}) { b["args"] = []string{"{in}"} }), "{in} and {out}"},
		{"app in a folder", set(func(b, _ map[string]interface{}) { b["app"] = "out/infected.vpk" }), "plain file name"},
		{"no app", set(func(b, _ map[string]interface{}) { delete(b, "app") }), "plain file name"},
		{"silly minutes", set(func(b, _ map[string]interface{}) { b["minutes"] = -3 }), "minutes"},
		{"no title id", set(func(_, root map[string]interface{}) { delete(root, "titleId") }), "titleId"},
		{"bad title id", set(func(_, root map[string]interface{}) { root["titleId"] = "infected" }), "titleId"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := parse(t, c.m)
			if err == nil {
				t.Fatal("accepted")
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Fatalf("error %q does not mention %q", err, c.want)
			}
		})
	}
}

// A release may name a VPK as well as a build; old-format releases are untouched.
func TestBuildAlongsideVPKAndOldFormat(t *testing.T) {
	m := buildManifest()
	m["vpk"] = "Infected.vpk"
	delete(m, "titleId")
	if mf, err := parse(t, m); err != nil || mf.VPK != "Infected.vpk" || mf.Build == nil {
		t.Fatalf("%v %+v", err, mf)
	}
	mf, err := ParseManifest([]byte(`{"name":"X","vpk":"X.vpk","needs":[{"id":"disc","type":"psp-iso","to":"ux0:data/x.iso"}]}`))
	if err != nil || mf.Build != nil {
		t.Fatalf("a release without build changed: %v %+v", err, mf)
	}
	if _, err := ParseManifest([]byte(`{"name":"X","vpk":"X.vpk","build":null}`)); err != nil {
		t.Fatalf("build: null should be the same as none: %v", err)
	}
}

func TestLoadBuildOnlyRelease(t *testing.T) {
	dir := t.TempDir()
	b, _ := json.Marshal(buildManifest())
	os.WriteFile(filepath.Join(dir, ManifestName), b, 0o644)
	if !LooksLikeManifest(filepath.Join(dir, ManifestName)) {
		t.Fatal("a build-only release file is not recognised")
	}
	if name, sum, id := AppOf(b); name != "" || sum != "" || id != "INFC00001" {
		t.Fatalf("AppOf a build-only file: %q %q %q", name, sum, id)
	}
	rs, err := Load(dir, t.TempDir())
	if err != nil || len(rs) != 1 {
		t.Fatalf("%v %d", err, len(rs))
	}
	r := rs[0]
	if r.Build == nil || r.VPK != "" || r.TitleID != "INFC00001" || r.Name != "Infected" || r.App == nil || r.App.Title != "Infected" {
		t.Fatalf("release: %+v", r)
	}
	// The player must allow the build, and the disc must be chosen.
	var consent *Input
	for _, in := range r.Inputs {
		if in.ID == BuildConsentID {
			consent = in
		}
	}
	if consent == nil || consent.Type != NeedConsent || consent.Satisfied() {
		t.Fatalf("no consent to ask for: %+v", consent)
	}
	for _, want := range []string{"on this computer", "from your own disc", "10 minutes", "2 GB", "Nothing is uploaded"} {
		if !strings.Contains(consent.Label+" "+consent.Help, want) {
			t.Errorf("consent text does not say %q: %q / %q", want, consent.Label, consent.Help)
		}
	}
	// Loading again gives the release the same ID.
	if dup, err := Load(dir, t.TempDir()); err != nil || dup[0].ID != r.ID {
		t.Fatalf("a release should keep its ID: %v", err)
	}
}

// {"step": "build"} on the starting need makes installers that predate builds
// refuse the release plainly; this one accepts it and has nothing to run.
func TestBuildMarkerStep(t *testing.T) {
	m := buildManifest()
	m["needs"].([]interface{})[0].(map[string]interface{})["prepare"] = []map[string]string{{"step": "build"}}
	mf, err := parse(t, m)
	if err != nil {
		t.Fatal(err)
	}
	if len(mf.Needs[0].Prepare) != 0 || mf.Needs[0].RunsTool() {
		t.Fatalf("the marker should not be a step to run: %+v", mf.Needs[0].Prepare)
	}
	m["needs"].([]interface{})[0].(map[string]interface{})["prepare"] = []map[string]string{{"step": "teleport"}}
	if _, err := parse(t, m); err == nil || !strings.Contains(err.Error(), "newer installer") {
		t.Fatalf("an unknown step must still be refused: %v", err)
	}
}
