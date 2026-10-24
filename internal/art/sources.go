package art

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

// Candidate is a picture the player can pick.
type Candidate struct {
	Source string `json:"source"` // libretro, steamgriddb
	Title  string `json:"title"`
	Kind   string `json:"kind"` // boxart, title, snap, grid, hero, logo, icon
	Slot   string `json:"slot"` // icon, background, logo
	URL    string `json:"url"`
	Thumb  string `json:"thumb"`
	Width  int    `json:"width,omitempty"`
	Height int    `json:"height,omitempty"`
	Credit string `json:"credit,omitempty"`
}

var client = &http.Client{Timeout: 45 * time.Second}

func get(ctx context.Context, u string, hdr map[string]string, limit int64) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("User-Agent", "cboc-vita-installer")
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, limit))
	return b, resp.StatusCode, err
}

// ---- libretro-thumbnails ----

// Systems are libretro's names for the platforms Vita ports come from.
var Systems = []string{
	"Sony - PlayStation Vita", "Sony - PlayStation Portable", "Nintendo - Wii", "Nintendo - GameCube",
	"Sony - PlayStation 2", "Sony - PlayStation", "Microsoft - Xbox 360", "Nintendo - Nintendo DS",
	"Nintendo - Nintendo 64", "Nintendo - Game Boy Advance", "Nintendo - Super Nintendo Entertainment System", "Sega - Dreamcast",
}

const libretroBase = "https://thumbnails.libretro.com/"

var kinds = []struct{ dir, kind, slot string }{
	{"Named_Boxarts", "boxart", "icon"},
	{"Named_Titles", "title", "background"},
	{"Named_Snaps", "snap", "background"},
}

var hrefPNG = regexp.MustCompile(`href="([^"?/]+\.png)"`)

// listing returns the picture names in one libretro folder, cached a week.
func listing(ctx context.Context, cache, system, dir string) ([]string, error) {
	file := filepath.Join(cache, "art", "libretro", hashOf(system+"/"+dir)+".txt")
	if st, err := os.Stat(file); err == nil && time.Since(st.ModTime()) < 7*24*time.Hour {
		b, _ := os.ReadFile(file)
		return strings.Split(strings.TrimSpace(string(b)), "\n"), nil
	}
	b, code, err := get(ctx, libretroBase+url.PathEscape(system)+"/"+dir+"/", nil, 32<<20)
	if err != nil {
		return nil, err
	}
	if code != 200 {
		return nil, fmt.Errorf("libretro-thumbnails answered %d", code)
	}
	var names []string
	for _, m := range hrefPNG.FindAllSubmatch(b, -1) {
		n, err := url.PathUnescape(html.UnescapeString(string(m[1])))
		if err == nil {
			names = append(names, strings.TrimSuffix(n, ".png"))
		}
	}
	os.MkdirAll(filepath.Dir(file), 0o755)
	os.WriteFile(file, []byte(strings.Join(names, "\n")), 0o644)
	return names, nil
}

var reTags = regexp.MustCompile(`\([^)]*\)|\[[^\]]*\]`)
var reNonWord = regexp.MustCompile(`[^a-z0-9]+`)

func normalize(s string) []string {
	s = strings.ToLower(reTags.ReplaceAllString(s, " "))
	s = strings.ReplaceAll(s, "&", " and ")
	s = strings.ReplaceAll(s, "'", "")
	f := strings.Fields(reNonWord.ReplaceAllString(s, " "))
	var out []string
	for _, w := range f {
		if w != "the" && w != "a" {
			out = append(out, w)
		}
	}
	return out
}

// match scores how well a libretro title fits the query (0..1).
func match(query []string, title string) float64 {
	t := normalize(title)
	if len(query) == 0 || len(t) == 0 {
		return 0
	}
	in := map[string]bool{}
	for _, w := range t {
		in[w] = true
	}
	hit := 0
	for _, w := range query {
		if in[w] {
			hit++
		}
	}
	if hit == 0 {
		return 0
	}
	recall := float64(hit) / float64(len(query))
	precision := float64(hit) / float64(len(t))
	return recall*0.7 + precision*0.3
}

