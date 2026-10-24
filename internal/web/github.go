package web

import (
	"context"
	"crypto/sha1"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/CBOC-Vita-Lab/vita-installer/internal/install"
	"github.com/CBOC-Vita-Lab/vita-installer/internal/release"
	"github.com/CBOC-Vita-Lab/vita-installer/internal/selfupdate"
)

// Repo is the installer's own GitHub repository.
const Repo = "CBOC-Vita-Lab/vita-installer"

// CatalogEntry is a release shown on the start screen for one-click install.
type CatalogEntry struct {
	Name  string `json:"name"`
	Repo  string `json:"repo"`
	About string `json:"about"`
	Needs string `json:"needs,omitempty"`
}

//go:embed catalog.json
var catalogJSON []byte

func builtinCatalog() []CatalogEntry {
	var c []CatalogEntry
	json.Unmarshal(catalogJSON, &c)
	return c
}

// Update describes a newer installer.
type Update struct {
	Version     string `json:"version"`
	URL         string `json:"url"`
	Ready       bool   `json:"ready"`            // downloaded and checked: one click to switch
	Downloading bool   `json:"downloading"`      // being downloaded in the background
	Percent     int    `json:"percent"`          // of the download
	Manual      string `json:"manual,omitempty"` // why it can't update itself (download it instead)
}

var httpClient = &http.Client{Timeout: 30 * time.Second}

