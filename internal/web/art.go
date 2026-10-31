package web

import (
	"archive/zip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/CBOC-Vita-Lab/vita-installer/internal/art"
	"github.com/CBOC-Vita-Lab/vita-installer/internal/release"
)

// ArtInfo is what the page shows about an item's artwork.
type ArtInfo struct {
	Custom  bool              `json:"custom"`
	Slots   map[string]string `json:"slots,omitempty"` // slot -> where it came from
	Seq     int               `json:"seq"`             // changes when the pictures change
	Preview bool              `json:"preview"`         // the game's own pictures are known (not before a game that is built on this computer has been built)
}

// savedArt is a game's artwork choice, kept for later installs and updates.
type savedArt struct {
	Choice art.Choice        `json:"choice"`
	Labels map[string]string `json:"labels"`
}

var artSlots = []string{"icon", "background", "logo"}

func (a *App) artDir(title string) string { return filepath.Join(a.config, "art", title) }

func (a *App) loadArt(title string) savedArt {
	var s savedArt
	if b, err := os.ReadFile(filepath.Join(a.artDir(title), "choice.json")); err == nil {
		json.Unmarshal(b, &s)
	}
	// Forget pictures that are gone.
	for _, p := range []*string{&s.Choice.Icon, &s.Choice.Background, &s.Choice.Logo} {
		if *p != "" {
			if _, err := os.Stat(*p); err != nil {
				*p = ""
			}
		}
	}
	if s.Labels == nil {
		s.Labels = map[string]string{}
	}
	return s
}

func (a *App) saveArt(title string, s savedArt) {
	os.MkdirAll(a.artDir(title), 0o755)
	b, _ := json.MarshalIndent(s, "", " ")
	os.WriteFile(filepath.Join(a.artDir(title), "choice.json"), b, 0o644)
}

func slotPtr(c *art.Choice, slot string) *string {
	switch slot {
	case "icon":
		return &c.Icon
	case "background":
		return &c.Background
	case "logo":
		return &c.Logo
	}
	return nil
}

// applyArt puts a game's saved artwork on its item (called when it is added).
func (a *App) applyArt(it *Item) {
	s := a.loadArt(it.TitleID)
	it.Release.Art = s.Choice
	info := &ArtInfo{Custom: !s.Choice.Empty(), Slots: map[string]string{}, Seq: int(time.Now().UnixNano() % 1e9), Preview: it.Release.VPK != ""}
	for _, slot := range artSlots {
		if p := slotPtr(&s.Choice, slot); p != nil && *p != "" {
			info.Slots[slot] = s.Labels[slot]
		}
	}
	it.Art = info
}

func (a *App) findItem(id string) *Item {
	for _, it := range a.st.Items {
		if it.ID == id {
			return it
		}
	}
	return nil
}

