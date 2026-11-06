package web

import (
	"context"
	"errors"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/CBOC-Vita-Lab/vita-installer/internal/art"
	"github.com/CBOC-Vita-Lab/vita-installer/internal/ftp"
)

// gameCapture is a picture a game takes of itself on the Vita and keeps as a
// raw RGBA frame: the public package carries no game artwork, so this is the
// player's own copy.
type gameCapture struct {
	path  string
	w, h  int
	title string
}

var gameCaptures = map[string][]gameCapture{
	"SMGV00001": {{"ux0:data/smg/art/title.rgba", 960, 544, "Title screen (taken by the game on your Vita)"}},
}

// maxScreenshots is how many of a game's newest screenshots are offered.
const maxScreenshots = 24

// vitaPictures copies the game's own pictures from the Vita (over Wi-Fi):
// what the game captured of itself, and the screenshots taken in it
// (ux0:picture/SCREENSHOT/<title ID>/). Each is offered for the bubble and
// the background.
func (a *App) vitaPictures(ctx context.Context, itemID string) ([]art.Candidate, error) {
	a.mu.Lock()
	it := a.findItem(itemID)
	v := a.st.Vita
	var title string
	if it != nil {
		title = it.TitleID
	}
	a.mu.Unlock()
	if it == nil {
		return nil, errors.New("that game is no longer in the list")
	}
	if v == nil {
		return nil, errors.New("connect your Vita over Wi-Fi (VitaShell open, FTP on) to use its pictures")
	}
	c, err := ftp.Dial(ctx, v.Addr(), 8*time.Second)
	if err != nil {
		return nil, errors.New("could not reach your Vita. Keep VitaShell open and try again")
	}
	defer c.Close()
	dir := filepath.Join(art.VitaDir(a.cache), title)
	os.MkdirAll(dir, 0o755)
	var out []art.Candidate
	offer := func(file, name string, w, h int) {
		for _, slot := range []string{"icon", "background"} {
			out = append(out, art.Candidate{Source: "vita", Title: name, Kind: "vita", Slot: slot, URL: art.LinkFor(file), Thumb: art.LinkFor(file), Width: w, Height: h})
		}
	}
	for i, g := range gameCaptures[title] {
		b, err := c.Retrieve(g.path, int64(g.w*g.h*4))
		if err != nil {
			continue
		}
		p, err := art.RawRGBA(b, g.w, g.h)
		if err != nil {
			continue
		}
		file := filepath.Join(dir, "capture"+string(rune('0'+i))+".png")
		if os.WriteFile(file, p, 0o644) == nil {
			offer(file, g.title, g.w, g.h)
		}
	}
	shots := "ux0:picture/SCREENSHOT/" + title
	entries, err := c.List(shots)
	if err == nil {
		var names []string
		for _, e := range entries {
			ext := strings.ToLower(path.Ext(e.Name))
			if !e.IsDir && e.Size > 0 && e.Size < 16<<20 && (ext == ".jpg" || ext == ".jpeg" || ext == ".png") {
				names = append(names, e.Name)
			}
		}
		// The Vita names screenshots by date and time, so the last names are the newest.
		sort.Sort(sort.Reverse(sort.StringSlice(names)))
		if len(names) > maxScreenshots {
			names = names[:maxScreenshots]
		}
		for _, n := range names {
			if ctx.Err() != nil {
				break
			}
			file := filepath.Join(dir, "shot-"+filepath.Base(n))
			if _, err := os.Stat(file); err != nil {
				b, err := c.Retrieve(shots+"/"+n, 16<<20)
				if err != nil {
					continue
				}
				if _, err := art.Decode(strings.NewReader(string(b))); err != nil {
					continue
				}
				if os.WriteFile(file, b, 0o644) != nil {
					continue
				}
			}
			offer(file, "Screenshot "+strings.TrimSuffix(n, path.Ext(n)), 0, 0)
		}
	}
	if len(out) == 0 {
		return nil, errors.New("no pictures of this game on your Vita yet. Take a screenshot in the game (PS + Start), then look again")
	}
	return out, nil
}