func getJSON(ctx context.Context, u string, v interface{}) error {
	req, _ := http.NewRequestWithContext(ctx, "GET", u, nil)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "cboc-vita-installer")
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == 404 {
		return errors.New("not found")
	}
	if resp.StatusCode != 200 {
		return fmt.Errorf("GitHub answered %d", resp.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(v)
}

// CheckOnline refreshes the catalog and looks for a newer installer.
func (a *App) CheckOnline(ctx context.Context) {
	var cat []CatalogEntry
	if err := getJSON(ctx, "https://raw.githubusercontent.com/"+Repo+"/main/internal/web/catalog.json", &cat); err == nil && len(cat) > 0 {
		a.update(func(s *State) { s.Catalog = cat })
	}
	var rel struct {
		Tag    string  `json:"tag_name"`
		HTML   string  `json:"html_url"`
		Assets []asset `json:"assets"`
	}
	feed := "https://api.github.com/repos/" + Repo + "/releases/latest"
	if f := os.Getenv("CBOC_UPDATE_FEED"); f != "" { // for testing updates
		feed = f
	}
	if err := getJSON(ctx, feed, &rel); err != nil || !newer(rel.Tag, a.st.Version) {
		return
	}
	version := strings.TrimPrefix(rel.Tag, "v")
	u := &Update{Version: version, URL: rel.HTML}
	asset, assetURL, sumsURL, digest, best := "", "", "", "", 0
	for _, as := range rel.Assets {
		low := strings.ToLower(as.Name)
		if sc := selfupdate.AssetScore(as.Name); sc > best {
			asset, assetURL, digest, best = as.Name, as.URL, as.Digest, sc
		}
		if strings.Contains(low, "sha256") || strings.Contains(low, "checksum") {
			sumsURL = as.URL
		}
	}
	ok, why := selfupdate.Possible()
	switch {
	case !ok:
		u.Manual = why
	case selfupdate.Failed(a.cache, version):
		u.Manual = "it could not replace itself on this computer"
	case assetURL == "" || (sumsURL == "" && digest == ""):
		u.Manual = "this release has no download for this computer"
	default:
		u.Downloading = true
	}
	a.update(func(s *State) { s.Update = u })
	if !u.Downloading {
		return
	}
	// Download in the background; it is applied with one click, or by
	// itself the next time the installer opens.
	last := time.Time{}
	st, err := selfupdate.Stage(ctx, a.cache, version, asset, assetURL, sumsURL, digest, func(done, total int64) {
		if total <= 0 || time.Since(last) < 500*time.Millisecond {
			return
		}
		last = time.Now()
		pct := int(done * 100 / total)
		a.update(func(s *State) {
			if s.Update != nil {
				s.Update.Percent = pct
			}
		})
	})
	a.mu.Lock()
	a.staged = st
	a.mu.Unlock()
	a.update(func(s *State) {
		s.Update.Downloading = false
		if err != nil {
			s.Update.Manual = "the automatic update did not work (" + err.Error() + ")"
		} else {
			s.Update.Ready = true
		}
	})
}

// Staged returns a downloaded update ready to apply, if any.
func (a *App) Staged() *selfupdate.Staged {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.staged
}

// Newer reports whether version tag a is newer than b.
func Newer(a, b string) bool { return newer(a, b) }

func newer(tag, current string) bool {
	parse := func(s string) []int {
		s = strings.TrimPrefix(strings.TrimSpace(s), "v")
		s = strings.SplitN(s, "-", 2)[0]
		var out []int
		for _, p := range strings.Split(s, ".") {
			n, err := strconv.Atoi(p)
			if err != nil {
				return nil
			}
			out = append(out, n)
		}
		return out
	}
	a, b := parse(tag), parse(current)
	if a == nil || b == nil {
		return false
	}
	for i := 0; i < len(a) || i < len(b); i++ {
		x, y := 0, 0
		if i < len(a) {
			x = a[i]
		}
		if i < len(b) {
			y = b[i]
		}
		if x != y {
			return x > y
		}
	}
	return false
}

var repoPattern = regexp.MustCompile(`^(?:https?://)?(?:www\.)?github\.com/([A-Za-z0-9_.-]+)/([A-Za-z0-9_.-]+?)(?:\.git)?(?:/releases/tag/([^/?#]+))?(?:[/?#].*)?$`)

// AddURL downloads a release from a GitHub repository or a direct link.
func (a *App) AddURL(ctx context.Context, raw string) error {
	raw = strings.TrimSpace(raw)
	repoName, tagName := "", ""
	if m := downloadPattern.FindStringSubmatch(raw); m != nil {
		repoName, tagName = m[1]+"/"+m[2], m[3]
	}
	if m := repoPattern.FindStringSubmatch(raw); m != nil && !strings.Contains(raw, "/releases/download/") {
		owner, repo, tag := m[1], m[2], m[3]
		api := "https://api.github.com/repos/" + owner + "/" + repo + "/releases/latest"
		if tag != "" {
			api = "https://api.github.com/repos/" + owner + "/" + repo + "/releases/tags/" + url.PathEscape(tag)
		}
		var rel ghRelease
		a.update(func(s *State) { s.Task = &Task{Text: "Looking up " + repo} })
		err := getJSON(ctx, api, &rel)
		a.update(func(s *State) { s.Task = nil })
		if err != nil {
			return fmt.Errorf("could not find a release of %s/%s on GitHub", owner, repo)
		}
		return a.addRelease(ctx, owner+"/"+repo, rel)
	}
	return a.fetch(ctx, asset{URL: raw}, "", repoName, tagName)
}

type asset struct {
	Name        string `json:"name"`
	URL         string `json:"browser_download_url"`
	Size        int64  `json:"size"`
	ContentType string `json:"content_type"`
	Digest      string `json:"digest"` // "sha256:…", when GitHub provides it
}

type ghRelease struct {
	Tag    string  `json:"tag_name"`
	Assets []asset `json:"assets"`
}

// addRelease picks what to download from a release by what the files are,
// not only by their names: complete release ZIPs first, then the app with
// its release file, then the app alone. A file is checked after download,
// and the next one is tried only when it turned out not to hold an app.
func (a *App) addRelease(ctx context.Context, repo string, rel ghRelease) error {
	var archives, sourceish, apps, unknown, manifests []asset
	for _, as := range rel.Assets {
		n, ct := strings.ToLower(as.Name), strings.ToLower(as.ContentType)
		ext := strings.ToLower(path.Ext(n))
		switch {
		case strings.Contains(n, "sha256") || strings.Contains(n, "checksum") || ext == ".txt" || ext == ".md" ||
			ext == ".png" || ext == ".jpg" || ext == ".pdf" || ext == ".exe" || ext == ".dmg" || strings.HasSuffix(n, ".tar.gz"):
		case ext == ".json":
			if as.Size < 1<<20 {
				manifests = append(manifests, as)
			}
		case ext == ".vpk":
			apps = append(apps, as)
		case ext == ".zip" || strings.Contains(ct, "zip"):
			if strings.Contains(n, "source") || strings.Contains(n, "src") {
				sourceish = append(sourceish, as)
			} else {
				archives = append(archives, as)
			}
		default:
			unknown = append(unknown, as) // maybe an app under another name
		}
	}
	// "-install" ZIPs first, then bigger ZIPs (more likely complete).
	sort.SliceStable(archives, func(i, j int) bool {
		ii, jj := strings.Contains(strings.ToLower(archives[i].Name), "install"), strings.Contains(strings.ToLower(archives[j].Name), "install")
		if ii != jj {
			return ii
		}
		return archives[i].Size > archives[j].Size
	})
	var err error = fmt.Errorf("the latest release of %s has no Vita app to download", repo)
	try := func(as asset, extra *asset) bool {
		err = a.fetch(ctx, as, rel.Tag, repo, rel.Tag, extra)
		return err == nil || errors.As(err, new(*downloadError))
	}
	for _, as := range archives {
		if try(as, nil) {
			return err
		}
	}
	// The app plus its release file, matched by checksum first.
	if len(manifests) > 0 && len(apps)+len(unknown) > 0 {
		for _, m := range manifests {
			if app := a.appFor(ctx, m, append(append([]asset{}, apps...), unknown...)); app != nil {
				if try(*app, &m) {
					return err
				}
			}
		}
	}
	for _, as := range append(append(apps, unknown...), sourceish...) {
		if try(as, nil) {
			return err
		}
	}
	return err
}

// appFor reads a release file asset and returns the app asset it describes:
// by checksum (GitHub's digest), then by name, then the only app.
func (a *App) appFor(ctx context.Context, m asset, apps []asset) *asset {
	req, _ := http.NewRequestWithContext(ctx, "GET", m.URL, nil)
	req.Header.Set("User-Agent", "cboc-vita-installer")
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil
	}
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	resp.Body.Close()
	name, sum, _ := release.AppOf(b)
	if name == "" && sum == "" {
		return nil
	}
	for i, as := range apps {
		if sum != "" && strings.EqualFold(strings.TrimPrefix(as.Digest, "sha256:"), sum) {
			return &apps[i]
		}
	}
	// Without matching checksums, go by name, then by being the only one
	// (files whose checksum is known and different are never picked).
	var maybe []int
	for i, as := range apps {
		if sum == "" || as.Digest == "" {
			maybe = append(maybe, i)
		}
	}
	for _, i := range maybe {
		if strings.EqualFold(apps[i].Name, name) {
			return &apps[i]
		}
	}
	if len(maybe) == 1 {
		return &apps[maybe[0]]
	}
	return nil
}

