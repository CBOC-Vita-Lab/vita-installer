package web

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/CBOC-Vita-Lab/vita-installer/internal/art"
	"github.com/CBOC-Vita-Lab/vita-installer/internal/ftp"
	"github.com/CBOC-Vita-Lab/vita-installer/internal/install"
)

// Refreshing artwork on connect.
//
// The installer builds a game's bubble and LiveArea pictures from the
// player's own pictures when it installs. Some of those pictures only exist
// after the game has run once, so whenever the Vita connects the installer
// checks, cheaply, whether the pictures should be brought up to date, and if
// so writes just the picture files (never game data, saves or the eboot).
//
// What stays on the Vita is one small marker per game,
// ux0:data/cboc-installer/art/<TITLEID>.json, next to the install receipts.
// It says which source the pictures were made from and which install they
// were written for. When nothing changed the check is one directory listing
// of the source and one small file read per game.

const (
	artMarkerDir = install.Staging + "/art"
	artBackupDir = install.Staging + "/art-backup"
)

// artMarker is what the Vita keeps about the pictures last written.
type artMarker struct {
	Version   int       `json:"version"`
	Source    string    `json:"source"`    // which picture they were made from (name, size, date)
	SHA256    string    `json:"sha256"`    // of the set of pictures written
	Install   string    `json:"install"`   // the install (receipt time) they were written for
	Files     []string  `json:"files"`     // where they were written
	Icon      string    `json:"icon"`      // sha256 of the bubble written, to notice a later change by hand
	UpdatedAt time.Time `json:"updatedAt"` //
}

// artSource is the picture a game's artwork is made from.
type artSource struct {
	sig   string // identifies it without reading it: kind, name, size, date
	path  string // on the Vita
	w, h  int    // size of a raw capture; 0 for a picture file
	title string
}

// artRole is one of the pictures of a game.
var artRoles = []string{"icon", "background", "startup", "pic0"}

// artAutoOn reports whether pictures are refreshed on connect.
func (a *App) artAutoOn() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.st.ArtAuto
}

// SetArtAuto turns the automatic refresh on or off.
func (a *App) SetArtAuto(on bool) {
	a.update(func(s *State) { s.ArtAuto = on })
	a.saveSettings()
	if on {
		a.background(a.refreshItems)
	}
}

// RefreshArt brings the artwork of every game this installer put on the Vita
// up to date now, whatever the setting says.
func (a *App) RefreshArt(ctx context.Context) error {
	a.mu.Lock()
	v := a.st.Vita
	a.mu.Unlock()
	if v == nil {
		return errors.New("connect your Vita over Wi-Fi (VitaShell open, FTP on) to refresh its artwork")
	}
	if a.Busy() {
		return errors.New("wait until the installation finishes")
	}
	rs, err := install.Installed(ctx, v, "")
	if err != nil {
		return errors.New("could not reach your Vita. Keep VitaShell open and try again")
	}
	n, firstErr := a.refreshArt(ctx, rs, true)
	switch {
	case firstErr != nil && n == 0:
		return firstErr
	case n == 0:
		a.notice("Your artwork is already up to date.", false)
	}
	return nil
}

// refreshArtAuto is the automatic check after the Vita's games are read.
func (a *App) refreshArtAuto(rs []*install.Receipt) {
	a.mu.Lock()
	v := a.st.Vita
	a.mu.Unlock()
	if v == nil || !a.artAutoOn() || a.Busy() {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	a.refreshArt(ctx, rs, false)
}

// refreshArt checks each managed game and refreshes its pictures when they
// should change (or always, with force). It returns how many games it
// updated and the first problem.
func (a *App) refreshArt(ctx context.Context, rs []*install.Receipt, force bool) (int, error) {
	if !a.artMu.TryLock() {
		return 0, nil
	}
	defer a.artMu.Unlock()
	a.mu.Lock()
	v := a.st.Vita
	repos := map[string]bool{}
	for _, c := range a.st.Catalog {
		repos[c.Repo] = true
	}
	a.mu.Unlock()
	if v == nil {
		return 0, nil
	}
	var todo []*install.Receipt
	seen := map[string]bool{}
	for _, r := range rs {
		_, known := gameCaptures[r.TitleID]
		if seen[r.TitleID] || !(repos[r.Repo] && r.Repo != "" || known) {
			continue
		}
		seen[r.TitleID] = true
		// Pictures the player chose are theirs to keep.
		if !a.loadArt(r.TitleID).Choice.Empty() {
			continue
		}
		todo = append(todo, r)
	}
	if len(todo) == 0 {
		return 0, nil
	}
	c, err := ftp.Dial(ctx, v.Addr(), 8*time.Second)
	if err != nil {
		return 0, errors.New("could not reach your Vita. Keep VitaShell open and try again")
	}
	defer c.Close()
	var done []string
	var firstErr error
	for _, r := range todo {
		if ctx.Err() != nil {
			break
		}
		changed, err := a.refreshArtOf(ctx, c, r, force)
		if err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("%s: %v", r.Name, err)
			}
			continue
		}
		if changed {
			done = append(done, r.Name)
		}
	}
	if len(done) > 0 {
		a.notice("Artwork refreshed for "+strings.Join(done, ", ")+". The new bubble and LiveArea pictures show after you restart your Vita (or use Refresh LiveArea in VitaShell).", false)
	}
	return len(done), firstErr
}