// SetArt sets one slot of an item's artwork from a local picture, a link
// from the artwork sources, or clears it ("" for both; slot "all" clears all).
func (a *App) SetArt(ctx context.Context, itemID, slot, path, link, label string) error {
	a.mu.Lock()
	it := a.findItem(itemID)
	a.mu.Unlock()
	if it == nil {
		return errors.New("that game is no longer in the list")
	}
	if it.Status == "working" || it.Status == "queued" {
		return errors.New("wait until the installation finishes")
	}
	s := a.loadArt(it.TitleID)
	switch {
	case slot == "all":
		os.RemoveAll(a.artDir(it.TitleID))
		s = savedArt{Labels: map[string]string{}}
	case slotPtr(&s.Choice, slot) == nil:
		return errors.New("unknown artwork slot")
	case path == "" && link == "":
		*slotPtr(&s.Choice, slot) = ""
		delete(s.Labels, slot)
	default:
		src := path
		if link != "" {
			a.update(func(st *State) { st.Task = &Task{Text: "Getting the picture"} })
			f, err := art.Fetch(ctx, a.cache, link)
			a.update(func(st *State) { st.Task = nil })
			if err != nil {
				return err
			}
			src = f
		}
		if _, err := art.Load(src); err != nil {
			return err
		}
		// Keep a copy with the game's settings, so it survives cache clean-ups
		// and is used again for updates.
		os.MkdirAll(a.artDir(it.TitleID), 0o755)
		ext := strings.ToLower(filepath.Ext(src))
		if ext != ".png" && ext != ".jpg" && ext != ".jpeg" && ext != ".gif" {
			ext = ".img"
		}
		dst := filepath.Join(a.artDir(it.TitleID), slot+ext)
		for _, old := range []string{".png", ".jpg", ".jpeg", ".gif", ".img"} {
			os.Remove(filepath.Join(a.artDir(it.TitleID), slot+old))
		}
		if err := copyFile(src, dst); err != nil {
			return err
		}
		*slotPtr(&s.Choice, slot) = dst
		if label == "" {
			label = "Your picture"
		}
		s.Labels[slot] = label
	}
	a.saveArt(it.TitleID, s)
	a.update(func(st *State) {
		for _, x := range st.Items {
			if x.TitleID == it.TitleID {
				a.applyArt(x)
				if x.Status == "done" {
					x.Status = ""
					setStatus(x)
				}
			}
		}
	})
	go a.refreshItems()
	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// ArtPreview renders exactly what the Vita will show for a slot: "icon",
// "background" or "startup".
func (a *App) ArtPreview(itemID, slot string) ([]byte, error) {
	a.mu.Lock()
	it := a.findItem(itemID)
	var vpk string
	var choice art.Choice
	if it != nil {
		vpk, choice = it.VPK, it.Release.Art
	}
	a.mu.Unlock()
	if it == nil {
		return nil, errors.New("no such game")
	}
	if vpk == "" {
		return nil, errors.New("this game has not been built yet")
	}
	zr, err := zip.OpenReader(vpk)
	if err != nil {
		return nil, err
	}
	defer zr.Close()
	r, err := art.Render(choice, &zr.Reader)
	if err != nil {
		return nil, err
	}
	paths := art.Find(&zr.Reader)
	var img image.Image
	switch slot {
	case "icon":
		img = r.Icon
		if img == nil {
			img = art.Original(&zr.Reader, paths.Icon)
		}
	case "background":
		img = r.Background
		if img == nil {
			img = art.Original(&zr.Reader, paths.Background)
		}
	case "startup":
		img = r.Startup
		if img == nil {
			img = art.Original(&zr.Reader, paths.Startup)
		}
	}
	if img == nil {
		return nil, errors.New("no picture")
	}
	return art.EncodePreview(img), nil
}

// ArtSearch looks for pictures. source: "libretro" (query, optional system),
// "steamgriddb" (query: returns games) or "steamgriddb-game" (game ID).
func (a *App) ArtSearch(ctx context.Context, itemID, source, query, system string) (interface{}, error) {
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	switch source {
	case "libretro":
		systems := []string{system}
		if system == "" {
			systems = a.systemsFor(itemID)
		}
		c, err := art.Libretro(ctx, a.cache, systems, query)
		return map[string]interface{}{"candidates": c}, err
	case "steamgriddb":
		g, err := art.SteamGridGames(ctx, a.steamGridKey(), query)
		return map[string]interface{}{"games": g}, err
	case "steamgriddb-game":
		var id int
		fmt.Sscan(query, &id)
		c, err := art.SteamGridImages(ctx, a.steamGridKey(), id, system)
		return map[string]interface{}{"candidates": c}, err
	}
	return nil, errors.New("unknown artwork source")
}

// systemsFor guesses which libretro systems to search from what the game
// asks the player for.
func (a *App) systemsFor(itemID string) []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	var out []string
	if it := a.findItem(itemID); it != nil {
		for _, in := range it.Inputs {
			switch in.Type {
			case release.NeedWiiDisc:
				out = append(out, "Nintendo - Wii")
			case release.NeedGCDisc:
				out = append(out, "Nintendo - GameCube")
			case release.NeedPSPISO:
				out = append(out, "Sony - PlayStation Portable")
			}
		}
	}
	if len(out) == 0 {
		out = []string{"Sony - PlayStation Vita", "Sony - PlayStation Portable", "Nintendo - Wii", "Nintendo - GameCube"}
	}
	return out
}

func (a *App) steamGridKey() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.sgdbKey
}

// SetSteamGridKey checks and saves the player's SteamGridDB API key ("" removes it).
func (a *App) SetSteamGridKey(ctx context.Context, key string) error {
	key = strings.TrimSpace(key)
	if key != "" {
		cctx, cancel := context.WithTimeout(ctx, 20*time.Second)
		defer cancel()
		if err := art.CheckKey(cctx, key); err != nil {
			return err
		}
	}
	a.mu.Lock()
	a.sgdbKey = key
	a.mu.Unlock()
	a.saveSettings()
	a.update(func(s *State) { s.SteamGrid = key != "" })
	return nil
}

func replyJSON(w http.ResponseWriter, v interface{}, err error) {
	if err != nil {
		reply(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	m := map[string]interface{}{"ok": true}
	if mm, ok := v.(map[string]interface{}); ok {
		for k, x := range mm {
			m[k] = x
		}
	}
	json.NewEncoder(w).Encode(m)
}