// fetch downloads one file (and the release file that goes with it, if
// any) and adds it. What it is (an app or a release ZIP) is decided by its
// contents once it is here.
func (a *App) fetch(ctx context.Context, as asset, folderKey, repoName, tagName string, extra ...*asset) error {
	u, err := url.Parse(as.URL)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") {
		return errors.New("paste a GitHub link or a direct link to a .vpk or .zip file")
	}
	name := as.Name
	if name == "" {
		name = path.Base(u.Path)
	}
	if name == "" || name == "/" || name == "." {
		name = "download"
	}
	// Releases often keep the same file names from version to version, so
	// each download gets its own folder.
	h := sha1.Sum([]byte(as.URL + "|" + folderKey))
	dir := filepath.Join(a.cache, "downloads", hex.EncodeToString(h[:6]))
	os.MkdirAll(dir, 0o755)
	dest := filepath.Join(dir, filepath.Base(name))
	if err := a.download(ctx, as.URL, dest, name, as.Digest); err != nil {
		return &downloadError{err}
	}
	if len(extra) > 0 && extra[0] != nil {
		m := extra[0]
		mdest := filepath.Join(dir, "release-"+filepath.Base(m.Name))
		if err := a.download(ctx, m.URL, mdest, m.Name, m.Digest); err != nil {
			return &downloadError{err}
		}
		return a.addFrom(mdest, repoName, tagName)
	}
	return a.addFrom(dest, repoName, tagName)
}

type downloadError struct{ err error }

func (e *downloadError) Error() string { return e.err.Error() }