// Libretro looks the game up in libretro-thumbnails (no key needed).
func Libretro(ctx context.Context, cache string, systems []string, query string) ([]Candidate, error) {
	q := normalize(query)
	if len(q) == 0 {
		return nil, errors.New("type the game's name")
	}
	type found struct {
		system, title string
		score         float64
	}
	var mu sync.Mutex
	var hits []found
	have := map[string]map[string]bool{} // system/dir -> names
	var firstErr error
	var wg sync.WaitGroup
	for _, sys := range systems {
		for _, k := range kinds {
			sys, k := sys, k
			wg.Add(1)
			go func() {
				defer wg.Done()
				names, err := listing(ctx, cache, sys, k.dir)
				mu.Lock()
				defer mu.Unlock()
				if err != nil {
					if firstErr == nil {
						firstErr = err
					}
					return
				}
				set := map[string]bool{}
				for _, n := range names {
					set[n] = true
					if k.dir == "Named_Boxarts" || k.dir == "Named_Titles" {
						if s := match(q, n); s >= 0.6 {
							hits = append(hits, found{sys, n, s})
						}
					}
				}
				have[sys+"/"+k.dir] = set
			}()
		}
	}
	wg.Wait()
	if len(hits) == 0 && firstErr != nil {
		return nil, fmt.Errorf("libretro-thumbnails could not be reached: %v", firstErr)
	}
	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].score != hits[j].score {
			return hits[i].score > hits[j].score
		}
		ui, uj := strings.Contains(hits[i].title, "(USA)"), strings.Contains(hits[j].title, "(USA)")
		if ui != uj {
			return ui
		}
		return hits[i].title < hits[j].title
	})
	seen := map[string]bool{}
	var out []Candidate
	for _, h := range hits {
		key := h.system + "/" + h.title
		if seen[key] {
			continue
		}
		seen[key] = true
		if len(seen) > 8 {
			break
		}
		for _, k := range kinds {
			if !have[h.system+"/"+k.dir][h.title] {
				continue
			}
			u := libretroBase + url.PathEscape(h.system) + "/" + k.dir + "/" + url.PathEscape(h.title) + ".png"
			out = append(out, Candidate{Source: "libretro", Title: h.title + " · " + shortSystem(h.system), Kind: k.kind, Slot: k.slot, URL: u, Thumb: u})
			if k.kind == "title" { // title screens also make good icons
				out = append(out, Candidate{Source: "libretro", Title: h.title + " · " + shortSystem(h.system), Kind: k.kind, Slot: "icon", URL: u, Thumb: u})
			}
		}
	}
	if len(out) == 0 {
		return nil, errors.New("nothing found on libretro-thumbnails. Try a shorter name, or another system")
	}
	return out, nil
}

func shortSystem(s string) string {
	if i := strings.Index(s, " - "); i >= 0 {
		return s[i+3:]
	}
	return s
}

// ---- SteamGridDB ----

var sgdbAPI = "https://www.steamgriddb.com/api/v2/" // a variable for tests

// Game is a SteamGridDB search result.
type Game struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
	Year int    `json:"year,omitempty"`
}

func sgdb(ctx context.Context, key, path string, v interface{}) error {
	if strings.TrimSpace(key) == "" {
		return errors.New("add your SteamGridDB API key first")
	}
	b, code, err := get(ctx, sgdbAPI+path, map[string]string{"Authorization": "Bearer " + strings.TrimSpace(key)}, 8<<20)
	if err != nil {
		return fmt.Errorf("SteamGridDB could not be reached: %v", err)
	}
	if code == 401 || code == 403 {
		return errors.New("SteamGridDB did not accept the API key. Check it on steamgriddb.com > Preferences > API")
	}
	var res struct {
		Success bool            `json:"success"`
		Data    json.RawMessage `json:"data"`
		Errors  []string        `json:"errors"`
	}
	if err := json.Unmarshal(b, &res); err != nil {
		return fmt.Errorf("SteamGridDB answered %d", code)
	}
	if !res.Success {
		if code == 404 {
			return json.Unmarshal([]byte("[]"), v)
		}
		return fmt.Errorf("SteamGridDB: %s", strings.Join(res.Errors, ", "))
	}
	return json.Unmarshal(res.Data, v)
}

// CheckKey makes a tiny request to see whether a key works.
func CheckKey(ctx context.Context, key string) error {
	var g []Game
	return sgdb(ctx, key, "search/autocomplete/"+url.PathEscape("mario"), &g)
}

// SteamGridGames searches SteamGridDB for a game.
func SteamGridGames(ctx context.Context, key, query string) ([]Game, error) {
	if strings.TrimSpace(query) == "" {
		return nil, errors.New("type the game's name")
	}
	var raw []struct {
		ID          int    `json:"id"`
		Name        string `json:"name"`
		ReleaseDate int64  `json:"release_date"`
	}
	if err := sgdb(ctx, key, "search/autocomplete/"+url.PathEscape(query), &raw); err != nil {
		return nil, err
	}
	var out []Game
	for _, r := range raw {
		g := Game{ID: r.ID, Name: r.Name}
		if r.ReleaseDate > 0 {
			g.Year = time.Unix(r.ReleaseDate, 0).UTC().Year()
		}
		out = append(out, g)
	}
	if len(out) == 0 {
		return nil, errors.New("SteamGridDB has nothing by that name")
	}
	return out, nil
}

