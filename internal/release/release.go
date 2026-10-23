// Package release turns whatever the player chose (a VPK, a release ZIP, a
// folder) into a list of installable apps and the files that go with them.
package release

import (
	"archive/zip"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/CBOC-Vita-Lab/vita-installer/internal/vita"
)

// Release is one app to install.
type Release struct {
	ID      string    `json:"id"`
	Name    string    `json:"name"`
	TitleID string    `json:"titleId"`
	Version string    `json:"version"`
	Source  string    `json:"source"`         // file or folder name the player chose
	Repo    string    `json:"repo,omitempty"` // GitHub owner/repo it was downloaded from
	Tag     string    `json:"tag,omitempty"`  // and that release's tag
	App     *vita.App `json:"app"`
	VPK     string    `json:"-"`
	Root    string    `json:"-"` // folder the release's relative paths start from
	Copies  []Bundled `json:"-"`
	Bundled int64     `json:"bundledBytes"` // size of bundled data files
	Inputs  []*Input  `json:"inputs"`
	Keep    []string  `json:"keep,omitempty"`
	Notes   string    `json:"notes,omitempty"`
	Held    string    `json:"held,omitempty"`
	Warning string    `json:"warning,omitempty"`
}

// Bundled is a file shipped in the release that is copied to the Vita.
type Bundled struct {
	Local  string
	Remote string
	Size   int64
}

// Load reads path. work is a scratch folder for unpacking ZIPs.
func Load(path, work string) ([]*Release, error) {
	st, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("cannot open %s", filepath.Base(path))
	}
	if st.IsDir() {
		return loadFolder(path, filepath.Base(path))
	}
	switch strings.ToLower(filepath.Ext(path)) {
	case ".vpk":
		return loadVPK(path)
	case ".zip":
		if isVPKZip(path) {
			return loadVPK(path)
		}
		dir, err := unpack(path, work)
		if err != nil {
			return nil, err
		}
		rs, err := loadFolder(dir, filepath.Base(path))
		if err != nil {
			return nil, err
		}
		return rs, nil
	case ".json":
		return loadFolder(filepath.Dir(path), filepath.Base(filepath.Dir(path)))
	}
	return nil, fmt.Errorf("%s is not a Vita app. Choose a .vpk file, a release .zip or its folder", filepath.Base(path))
}

func isVPKZip(path string) bool {
	z, err := zip.OpenReader(path)
	if err != nil {
		return false
	}
	defer z.Close()
	for _, f := range z.File {
		if strings.EqualFold(f.Name, "sce_sys/param.sfo") {
			return true
		}
	}
	return false
}

func idFor(s string) string {
	h := sha1.Sum([]byte(s))
	return hex.EncodeToString(h[:6])
}