var downloadPattern = regexp.MustCompile(`^https?://(?:www\.)?github\.com/([A-Za-z0-9_.-]+)/([A-Za-z0-9_.-]+)/releases/download/([^/?#]+)/`)

// LibraryGame is a game this installer put on the connected Vita.
type LibraryGame struct {
	Name    string `json:"name"`
	TitleID string `json:"titleId"`
	Release string `json:"release,omitempty"` // installed tag or version
	Repo    string `json:"repo,omitempty"`
	Latest  string `json:"latest,omitempty"` // newest release on GitHub
	Update  bool   `json:"update"`
}

type latestTag struct {
	tag string
	at  time.Time
}

// latest returns a repository's newest release tag (cached for a while).
func (a *App) latest(ctx context.Context, repo string) string {
	a.mu.Lock()
	if a.tags == nil {
		a.tags = map[string]latestTag{}
	}
	c, ok := a.tags[repo]
	a.mu.Unlock()
	if ok && time.Since(c.at) < 15*time.Minute {
		return c.tag
	}
	var rel struct {
		Tag string `json:"tag_name"`
	}
	getJSON(ctx, "https://api.github.com/repos/"+repo+"/releases/latest", &rel) // private or missing: no answer
	a.mu.Lock()
	a.tags[repo] = latestTag{rel.Tag, time.Now()}
	a.mu.Unlock()
	return rel.Tag
}

// refreshLibrary reads what this installer has put on the connected Vita
// and looks up newer releases of those games.
func (a *App) refreshLibrary() {
	a.mu.Lock()
	v, drive := a.st.Vita, a.st.Drive
	a.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var games []LibraryGame
	if v != nil || drive != "" {
		rs, _ := install.Installed(ctx, v, drive)
		seen := map[string]bool{}
		for _, r := range rs {
			if seen[r.TitleID] {
				continue
			}
			seen[r.TitleID] = true
			g := LibraryGame{Name: r.Name, TitleID: r.TitleID, Release: r.Release, Repo: r.Repo}
			if r.Repo != "" {
				g.Latest = a.latest(ctx, r.Repo)
				g.Update = g.Latest != "" && g.Latest != r.Release
			}
			games = append(games, g)
		}
	}
	a.update(func(s *State) { s.Library = games })
}

func (a *App) download(ctx context.Context, u, dest, name, digest string) error {
	if err := a.downloadOnce(ctx, u, dest, name); err != nil {
		return err
	}
	if want := strings.TrimPrefix(strings.ToLower(digest), "sha256:"); len(want) == 64 {
		f, err := os.Open(dest)
		if err != nil {
			return err
		}
		h := sha256.New()
		io.Copy(h, f)
		f.Close()
		if hex.EncodeToString(h.Sum(nil)) != want {
			os.Remove(dest)
			return fmt.Errorf("%s was damaged on the way (checksum mismatch). Try again", name)
		}
	}
	return nil
}

func (a *App) downloadOnce(ctx context.Context, u, dest, name string) error {
	req, _ := http.NewRequestWithContext(ctx, "GET", u, nil)
	req.Header.Set("User-Agent", "cboc-vita-installer")
	client := &http.Client{Timeout: 0}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("download failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("download failed (HTTP %d)", resp.StatusCode)
	}
	if st, err := os.Stat(dest); err == nil && resp.ContentLength > 0 && st.Size() == resp.ContentLength {
		return nil // already downloaded
	}
	tmp := dest + ".part"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	total := resp.ContentLength
	if total < 0 {
		total = 0
	}
	a.update(func(s *State) { s.Task = &Task{Text: "Downloading " + name, Total: total} })
	defer a.update(func(s *State) { s.Task = nil })
	buf := make([]byte, 1<<20)
	var done int64
	last := time.Now()
	for {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			if _, err := f.Write(buf[:n]); err != nil {
				f.Close()
				os.Remove(tmp)
				return err
			}
			done += int64(n)
			if time.Since(last) > 200*time.Millisecond {
				last = time.Now()
				d := done
				a.update(func(s *State) {
					if s.Task != nil {
						s.Task.Done = d
					}
				})
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			f.Close()
			os.Remove(tmp)
			return fmt.Errorf("download failed: %v", rerr)
		}
	}
	f.Close()
	os.Remove(dest)
	return os.Rename(tmp, dest)
}
