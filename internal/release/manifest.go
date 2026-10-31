package release

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"regexp"
	"runtime"
	"sort"
	"strings"

	"github.com/CBOC-Vita-Lab/vita-installer/internal/vita"
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
	Build     *Build   `json:"build,omitempty"`     // build the app on the player's computer from one of their files
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
	Ext      []string          `json:"ext,omitempty"`      // accepted file extensions for file/files
	Prepare  []Step            `json:"prepare,omitempty"`  // conversion before copying (disc and folder needs)
	Names    []string          `json:"names,omitempty"`    // file: names to look for automatically (default: the name in to)
	MaxBytes int64             `json:"maxBytes,omitempty"` // file: refuse bigger files
	Magic    string            `json:"magic,omitempty"`    // file: the bytes it starts with (then any name is fine)
}

// Step is one preparation step run on the player's files before they are
// copied. Paths in a step are relative to the prepared folder, except
// catalog and program paths, which are relative to the release.
type Step struct {
	Step    string            `json:"step"`              // galaxy, yaz0, thp-le, thp, buckets, verify, tool
	Path    string            `json:"path,omitempty"`    // folder the step works on (default: everything)
	Quality int               `json:"quality,omitempty"` // thp: JPEG quality (default 55)
	Min     int               `json:"min,omitempty"`     // buckets: folders with more files than this are split (default 64)
	Per     int               `json:"per,omitempty"`     // buckets: files per bucket folder (default 32)
	Catalog string            `json:"catalog,omitempty"` // verify: file list in the release
	Skip    []string          `json:"skip,omitempty"`    // verify: path prefixes not checked
	Program map[string]string `json:"program,omitempty"` // tool: "darwin-arm64"/"windows-amd64"/... -> program in the release
	SHA256  map[string]string `json:"sha256,omitempty"`  // tool: the same keys -> the program's SHA-256
	Args    []string          `json:"args,omitempty"`    // tool: arguments; {in} and {out} are replaced
	Label   string            `json:"label,omitempty"`   // shown while the step runs
}

// Steps the installer knows. "build" does nothing by itself: put it on the
// need a Build is made from, so installers that predate builds refuse the
// release with "a newer installer may be required".
var knownSteps = map[string]bool{"galaxy": true, "galaxy-cook": true, "yaz0": true, "thp-le": true, "thp": true, "buckets": true, "verify": true, "tool": true, "build": true}

// Build makes the app on the player's computer from one of their files (a
// game disc), with a program that ships in a "kit" archive of the same
// GitHub release. Nothing made from the game is ever published.
type Build struct {
	From    string             `json:"from"`              // ID of the need whose file is {in}
	Kit     map[string]KitFile `json:"kit"`               // "<os>-<arch>" -> the kit archive for that computer
	Run     string             `json:"run"`               // program inside the kit
	Args    []string           `json:"args,omitempty"`    // {in}, {out} (an empty folder for the VPK) and {work} (a scratch folder) are replaced
	App     string             `json:"app"`               // name of the VPK the program writes into {out}
	Label   string             `json:"label,omitempty"`   // shown while the build starts
	Minutes int                `json:"minutes,omitempty"` // about how long it takes, for the player
}

// KitFile is one kit archive, shipped as a file of the release.
type KitFile struct {
	File   string `json:"file"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size,omitempty"`
}

var kitKeyPattern = regexp.MustCompile(`^(windows|darwin|linux)-[a-z0-9]+$`)

// KitFor picks the kit for a computer. A Mac uses the other Mac kit when its
// own is missing (Intel programs run on Apple Silicon and the other way round).
func (b *Build) KitFor(goos, goarch string) (string, KitFile, bool) {
	keys := []string{goos + "-" + goarch}
	switch goos {
	case "darwin":
		keys = append(keys, "darwin-universal", "darwin-arm64", "darwin-amd64")
	}
	for _, k := range keys {
		if f, ok := b.Kit[k]; ok {
			return k, f, true
		}
	}
	return goos + "-" + goarch, KitFile{}, false
}