// SteamGridImages lists a SteamGridDB game's pictures: square and tall
// grids and icons for the bubble, heroes for the background, logos.
func SteamGridImages(ctx context.Context, key string, game int, name string) ([]Candidate, error) {
	type img struct {
		URL    string `json:"url"`
		Thumb  string `json:"thumb"`
		Width  int    `json:"width"`
		Height int    `json:"height"`
		Author struct {
			Name string `json:"name"`
		} `json:"author"`
	}
	common := "mimes=image/png,image/jpeg&types=static&nsfw=false"
	queries := []struct{ path, kind, slot string }{
		{fmt.Sprintf("grids/game/%d?dimensions=512x512,1024x1024&%s", game, common), "grid", "icon"},
		{fmt.Sprintf("icons/game/%d?%s", game, "mimes=image/png&types=static&nsfw=false"), "icon", "icon"},
		{fmt.Sprintf("grids/game/%d?dimensions=600x900,342x482,660x930&%s", game, common), "grid", "icon"},
		{fmt.Sprintf("heroes/game/%d?%s", game, common), "hero", "background"},
		{fmt.Sprintf("grids/game/%d?dimensions=920x430,460x215&%s", game, common), "grid", "background"},
		{fmt.Sprintf("logos/game/%d?%s", game, "mimes=image/png&types=static&nsfw=false"), "logo", "logo"},
	}
	results := make([][]Candidate, len(queries))
	errs := make([]error, len(queries))
	var wg sync.WaitGroup
	for i, q := range queries {
		i, q := i, q
		wg.Add(1)
		go func() {
			defer wg.Done()
			var imgs []img
			if errs[i] = sgdb(ctx, key, q.path, &imgs); errs[i] != nil {
				return
			}
			for j, m := range imgs {
				if j >= 24 {
					break
				}
				results[i] = append(results[i], Candidate{Source: "steamgriddb", Title: name, Kind: q.kind, Slot: q.slot,
					URL: m.URL, Thumb: m.Thumb, Width: m.Width, Height: m.Height, Credit: m.Author.Name})
			}
		}()
	}
	wg.Wait()
	var out []Candidate
	for i := range results {
		out = append(out, results[i]...)
	}
	if len(out) == 0 {
		for _, e := range errs {
			if e != nil {
				return nil, e
			}
		}
		return nil, errors.New("SteamGridDB has no pictures for this game yet")
	}
	return out, nil
}

// ---- downloading a chosen picture ----

var allowedHosts = []string{"thumbnails.libretro.com", "steamgriddb.com", "raw.githubusercontent.com"}

// Fetch downloads a picture (from the sources above) into the cache and
// checks it can be read. Returns the local file.
func Fetch(ctx context.Context, cache, raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" {
		return "", errors.New("not a picture link")
	}
	ok := false
	for _, h := range allowedHosts {
		if u.Hostname() == h || strings.HasSuffix(u.Hostname(), "."+h) {
			ok = true
		}
	}
	if !ok {
		return "", errors.New("pictures can only come from libretro-thumbnails or SteamGridDB")
	}
	ext := strings.ToLower(filepath.Ext(u.Path))
	if ext != ".png" && ext != ".jpg" && ext != ".jpeg" {
		ext = ".img"
	}
	file := filepath.Join(cache, "art", "img", hashOf(raw)+ext)
	if _, err := os.Stat(file); err == nil {
		return file, nil
	}
	b, code, err := get(ctx, raw, nil, 40<<20)
	if err != nil {
		return "", err
	}
	if code != 200 {
		return "", fmt.Errorf("the picture could not be downloaded (HTTP %d)", code)
	}
	if _, err := Decode(strings.NewReader(string(b))); err != nil {
		return "", err
	}
	os.MkdirAll(filepath.Dir(file), 0o755)
	if err := os.WriteFile(file+".part", b, 0o644); err != nil {
		return "", err
	}
	return file, os.Rename(file+".part", file)
}

func hashOf(s string) string {
	h := sha1.Sum([]byte(s))
	return hex.EncodeToString(h[:10])
}

// Thumb returns a small PNG of a picture from the sources, for the page's
// result grids (downloaded once, then cached).
func Thumb(ctx context.Context, cache, raw string) ([]byte, error) {
	file := filepath.Join(cache, "art", "thumb", hashOf(raw)+".png")
	if b, err := os.ReadFile(file); err == nil {
		return b, nil
	}
	src, err := Fetch(ctx, cache, raw)
	if err != nil {
		return nil, err
	}
	img, err := Load(src)
	if err != nil {
		return nil, err
	}
	b := EncodePreview(Contain(img, 320, 320))
	os.MkdirAll(filepath.Dir(file), 0o755)
	os.WriteFile(file, b, 0o644)
	return b, nil
}
