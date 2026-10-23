package release

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"strings"
)

// ManifestName is the optional file a release can ship next to its VPK to
// describe extra install steps. See docs/release-format.md.
const ManifestName = "vita-install.json"

// Manifest is the vita-install.json format.
type Manifest struct {
	Name      string   `json:"name,omitempty"`
	TitleID   string   `json:"titleId,omitempty"`
	VPK       string   `json:"vpk,omitempty"`       // path of the VPK, relative to the manifest
	VPKSHA256 string   `json:"vpkSha256,omitempty"` // optional download check
	Copy      []Copy   `json:"copy,omitempty"`      // bundled files to copy (default: ux0/ and data/ folders)
	Needs     []Need   `json:"needs,omitempty"`     // files the player supplies
	Keep      []string `json:"keep,omitempty"`      // Vita paths never written (saves, settings)
	Notes     string   `json:"notes,omitempty"`     // shown before installing
	Held      string   `json:"held,omitempty"`      // if set, the release cannot be installed yet; the text says why
}

// Copy maps a bundled file or folder to a Vita path.
type Copy struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// Need is something the player provides.
type Need struct {
	ID       string            `json:"id"`
	Type     string            `json:"type"`               // wii-disc, gc-disc, psp-iso, file, folder, files, shader-compiler
	Label    string            `json:"label,omitempty"`    // what to ask for
	Help     string            `json:"help,omitempty"`     // one extra line of help
	To       string            `json:"to,omitempty"`       // Vita destination (folder, or file for psp-iso/file)
	GameID   string            `json:"gameId,omitempty"`   // disc game ID such as R4QE01 or ULUS-10054
	Revision *int              `json:"revision,omitempty"` // disc revision, if one is required
	Verify   map[string]string `json:"verify,omitempty"`   // relative file -> SHA-256 inside an extracted disc
	Optional bool              `json:"optional,omitempty"`
	Ext      []string          `json:"ext,omitempty"` // accepted file extensions for file/files
}

// Need types.
const (
	NeedWiiDisc = "wii-disc"
	NeedGCDisc  = "gc-disc"
	NeedPSPISO  = "psp-iso"
	NeedFile    = "file"
	NeedFolder  = "folder"
	NeedFiles   = "files"
	NeedShader  = "shader-compiler"
)

// ShaderPath is where vitaGL ports look for the shader compiler.
const ShaderPath = "ur0:data/libshacccg.suprx"

func readManifest(file string) (*Manifest, error) {
	b, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	if strings.EqualFold(path.Base(strings.ReplaceAll(file, "\\", "/")), "cboc-release.json") {
		return fromCBOC(b)
	}
	var m Manifest
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("%s is not valid JSON: %v", ManifestName, err)
	}
	return &m, m.check()
}

func (m *Manifest) check() error {
	for i := range m.Needs {
		n := &m.Needs[i]
		if n.ID == "" {
			n.ID = fmt.Sprintf("need%d", i+1)
		}
		switch n.Type {
		case NeedWiiDisc, NeedGCDisc, NeedFolder, NeedFiles:
			if !ValidVitaPath(n.To) {
				return fmt.Errorf("need %q has an invalid destination %q", n.ID, n.To)
			}
		case NeedPSPISO, NeedFile:
			if !ValidVitaPath(n.To) || strings.HasSuffix(n.To, "/") {
				return fmt.Errorf("need %q must name a destination file", n.ID)
			}
		case NeedShader:
			n.To = ShaderPath
			if n.Label == "" {
				n.Label = "Shader compiler (libshacccg.suprx)"
			}
		default:
			return fmt.Errorf("need %q has unknown type %q; a newer installer may be required", n.ID, n.Type)
		}
		if n.Label == "" {
			n.Label = n.ID
		}
	}
	for _, c := range m.Copy {
		if !ValidVitaPath(c.To) || c.From == "" || strings.Contains(c.From, "..") {
			return fmt.Errorf("copy rule %q -> %q is invalid", c.From, c.To)
		}
	}
	for _, k := range m.Keep {
		if !ValidVitaPath(k) {
			return fmt.Errorf("keep path %q is invalid", k)
		}
	}
	return nil
}