// Fingerprint identifies the recipe (not the player's file), the same on every
// computer, so the installer can tell whether the app on the Vita came from
// this release.
func (b *Build) Fingerprint() string {
	var keys []string
	for k := range b.Kit {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var sb strings.Builder
	for _, k := range keys {
		sb.WriteString(k + "=" + strings.ToLower(b.Kit[k].SHA256) + "\n")
	}
	sb.WriteString(b.Run + "\n" + strings.Join(b.Args, "\x00") + "\n" + b.App)
	sum := sha256.Sum256([]byte(sb.String()))
	return hex.EncodeToString(sum[:8])
}

// parseBuild reads the build section strictly: a setting this installer does
// not know means the release needs a newer one.
func parseBuild(raw json.RawMessage) (*Build, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var b Build
	if err := dec.Decode(&b); err != nil {
		if strings.Contains(err.Error(), "unknown field") {
			return nil, fmt.Errorf("the release's build settings use something this installer does not know (%v); a newer installer may be required", err)
		}
		return nil, fmt.Errorf("the release's build settings are not valid: %v", err)
	}
	return &b, nil
}

// buildFromTypes are the needs a build can start from.
var buildFromTypes = map[string]bool{NeedPSPISO: true, NeedWiiDisc: true, NeedGCDisc: true, NeedFile: true, NeedFolder: true}

func (b *Build) check(needs []Need) error {
	var from *Need
	for i := range needs {
		if needs[i].ID == b.From {
			from = &needs[i]
		}
	}
	switch {
	case b.From == "":
		return fmt.Errorf("build needs a \"from\": the need it is built from")
	case from == nil:
		return fmt.Errorf("build.from %q is not one of the release's needs", b.From)
	case from.Optional:
		return fmt.Errorf("build.from %q must be a need the player always supplies, not an optional one", b.From)
	case !buildFromTypes[from.Type]:
		return fmt.Errorf("build.from %q is a %s need, which a build cannot start from", b.From, from.Type)
	}
	if len(b.Kit) == 0 {
		return fmt.Errorf("build needs a kit for at least one kind of computer")
	}
	for k, f := range b.Kit {
		if !kitKeyPattern.MatchString(k) {
			return fmt.Errorf("build kit %q is not an <os>-<arch> name such as windows-amd64", k)
		}
		if !safeFileName(f.File) {
			return fmt.Errorf("build kit %s needs a plain file name", k)
		}
		if !isHex64(f.SHA256) {
			return fmt.Errorf("build kit %s needs a 64-digit SHA-256", k)
		}
		if f.Size < 0 {
			return fmt.Errorf("build kit %s has an invalid size", k)
		}
	}
	if !safeRelPath(b.Run) {
		return fmt.Errorf("build.run needs a safe relative path inside the kit")
	}
	if len(b.Args) > 0 {
		in, out := false, false
		for _, a := range b.Args {
			if strings.ContainsRune(a, 0) {
				return fmt.Errorf("build.args has an invalid argument")
			}
			in = in || strings.Contains(a, "{in}")
			out = out || strings.Contains(a, "{out}")
		}
		if !in || !out {
			return fmt.Errorf("build.args must contain {in} and {out}")
		}
	}
	if !safeFileName(b.App) {
		return fmt.Errorf("build.app needs a plain file name, such as game.vpk")
	}
	if b.Minutes < 0 || b.Minutes > 1440 {
		return fmt.Errorf("build.minutes is not a sensible number")
	}
	return nil
}

// consentHelp is what the player is told before the build may run.
func (b *Build) consentHelp() string {
	var sb strings.Builder
	sb.WriteString("The game is built on this computer from your own disc. ")
	if _, k, ok := b.KitFor(runtime.GOOS, runtime.GOARCH); ok && k.Size > 0 {
		sb.WriteString(fmt.Sprintf("It uses build tools of about %d MB, downloaded once. ", (k.Size+(1<<20)-1)>>20))
	}
	if b.Minutes > 0 {
		sb.WriteString(fmt.Sprintf("It takes about %d minute", b.Minutes))
		if b.Minutes != 1 {
			sb.WriteString("s")
		}
		sb.WriteString(" the first time")
	} else {
		sb.WriteString("It takes a few minutes the first time")
	}
	sb.WriteString(" and needs about 1 GB of free disk space. Nothing is uploaded.")
	return sb.String()
}

func safeFileName(n string) bool {
	return n != "" && n != "." && n != ".." && !strings.ContainsAny(n, "/\\:\x00")
}

// safeRelPath accepts a relative path inside a folder, with "/" separators.
func safeRelPath(p string) bool {
	if p == "" || strings.HasPrefix(p, "/") || strings.ContainsAny(p, "\\:\x00") {
		return false
	}
	for _, part := range strings.Split(p, "/") {
		if part == "" || part == "." || part == ".." {
			return false
		}
	}
	return true
}

func isHex64(s string) bool {
	if len(s) != 64 {
		return false
	}
	for _, c := range s {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F') {
			return false
		}
	}
	return true
}