// unpack extracts a release ZIP once into work/<hash>; repeated loads reuse it.
func unpack(path, work string) (string, error) {
	st, _ := os.Stat(path)
	dir := filepath.Join(work, "releases", idFor(fmt.Sprintf("%s|%d|%d", path, st.Size(), st.ModTime().UnixNano())))
	if _, err := os.Stat(filepath.Join(dir, ".complete")); err == nil {
		return dir, nil
	}
	os.RemoveAll(dir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	z, err := zip.OpenReader(path)
	if err != nil {
		return "", fmt.Errorf("%s is not a valid ZIP file", filepath.Base(path))
	}
	hasVPK := false
	for _, f := range z.File {
		if strings.EqualFold(filepath.Ext(f.Name), ".vpk") {
			hasVPK = true
		}
	}
	z.Close()
	if !hasVPK {
		os.RemoveAll(dir)
		return "", fmt.Errorf("%s has no Vita app (.vpk) inside", filepath.Base(path))
	}
	if err := vita.ExtractZip(path, dir, nil); err != nil {
		os.RemoveAll(dir)
		return "", err
	}
	os.WriteFile(filepath.Join(dir, ".complete"), nil, 0o644)
	return dir, nil
}

func loadVPK(path string) ([]*Release, error) {
	// A VPK sitting next to its own manifest is loaded with it.
	for _, m := range []string{ManifestName, "cboc-release.json"} {
		if _, err := os.Stat(filepath.Join(filepath.Dir(path), m)); err == nil {
			if rs, err := loadFolder(filepath.Dir(path), filepath.Base(path)); err == nil {
				for _, r := range rs {
					if sameFile(r.VPK, path) {
						return []*Release{r}, nil
					}
				}
			}
		}
	}
	app, err := vita.OpenVPK(path)
	if err != nil {
		return nil, err
	}
	r := &Release{ID: idFor(path), Name: app.Title, TitleID: app.TitleID, Version: app.Version, Source: filepath.Base(path), App: app, VPK: path, Root: filepath.Dir(path)}
	r.addShaderNeed()
	return []*Release{r}, nil
}

func sameFile(a, b string) bool {
	sa, err1 := os.Stat(a)
	sb, err2 := os.Stat(b)
	return err1 == nil && err2 == nil && os.SameFile(sa, sb)
}

var skipDirs = map[string]bool{".git": true, "node_modules": true, "__MACOSX": true, "src": true, "source": true, "native": true, "renderer": true, "licenses": true, "tests": true}

// loadFolder finds every VPK (and manifest) up to a few levels deep.
func loadFolder(root, source string) ([]*Release, error) {
	var vpks, manifests []string
	count := 0
	var walk func(dir string, depth int)
	walk = func(dir string, depth int) {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return
		}
		for _, e := range entries {
			count++
			if count > 50000 {
				return
			}
			p := filepath.Join(dir, e.Name())
			if e.IsDir() {
				if depth < 4 && !skipDirs[e.Name()] && !strings.HasPrefix(e.Name(), ".") {
					walk(p, depth+1)
				}
				continue
			}
			low := strings.ToLower(e.Name())
			switch {
			case strings.HasSuffix(low, ".vpk") && !strings.HasPrefix(low, "._"):
				vpks = append(vpks, p)
			case low == ManifestName || low == "cboc-release.json":
				manifests = append(manifests, p)
			}
		}
	}
	walk(root, 0)
	sort.Strings(vpks)
	sort.Slice(manifests, func(i, j int) bool { // vita-install.json wins over cboc-release.json in the same folder
		di, dj := filepath.Dir(manifests[i]), filepath.Dir(manifests[j])
		if di != dj {
			return di < dj
		}
		return strings.EqualFold(filepath.Base(manifests[i]), ManifestName)
	})
	var out []*Release
	claimed := map[string]bool{}
	doneDirs := map[string]bool{}
	badDirs := map[string]bool{}
	var firstErr error
	for _, mf := range manifests {
		dir := filepath.Dir(mf)
		if doneDirs[dir] {
			continue
		}
		doneDirs[dir] = true
		m, err := readManifest(mf)
		if err != nil {
			badDirs[dir] = true
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		vpk := ""
		if m.VPK != "" {
			vpk = filepath.Join(dir, filepath.FromSlash(m.VPK))
			if _, err := os.Stat(vpk); err != nil {
				vpk = findByName(vpks, filepath.Base(m.VPK))
			}
		} else {
			for _, v := range vpks {
				if filepath.Dir(v) == dir {
					if vpk != "" {
						vpk = ""
						break
					}
					vpk = v
				}
			}
		}
		if vpk == "" {
			if firstErr == nil {
				firstErr = fmt.Errorf("the release's app (%s) is missing; download the complete release ZIP", m.VPK)
			}
			continue
		}
		r, err := fromManifest(m, dir, vpk, source)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		claimed[vpk] = true
		out = append(out, r)
	}
	for _, v := range vpks {
		if claimed[v] || badDirs[filepath.Dir(v)] {
			continue
		}
		app, err := vita.OpenVPK(v)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		r := &Release{ID: idFor(v), Name: app.Title, TitleID: app.TitleID, Version: app.Version, Source: source, App: app, VPK: v, Root: filepath.Dir(v)}
		r.Copies = defaultCopies(r.Root)
		r.finish()
		r.addShaderNeed()
		out = append(out, r)
	}
	if len(out) == 0 {
		if firstErr != nil {
			return nil, firstErr
		}
		return nil, fmt.Errorf("no Vita app (.vpk) was found in %s", source)
	}
	return dedupe(out), nil
}

// dedupe keeps one release per title ID: the newest by app version.
func dedupe(rs []*Release) []*Release {
	best := map[string]*Release{}
	var order []string
	for _, r := range rs {
		cur, ok := best[r.TitleID]
		if !ok {
			order = append(order, r.TitleID)
			best[r.TitleID] = r
			continue
		}
		if len(r.Inputs) > len(cur.Inputs) || r.Version > cur.Version {
			best[r.TitleID] = r
		}
	}
	out := make([]*Release, 0, len(order))
	for _, id := range order {
		out = append(out, best[id])
	}
	return out
}

func findByName(paths []string, name string) string {
	for _, p := range paths {
		if strings.EqualFold(filepath.Base(p), name) {
			return p
		}
	}
	return ""
}

func fromManifest(m *Manifest, dir, vpk, source string) (*Release, error) {
	if m.VPKSHA256 != "" {
		if sum, err := sha256File(vpk); err != nil {
			return nil, err
		} else if !strings.EqualFold(sum, m.VPKSHA256) {
			return nil, fmt.Errorf("%s is damaged or incomplete (checksum mismatch); download it again", filepath.Base(vpk))
		}
	}
	app, err := vita.OpenVPK(vpk)
	if err != nil {
		return nil, err
	}
	if m.TitleID != "" && m.TitleID != app.TitleID {
		return nil, fmt.Errorf("%s is %s, but its release says %s", filepath.Base(vpk), app.TitleID, m.TitleID)
	}
	name := m.Name
	if name == "" {
		name = app.Title
	}
	r := &Release{ID: idFor(vpk), Name: name, TitleID: app.TitleID, Version: app.Version, Source: source, App: app, VPK: vpk, Root: dir, Keep: m.Keep, Notes: m.Notes, Held: m.Held}
	if m.Copy == nil {
		r.Copies = defaultCopies(dir)
	} else {
		for _, c := range m.Copy {
			r.Copies = append(r.Copies, expand(filepath.Join(dir, filepath.FromSlash(c.From)), c.To)...)
		}
	}
	tool := false
	for _, n := range m.Needs {
		r.Inputs = append(r.Inputs, &Input{Need: n})
		tool = tool || n.RunsTool()
	}
	if tool {
		r.Inputs = append(r.Inputs, &Input{Need: Need{ID: "allow-tool", Type: NeedConsent,
			Label: "Allow this release's converter to run on this computer",
			Help:  "It prepares your game files for the Vita. Only allow releases you trust."}, Auto: true})
	}
	r.finish()
	r.addShaderNeed()
	return r, nil
}

func (r *Release) finish() {
	var kept []Bundled
	for _, c := range r.Copies {
		if !r.IsKept(c.Remote) {
			kept = append(kept, c)
			r.Bundled += c.Size
		}
	}
	r.Copies = kept
}

// addShaderNeed asks for the shader compiler when the app appears to use it,
// even if the release does not say so. It is skipped automatically when the
// Vita already has it.
func (r *Release) addShaderNeed() {
	for _, in := range r.Inputs {
		if in.Type == NeedShader {
			return
		}
	}
	if r.App != nil && r.App.UsesShaders {
		r.Inputs = append(r.Inputs, &Input{Need: Need{ID: "shader", Type: NeedShader, Label: "Shader compiler (libshacccg.suprx)", To: ShaderPath, Optional: true}, Auto: true})
	}
}

// IsKept reports whether a Vita path is protected by the release.
func (r *Release) IsKept(remote string) bool {
	for _, k := range r.Keep {
		k = strings.TrimSuffix(k, "/")
		if remote == k || strings.HasPrefix(remote, k+"/") {
			return true
		}
	}
	return false
}

// defaultCopies maps ux0/, ur0/data/ and data/ folders next to a VPK.
func defaultCopies(dir string) []Bundled {
	var out []Bundled
	out = append(out, expand(filepath.Join(dir, "ux0"), "ux0:")...)
	out = append(out, expand(filepath.Join(dir, "ur0", "data"), "ur0:data")...)
	out = append(out, expand(filepath.Join(dir, "data"), "ux0:data")...)
	var safe []Bundled
	for _, b := range out {
		low := strings.ToLower(b.Remote)
		// Plugin configuration is never touched automatically.
		if strings.HasPrefix(low, "ux0:tai/") || strings.HasPrefix(low, "ur0:tai/") || strings.HasPrefix(low, "ux0:app/") {
			continue
		}
		safe = append(safe, b)
	}
	return safe
}

// expand lists every file under local (a file or folder) mapped below remote.
func expand(local, remote string) []Bundled {
	st, err := os.Stat(local)
	if err != nil {
		return nil
	}
	remote = strings.TrimSuffix(remote, "/")
	if !st.IsDir() {
		return []Bundled{{Local: local, Remote: remote, Size: st.Size()}}
	}
	var out []Bundled
	filepath.Walk(local, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		name := info.Name()
		if name == ".DS_Store" || strings.HasPrefix(name, "._") || name == "Thumbs.db" || name == "desktop.ini" {
			return nil
		}
		rel, _ := filepath.Rel(local, p)
		sep := "/"
		if strings.HasSuffix(remote, ":") {
			sep = ""
		}
		out = append(out, Bundled{Local: p, Remote: remote + sep + filepath.ToSlash(rel), Size: info.Size()})
		return nil
	})
	return out
}

func sha256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