// ValidVitaPath accepts ux0:/ur0:/uma0:/imc0:/xmc0: paths without "..".
func ValidVitaPath(p string) bool {
	i := strings.IndexByte(p, ':')
	if i < 0 {
		return false
	}
	switch p[:i] {
	case "ux0", "ur0", "uma0", "imc0", "xmc0", "grw0":
	default:
		return false
	}
	rest := p[i+1:]
	if strings.Contains(rest, "\\") || strings.ContainsRune(rest, 0) {
		return false
	}
	for _, part := range strings.Split(rest, "/") {
		if part == ".." {
			return false
		}
	}
	return true
}

// fromCBOC converts the schema-1 cboc-release.json used by earlier CBOC releases.
func fromCBOC(b []byte) (*Manifest, error) {
	var c struct {
		TitleID        string `json:"titleId"`
		DisplayName    string `json:"displayName"`
		ReleaseState   string `json:"releaseState"`
		Package        struct{ Path, Sha256 string }
		RequiredInputs []struct{ ID, Label, Kind string } `json:"requiredInputs"`
		OptionalInputs []struct{ ID, Label, Kind string } `json:"optionalInputs"`
		Destinations   map[string]string                  `json:"destinations"`
		ProtectedPaths []string                           `json:"protectedPaths"`
	}
	if err := json.Unmarshal(b, &c); err != nil {
		return nil, fmt.Errorf("cboc-release.json is not valid JSON: %v", err)
	}
	m := &Manifest{Name: c.DisplayName, TitleID: c.TitleID, VPK: c.Package.Path, VPKSHA256: c.Package.Sha256, Keep: c.ProtectedPaths}
	data := c.Destinations["data"]
	rev1, rev0 := 1, 0
	add := func(id, label, kind string, optional bool) error {
		n := Need{ID: id, Label: label, Optional: optional}
		switch kind {
		case "strikers-disc":
			n.Type, n.To, n.GameID, n.Revision = NeedWiiDisc, data, "R4QE01", &rev1
			n.Label = "Your Mario Strikers Charged disc (Wii, USA, Rev 1)"
			n.Verify = map[string]string{"sys/main.dol": "8f811628ee6646c60620a9c599ef08ed929d2eccc98a779c211847f1500f81fd"}
		case "galaxy-disc":
			m.Held = "Super Mario Galaxy needs a newer installer for its game-data conversion."
			n.Type, n.To, n.GameID, n.Revision = NeedWiiDisc, data, "RMGE01", &rev0
		case "infected-disc":
			n.Type, n.To, n.GameID = NeedPSPISO, strings.TrimSuffix(data, "/")+"/Infected.iso", "ULUS-10054"
			n.Label = "Your Infected UMD backup (PSP, USA, ISO)"
		case "shader-compiler":
			n.Type = NeedShader
		case "roms":
			n.Type, n.To, n.Ext = NeedFiles, data, []string{".gb", ".gbc", ".gba", ".zip"}
			n.Label = "Your GB, GBC or GBA games (optional)"
		case "gba-bios":
			n.Type, n.To = NeedFile, "ux0:data/GBAdhoc/gba_bios.bin"
			n.Label = "Your GBA BIOS (optional)"
		default:
			return nil // optional extras this installer does not handle are skipped
		}
		m.Needs = append(m.Needs, n)
		return nil
	}
	for _, in := range c.RequiredInputs {
		if err := add(in.ID, in.Label, in.Kind, false); err != nil {
			return nil, err
		}
	}
	for _, in := range c.OptionalInputs {
		if err := add(in.ID, in.Label, in.Kind, true); err != nil {
			return nil, err
		}
	}
	if c.ReleaseState != "" && c.ReleaseState != "ready" && m.Held == "" {
		m.Held = "This release is not ready to install yet."
	}
	return m, m.check()
}