// RunsTool reports whether the need runs a program shipped in the release.
func (n *Need) RunsTool() bool {
	for _, s := range n.Prepare {
		if s.Step == "tool" {
			return true
		}
	}
	return false
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
	NeedConsent = "consent" // added by the installer when a release runs its own converter
)

// ShaderPath is where vitaGL ports look for the shader compiler.
const ShaderPath = "ur0:data/libshacccg.suprx"

func readManifest(file string) (*Manifest, error) {
	b, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	return ParseManifest(b)
}

// ParseManifest reads either format, told apart by its contents rather than
// its file name.
func ParseManifest(b []byte) (*Manifest, error) {
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(b, &probe); err != nil {
		return nil, fmt.Errorf("the release file is not valid JSON: %v", err)
	}
	if isCBOC(probe) {
		return fromCBOC(b)
	}
	var m Manifest
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("%s is not valid JSON: %v", ManifestName, err)
	}
	if raw := probe["build"]; len(raw) > 0 && string(raw) != "null" {
		bd, err := parseBuild(raw)
		if err != nil {
			return nil, err
		}
		m.Build = bd
	}
	addHDPack(&m)
	return &m, m.check()
}

func isCBOC(probe map[string]json.RawMessage) bool {
	_, a := probe["requiredInputs"]
	_, b := probe["package"]
	_, c := probe["schemaVersion"]
	return a || (b && c)
}

// LooksLikeManifest reports whether a JSON file is a release description of
// either kind (so a renamed one is still found).
func LooksLikeManifest(file string) bool {
	st, err := os.Stat(file)
	if err != nil || st.Size() > 1<<20 {
		return false
	}
	b, err := os.ReadFile(file)
	if err != nil {
		return false
	}
	var probe map[string]json.RawMessage
	if json.Unmarshal(b, &probe) != nil {
		return false
	}
	if isCBOC(probe) {
		_, inputs := probe["requiredInputs"]
		_, dest := probe["destinations"]
		return inputs || dest
	}
	_, needs := probe["needs"]
	_, vpk := probe["vpk"]
	_, cp := probe["copy"]
	_, build := probe["build"]
	return needs || vpk || cp || build
}

// AppOf returns what a release file says about its app: file name, SHA-256
// and title ID (any may be empty).
func AppOf(b []byte) (name, sha256, titleID string) {
	m, err := ParseManifest(b)
	if err != nil {
		return "", "", ""
	}
	if m.VPK == "" {
		return "", strings.ToLower(m.VPKSHA256), m.TitleID
	}
	return path.Base(m.VPK), strings.ToLower(m.VPKSHA256), m.TitleID
}