// findArtSource looks (with one directory listing) for the newest picture of
// the game on the Vita: what the game captured of itself, else the newest
// screenshot. It returns nil when there is none.
func findArtSource(c *ftp.Conn, title string) (*artSource, error) {
	for _, g := range gameCaptures[title] {
		es, err := c.List(path.Dir(g.path))
		if err != nil {
			if ftp.IsNotFound(err) {
				continue
			}
			return nil, err
		}
		for _, e := range es {
			if e.Name == path.Base(g.path) && !e.IsDir && e.Size == int64(g.w*g.h*4) {
				return &artSource{sig: fmt.Sprintf("capture %s %d %s", e.Name, e.Size, e.Date), path: g.path, w: g.w, h: g.h, title: title}, nil
			}
		}
	}
	dir := "ux0:picture/SCREENSHOT/" + title
	es, err := c.List(dir)
	if err != nil {
		if ftp.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	var best *ftp.Entry
	for i, e := range es {
		ext := strings.ToLower(path.Ext(e.Name))
		if e.IsDir || e.Size <= 0 || e.Size >= 16<<20 || (ext != ".jpg" && ext != ".jpeg" && ext != ".png") {
			continue
		}
		// The Vita names screenshots by date and time: the last name is the newest.
		if best == nil || e.Name > best.Name {
			best = &es[i]
		}
	}
	if best == nil {
		return nil, nil
	}
	return &artSource{sig: fmt.Sprintf("shot %s %d %s", best.Name, best.Size, best.Date), path: dir + "/" + best.Name, title: title}, nil
}

func readArtMarker(c *ftp.Conn, title string) *artMarker {
	b, err := c.Retrieve(artMarkerDir+"/"+title+".json", 1<<20)
	if err != nil {
		return nil
	}
	var m artMarker
	if json.Unmarshal(b, &m) != nil || m.Version != 1 {
		return nil
	}
	return &m
}

// refreshArtOf checks one game and updates its pictures when needed.
func (a *App) refreshArtOf(ctx context.Context, c *ftp.Conn, r *install.Receipt, force bool) (bool, error) {
	src, err := findArtSource(c, r.TitleID)
	if err != nil || src == nil {
		return false, err
	}
	stamp := r.InstalledAt.UTC().Format(time.RFC3339Nano)
	m := readArtMarker(c, r.TitleID)
	if !force && m != nil && m.Source == src.sig && m.Install == stamp {
		return false, nil // nothing changed: no more than the listing and the marker were touched
	}
	if !force && !a.artIsOurs(c, r, m) {
		return false, nil // the player has put their own pictures there: leave them
	}
	set, err := a.artSetFor(c, src)
	if err != nil {
		return false, err
	}
	if !force && m != nil && m.SHA256 == set.sum && m.Install == stamp {
		// The source file changed but makes the same pictures: only note it.
		m.Source = src.sig
		return false, putArtMarker(ctx, c, r.TitleID, m)
	}
	files, err := pushArt(ctx, c, r.TitleID, set)
	if err != nil {
		return false, err
	}
	if files == nil {
		return false, nil // the game is no longer on the Vita
	}
	return true, putArtMarker(ctx, c, r.TitleID, &artMarker{Version: 1, Source: src.sig, SHA256: set.sum, Install: stamp, Files: files, Icon: sha256Hex(set.png["icon"]), UpdatedAt: time.Now().UTC()})
}

func putArtMarker(ctx context.Context, c *ftp.Conn, title string, m *artMarker) error {
	b, _ := json.MarshalIndent(m, "", " ")
	if err := mkdirAll(c, artMarkerDir); err != nil {
		return err
	}
	return putVerified(ctx, c, artMarkerDir+"/"+title+".json", b)
}

// artSet is the made pictures of a game, by role.
type artSet struct {
	png map[string][]byte
	sum string
}

// artSetFor returns the pictures made from src: from the cache when they
// were made before (the picture on the Vita is then not downloaded), else
// from the picture, which is read from the Vita once.
func (a *App) artSetFor(c *ftp.Conn, src *artSource) (*artSet, error) {
	h := sha256.Sum256([]byte(src.title + "\x00" + src.sig))
	base := filepath.Join(a.cache, "art", "refresh", src.title)
	dir := filepath.Join(base, hex.EncodeToString(h[:8]))
	if s := loadArtSet(dir); s != nil {
		return s, nil
	}
	var b []byte
	var err error
	if src.w > 0 {
		var raw []byte
		if raw, err = c.Retrieve(src.path, int64(src.w*src.h*4)+1); err == nil {
			b, err = art.RawRGBA(raw, src.w, src.h)
		}
	} else {
		b, err = c.Retrieve(src.path, 16<<20)
	}
	if err != nil {
		return nil, fmt.Errorf("could not read the picture on your Vita: %v", err)
	}
	os.RemoveAll(base) // older sets of this game are not needed again
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	srcFile := filepath.Join(dir, "source.img")
	if err := os.WriteFile(srcFile, b, 0o644); err != nil {
		return nil, err
	}
	// The same pictures an install makes: a bubble icon and a background
	// (with its start image and pic0) from the one picture.
	r, err := art.Render(art.Choice{Icon: srcFile, Background: srcFile}, nil)
	if err != nil {
		return nil, err
	}
	set := &artSet{png: map[string][]byte{}}
	for _, e := range []struct {
		role string
		img  image.Image
	}{{"icon", r.Icon}, {"background", r.Background}, {"startup", r.Startup}, {"pic0", r.Pic0}} {
		if e.img == nil {
			continue
		}
		p, err := art.EncodeVita(e.img)
		if err != nil {
			return nil, err
		}
		set.png[e.role] = p
	}
	set.sum = sumOfSet(set.png)
	for role, p := range set.png {
		if err := os.WriteFile(filepath.Join(dir, role+".png"), p, 0o644); err != nil {
			return nil, err
		}
	}
	os.WriteFile(filepath.Join(dir, "sha256"), []byte(set.sum), 0o644) // written last: the set is complete
	return set, nil
}

func sumOfSet(m map[string][]byte) string {
	h := sha256.New()
	roles := make([]string, 0, len(m))
	for r := range m {
		roles = append(roles, r)
	}
	sort.Strings(roles)
	for _, r := range roles {
		fmt.Fprintf(h, "%s\x00%d\x00", r, len(m[r]))
		h.Write(m[r])
	}
	return hex.EncodeToString(h.Sum(nil))
}

func loadArtSet(dir string) *artSet {
	want, err := os.ReadFile(filepath.Join(dir, "sha256"))
	if err != nil {
		return nil
	}
	s := &artSet{png: map[string][]byte{}}
	for _, role := range artRoles {
		if b, err := os.ReadFile(filepath.Join(dir, role+".png")); err == nil {
			s.png[role] = b
		}
	}
	s.sum = sumOfSet(s.png)
	if s.sum != string(want) || len(s.png) == 0 {
		return nil
	}
	return s
}

// ---- Writing to the Vita ----

type artTarget struct {
	to   string // Vita path
	data []byte
}

// pushArt writes the pictures into the app and into the system's copy, after
// keeping the previous files once. It returns where it wrote, or nil when the
// game is not on the Vita.
func pushArt(ctx context.Context, c *ftp.Conn, title string, set *artSet) ([]string, error) {
	app := "ux0:app/" + title
	sys, err := c.List(app + "/sce_sys")
	if err != nil {
		if ftp.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	names := map[string]bool{}
	for _, e := range sys {
		names["sce_sys/"+e.Name] = !e.IsDir
	}
	var tmpl []byte
	contents, err := c.List(app + "/sce_sys/livearea/contents")
	if err != nil && !ftp.IsNotFound(err) {
		return nil, err
	}
	for _, e := range contents {
		names["sce_sys/livearea/contents/"+e.Name] = !e.IsDir
		if e.Name == "template.xml" && !e.IsDir {
			tmpl, _ = c.Retrieve(app+"/sce_sys/livearea/contents/template.xml", 1<<20)
		}
	}
	p := art.FindIn(func(n string) bool { return names[n] }, tmpl)
	var ts []artTarget
	add := func(rel string, role string) {
		if rel != "" && set.png[role] != nil {
			ts = append(ts, artTarget{app + "/" + rel, set.png[role]})
		}
	}
	add(p.Icon, "icon")
	add(p.Background, "background")
	add(p.Startup, "startup")
	add(p.Pic0, "pic0")
	// The system keeps its own copy of the bubble icon (and only that).
	if _, err := c.List("ur0:appmeta/" + title); err == nil && set.png["icon"] != nil {
		ts = append(ts, artTarget{"ur0:appmeta/" + title + "/icon0.png", set.png["icon"]})
	}
	if len(ts) == 0 {
		return nil, errors.New("this game has no pictures that can be replaced")
	}
	if err := backupArt(ctx, c, title, ts); err != nil {
		return nil, err
	}
	var out []string
	for _, t := range ts {
		if err := putVerified(ctx, c, t.to, t.data); err != nil {
			return nil, err
		}
		out = append(out, t.to)
	}
	return out, nil
}

// backupArt keeps the files that are about to be replaced, once per game:
// ux0:data/cboc-installer/art-backup/<TITLEID>/, with backup.json saying where
// each came from (written last, so a cut-short backup is made again).
func backupArt(ctx context.Context, c *ftp.Conn, title string, ts []artTarget) error {
	dir := artBackupDir + "/" + title
	if es, err := c.List(dir); err == nil {
		for _, e := range es {
			if e.Name == "backup.json" {
				return nil
			}
		}
	}
	if err := mkdirAll(c, dir); err != nil {
		return err
	}
	from := map[string]string{}
	for i, t := range ts {
		old, err := c.Retrieve(t.to, 32<<20)
		if err != nil {
			if ftp.IsNotFound(err) {
				continue // nothing there to keep
			}
			return err
		}
		name := fmt.Sprintf("%d-%s", i, path.Base(t.to))
		if err := c.Store(ctx, dir+"/"+name, bytes.NewReader(old), nil); err != nil {
			return err
		}
		from[name] = t.to
	}
	b, _ := json.MarshalIndent(map[string]interface{}{"title": title, "files": from, "at": time.Now().UTC()}, "", " ")
	return c.Store(ctx, dir+"/backup.json", bytes.NewReader(b), nil)
}

// putVerified writes data to dest through a temporary name, checks it and
// renames it into place. The Vita refuses to rename over a file, so the old
// one is removed just before (it has been backed up).
func putVerified(ctx context.Context, c *ftp.Conn, dest string, data []byte) error {
	tmp := dest + ".cboc-new"
	fail := func(err error) error {
		c.Delete(tmp)
		return fmt.Errorf("could not write %s on your Vita: %v", path.Base(dest), err)
	}
	if err := c.Store(ctx, tmp, bytes.NewReader(data), nil); err != nil {
		return fail(err)
	}
	if n, ok, err := c.Size(tmp); err != nil || !ok || n != int64(len(data)) {
		return fail(errors.New("the file arrived incomplete"))
	}
	if len(data) <= 1<<20 { // small files are read back and compared
		back, err := c.Retrieve(tmp, int64(len(data))+1)
		if err != nil || sha256.Sum256(back) != sha256.Sum256(data) {
			return fail(errors.New("the file did not arrive intact"))
		}
	}
	if err := c.Rename(tmp, dest); err != nil {
		if err := c.Delete(dest); err != nil && !ftp.IsNotFound(err) {
			return fail(err)
		}
		if err := c.Rename(tmp, dest); err != nil {
			// The old file is gone: put the new one straight in place.
			if err := c.Store(ctx, dest, bytes.NewReader(data), nil); err != nil {
				return fail(err)
			}
			c.Delete(tmp)
		}
	}
	if n, ok, err := c.Size(dest); err != nil || !ok || n != int64(len(data)) {
		return fmt.Errorf("could not check %s on your Vita", path.Base(dest))
	}
	return nil
}

// mkdirAll creates dir and the folders above it that are missing.
func mkdirAll(c *ftp.Conn, dir string) error {
	i := strings.IndexByte(dir, ':')
	cur := dir[:i+1]
	for _, part := range strings.Split(strings.Trim(dir[i+1:], "/"), "/") {
		if strings.HasSuffix(cur, ":") {
			cur += part
		} else {
			cur += "/" + part
		}
		if err := c.Mkdir(cur); err != nil {
			return fmt.Errorf("cannot create %s on your Vita: %v", cur, err)
		}
	}
	return nil
}

func sha256Hex(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

// artIsOurs reports whether the game's bubble on the Vita is still one the
// installer put there: the one it installed (recorded in the receipt), the
// package's own, or the one the last refresh wrote. Anything else was
// changed by hand (in VitaShell, another tool, or a custom install) and is
// never replaced automatically.
func (a *App) artIsOurs(c *ftp.Conn, r *install.Receipt, m *artMarker) bool {
	title := r.TitleID
	cur, err := c.Retrieve("ux0:app/"+title+"/sce_sys/icon0.png", 4<<20)
	if err != nil {
		return false
	}
	sum := sha256Hex(cur)
	if (m != nil && m.Icon != "" && sum == m.Icon) || (r.Icon != "" && sum == r.Icon) {
		return true
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, it := range a.st.Items {
		if it.Release != nil && it.Release.App != nil && it.Release.App.TitleID == title && len(it.Release.App.Icon) > 0 {
			if bytes.Equal(cur, it.Release.App.Icon) {
				return true
			}
		}
	}
	return false
}
