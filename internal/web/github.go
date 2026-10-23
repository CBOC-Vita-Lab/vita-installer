package web

import (
	"context"
	"crypto/sha1"
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
		Tag    string `json:"tag_name"`
		HTML   string `json:"html_url"`
		Assets []struct {
			Name string `json:"name"`
			URL  string `json:"browser_download_url"`
		} `json:"assets"`
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
	asset, assetURL, sumsURL := "", "", ""
	for _, as := range rel.Assets {
		switch {
		case strings.HasSuffix(as.Name, selfupdate.AssetSuffix()) && selfupdate.AssetSuffix() != "":
			asset, assetURL = as.Name, as.URL
		case as.Name == "SHA256SUMS.txt":
			sumsURL = as.URL
		}
	}
	ok, why := selfupdate.Possible()
	switch {
	case !ok:
		u.Manual = why
	case selfupdate.Failed(a.cache, version):
		u.Manual = "it could not replace itself on this computer"
	case assetURL == "" || sumsURL == "":
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
	st, err := selfupdate.Stage(ctx, a.cache, version, asset, assetURL, sumsURL, func(done, total int64) {
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
		var rel struct {
			Tag    string `json:"tag_name"`
			Assets []struct {
				Name string `json:"name"`
				URL  string `json:"browser_download_url"`
				Size int64  `json:"size"`
			} `json:"assets"`
		}
		a.update(func(s *State) { s.Task = &Task{Text: "Looking up " + repo} })
		err := getJSON(ctx, api, &rel)
		a.update(func(s *State) { s.Task = nil })
		if err != nil {
			return fmt.Errorf("could not find a release of %s/%s on GitHub", owner, repo)
		}
		// A complete release ZIP (the app plus what it needs) beats a bare VPK;
		// source archives are never installed.
		type cand struct {
			url   string
			score int
		}
		var cands []cand
		for _, as := range rel.Assets {
			n := strings.ToLower(as.Name)
			sc := 0
			switch {
			case strings.HasSuffix(n, ".zip") && (strings.Contains(n, "source") || strings.Contains(n, "src")):
			case strings.HasSuffix(n, "-install.zip"):
				sc = 5
			case strings.HasSuffix(n, ".zip"):
				sc = 4
			case strings.HasSuffix(n, ".vpk"):
				sc = 3
			}
			if sc > 0 {
				cands = append(cands, cand{as.URL, sc})
			}
		}
		if len(cands) == 0 {
			return fmt.Errorf("the latest release of %s has no Vita app to download", repo)
		}
		sort.SliceStable(cands, func(i, j int) bool { return cands[i].score > cands[j].score })
		for _, c := range cands {
			// Try the next file only when this one holds no app, not when
			// the download itself failed.
			if err = a.fetch(ctx, c.url, owner+"/"+repo, rel.Tag); err == nil || errors.As(err, new(*downloadError)) {
				return err
			}
		}
		return err
	}
	return a.fetch(ctx, raw, repoName, tagName)
}

// fetch downloads one .vpk or .zip and adds it.
func (a *App) fetch(ctx context.Context, raw, repoName, tagName string) error {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") {
		return errors.New("paste a GitHub link or a direct link to a .vpk or .zip file")
	}
	name := path.Base(u.Path)
	if ext := strings.ToLower(path.Ext(name)); ext != ".vpk" && ext != ".zip" {
		return errors.New("that link is not a .vpk or .zip file")
	}
	// Releases often keep the same file names from version to version, so
	// each download gets its own folder.
	h := sha1.Sum([]byte(raw))
	dir := filepath.Join(a.cache, "downloads", hex.EncodeToString(h[:6]))
	os.MkdirAll(dir, 0o755)
	dest := filepath.Join(dir, name)
	if err := a.download(ctx, raw, dest, name); err != nil {
		return &downloadError{err}
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

func (a *App) download(ctx context.Context, u, dest, name string) error {
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
