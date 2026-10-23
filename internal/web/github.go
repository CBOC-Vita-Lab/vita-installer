package web

import (
	"context"
	_ "embed"
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
	"strconv"
	"strings"
	"time"
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
	Version string `json:"version"`
	URL     string `json:"url"`
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
		Tag  string `json:"tag_name"`
		HTML string `json:"html_url"`
	}
	if err := getJSON(ctx, "https://api.github.com/repos/"+Repo+"/releases/latest", &rel); err == nil {
		if newer(rel.Tag, a.st.Version) {
			a.update(func(s *State) { s.Update = &Update{Version: strings.TrimPrefix(rel.Tag, "v"), URL: rel.HTML} })
		}
	}
}

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
	if m := repoPattern.FindStringSubmatch(raw); m != nil && !strings.Contains(raw, "/releases/download/") {
		owner, repo, tag := m[1], m[2], m[3]
		api := "https://api.github.com/repos/" + owner + "/" + repo + "/releases/latest"
		if tag != "" {
			api = "https://api.github.com/repos/" + owner + "/" + repo + "/releases/tags/" + url.PathEscape(tag)
		}
		var rel struct {
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
		best, score := "", 0
		for _, as := range rel.Assets {
			n := strings.ToLower(as.Name)
			sc := 0
			switch {
			case strings.HasSuffix(n, "-install.zip"):
				sc = 4
			case strings.HasSuffix(n, ".vpk"):
				sc = 3
			case strings.HasSuffix(n, ".zip") && !strings.Contains(n, "source") && !strings.Contains(n, "src"):
				sc = 2
			}
			if sc > score {
				best, score = as.URL, sc
			}
		}
		if best == "" {
			return fmt.Errorf("the latest release of %s has no Vita app to download", repo)
		}
		raw = best
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") {
		return errors.New("paste a GitHub link or a direct link to a .vpk or .zip file")
	}
	name := path.Base(u.Path)
	if ext := strings.ToLower(path.Ext(name)); ext != ".vpk" && ext != ".zip" {
		return errors.New("that link is not a .vpk or .zip file")
	}
	dir := filepath.Join(a.cache, "downloads")
	os.MkdirAll(dir, 0o755)
	dest := filepath.Join(dir, name)
	if err := a.download(ctx, raw, dest, name); err != nil {
		return err
	}
	return a.Add(dest)
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