func (m *Manifest) check() error {
	for i := range m.Needs {
		n := &m.Needs[i]
		for _, st := range n.Prepare {
			if !knownSteps[st.Step] {
				return fmt.Errorf("preparation step %q is unknown; a newer installer may be required", st.Step)
			}
			if strings.Contains(st.Path, "..") || strings.Contains(st.Catalog, "..") {
				return fmt.Errorf("preparation step %q has an unsafe path", st.Step)
			}
			if st.Step == "verify" && st.Catalog == "" {
				return fmt.Errorf("a verify step needs a catalog")
			}
			if st.Step == "tool" {
				if len(st.Program) == 0 {
					return fmt.Errorf("a tool step needs a program")
				}
				for k, prog := range st.Program {
					if strings.Contains(prog, "..") || len(st.SHA256[k]) != 64 {
						return fmt.Errorf("tool program for %s needs a safe path and a SHA-256", k)
					}
				}
			}
		}
		// "build" only marks the need (see knownSteps); it is not a step to run.
		var steps []Step
		for _, st := range n.Prepare {
			if st.Step != "build" {
				steps = append(steps, st)
			}
		}
		n.Prepare = steps
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
	if m.Build != nil {
		if err := m.Build.check(m.Needs); err != nil {
			return err
		}
		if m.VPK == "" && !vita.ValidTitleID(m.TitleID) {
			return fmt.Errorf("a release that builds its app needs its titleId (for example INFC00001)")
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
			n.Type, n.To, n.GameID, n.Revision = NeedWiiDisc, data, "RMGE01", &rev0
			n.Label = "Your Super Mario Galaxy disc (Wii, USA)"
			n.Prepare = []Step{{Step: "galaxy"}}
		case "infected-disc":
			n.Type, n.To, n.GameID = NeedPSPISO, strings.TrimSuffix(data, "/")+"/Infected.iso", "ULUS-10054"
			n.Label = "Your Infected UMD backup (PSP, USA, ISO)"
		case "infected-hd-2x":
			n.Type, n.To, n.Ext = NeedFile, strings.TrimSuffix(data, "/")+"/texpack_2x.bin", []string{".bin"}
			n.Label, n.Optional = "HD textures, 2x pack (optional)", true
			n.Names, n.MaxBytes = []string{"texpack_2x.bin"}, 120<<20
			n.Help = "texpack_2x.bin, made from your own disc with the texture builder"
		case "infected-hd-4x":
			n.Type, n.To, n.Ext = NeedFile, strings.TrimSuffix(data, "/")+"/texpack_4x.bin", []string{".bin"}
			n.Label, n.Optional = "HD textures, Vita 4x pack (optional)", true
			n.Names, n.MaxBytes = []string{"texpack_4x_mixed.bin"}, 160<<20
			n.Help = "texpack_4x_mixed.bin (not the full 4x pack, which is too big for the Vita)"
		case "strikers-hd", "galaxy-hd":
			n = hdPack(kind)
			n.ID = id
		case "shader-compiler":
			n.Type = NeedShader
		case "roms":
			n.Type, n.To, n.Ext = NeedFiles, data, []string{".gb", ".gbc", ".gba"}
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
	// Infected releases made before the HD texture slots existed get them too.
	kinds := map[string]bool{}
	for _, in := range append(c.RequiredInputs, c.OptionalInputs...) {
		kinds[in.Kind] = true
	}
	if kinds["infected-disc"] {
		for _, k := range []string{"infected-hd-2x", "infected-hd-4x"} {
			if !kinds[k] {
				add(strings.TrimPrefix(k, "infected-"), "", k, true)
			}
		}
	}
	addHDPack(m)
	if c.ReleaseState != "" && c.ReleaseState != "ready" && m.Held == "" {
		m.Held = "This release is not ready to install yet."
	}
	return m, m.check()
}

// hdPackMagic starts every HD texture pack the Wii ports read (format 1).
const hdPackMagic = "HDTX\x01\x00\x00\x00"

// hdPack is the optional HD texture pack slot for a Wii port.
func hdPack(kind string) Need {
	n := Need{ID: "hd", Type: NeedFile, Optional: true, Magic: hdPackMagic, Names: []string{"hdtex.pack"}, MaxBytes: 400 << 20}
	n.Label = "HD textures (optional)"
	n.Help = "hdtex.pack, made from your own game. A ZIP, folder or split pieces of it work too"
	switch kind {
	case "strikers-hd":
		n.To = "ux0:data/mscharged/hdtex.pack"
	case "galaxy-hd":
		n.To = "ux0:data/smg/hdtex.pack"
	}
	return n
}

// addHDPack gives Strikers and Galaxy releases the HD texture slot when the
// release does not list one itself.
func addHDPack(m *Manifest) {
	kind := ""
	for _, n := range m.Needs {
		if n.Type == NeedFile && n.Magic == hdPackMagic {
			return
		}
		switch strings.ToUpper(n.GameID) {
		case "R4QE01":
			kind = "strikers-hd"
		case "RMGE01":
			kind = "galaxy-hd"
		}
	}
	if kind != "" {
		m.Needs = append(m.Needs, hdPack(kind))
	}
}
