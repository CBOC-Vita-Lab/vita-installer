// Package web runs the installer's local web interface and holds its state.
package web

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/CBOC-Vita-Lab/vita-installer/internal/device"
	"github.com/CBOC-Vita-Lab/vita-installer/internal/install"
	"github.com/CBOC-Vita-Lab/vita-installer/internal/release"
)

// Item is a release in the install list.
type Item struct {
	*release.Release
	Installed bool            `json:"installed"` // already on the Vita (this install updates it)
	Status    string          `json:"status"`    // ready, needs, queued, working, done, failed
	Error     string          `json:"error,omitempty"`
	Result    *install.Result `json:"result,omitempty"`
	Launched  bool            `json:"launched,omitempty"`
}

// Job is the running installation.
type Job struct {
	ItemID   string           `json:"itemId"`
	Name     string           `json:"name"`
	Progress install.Progress `json:"progress"`
	Index    int              `json:"index"`
	Count    int              `json:"count"`
}

// Task is a slower action such as a download, shown with progress.
type Task struct {
	Text  string `json:"text"`
	Done  int64  `json:"done"`
	Total int64  `json:"total"`
}

// State is everything the page shows.
type State struct {
	Version   string         `json:"version"`
	OS        string         `json:"os"`
	Vitas     []device.Vita  `json:"vitas"`
	Vita      *device.Vita   `json:"vita"`
	Drives    []device.Drive `json:"drives"`
	Drive     string         `json:"drive"`
	Scanning  bool           `json:"scanning"`
	Scanned   bool           `json:"scanned"`
	Items     []*Item        `json:"items"`
	Job       *Job           `json:"job"`
	Task      *Task          `json:"task"`
	Notice    string         `json:"notice"`
	NoticeBad bool           `json:"noticeBad"`
	Update    *Update        `json:"update"`
	Catalog   []CatalogEntry `json:"catalog"`
	Dialogs   bool           `json:"dialogs"`
	Seq       int64          `json:"seq"`
}

// App owns the state. All methods are safe for concurrent use.
type App struct {
	mu        sync.Mutex
	st        State
	cache     string
	config    string
	changed   chan struct{}
	subs      map[chan struct{}]bool
	cancelJob context.CancelFunc
	scanMu    sync.Mutex
	lastHost  string
}

// NewApp prepares state, loading saved settings.
func NewApp(version, cache, config string) *App {
	a := &App{cache: cache, config: config, subs: map[chan struct{}]bool{}}
	a.st.Version = version
	a.st.Dialogs = true
	a.st.Catalog = builtinCatalog()
	os.MkdirAll(cache, 0o755)
	os.MkdirAll(config, 0o755)
	var saved struct{ LastHost string }
	if b, err := os.ReadFile(filepath.Join(config, "settings.json")); err == nil {
		json.Unmarshal(b, &saved)
	}
	a.lastHost = saved.LastHost
	go cleanOld(cache, 48*time.Hour)
	return a
}

// cleanOld removes copies of dragged files, downloads, unpacked releases and
// interrupted work left from earlier sessions, so the cache does not grow.
func cleanOld(cache string, age time.Duration) {
	for _, sub := range []string{"uploads", "downloads", "releases", filepath.Join("work", "apps"), filepath.Join("work", "discs")} {
		entries, _ := os.ReadDir(filepath.Join(cache, sub))
		for _, e := range entries {
			info, err := e.Info()
			if err == nil && time.Since(info.ModTime()) > age {
				os.RemoveAll(filepath.Join(cache, sub, e.Name()))
			}
		}
	}
}

func (a *App) saveSettings() {
	b, _ := json.Marshal(struct{ LastHost string }{a.lastHost})
	os.WriteFile(filepath.Join(a.config, "settings.json"), b, 0o644)
}

// Snapshot returns the state as JSON.
func (a *App) Snapshot() []byte {
	a.mu.Lock()
	defer a.mu.Unlock()
	b, _ := json.Marshal(&a.st)
	return b
}

// update changes state under the lock and notifies the page.
func (a *App) update(fn func(s *State)) {
	a.mu.Lock()
	fn(&a.st)
	a.st.Seq++
	for c := range a.subs {
		select {
		case c <- struct{}{}:
		default:
		}
	}
	a.mu.Unlock()
}

// Subscribe returns a channel that receives a signal on each change.
func (a *App) Subscribe() (chan struct{}, func()) {
	c := make(chan struct{}, 1)
	a.mu.Lock()
	a.subs[c] = true
	a.mu.Unlock()
	return c, func() {
		a.mu.Lock()
		delete(a.subs, c)
		a.mu.Unlock()
	}
}

// Busy reports whether an install is running (the app should not quit).
func (a *App) Busy() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.st.Job != nil || a.st.Task != nil
}

func (a *App) notice(msg string, bad bool) {
	a.update(func(s *State) { s.Notice, s.NoticeBad = msg, bad })
}

// ---- Finding the Vita ----

// Watch keeps looking for the Vita until ctx ends.
func (a *App) Watch(ctx context.Context) {
	for {
		a.Scan(ctx)
		wait := 4 * time.Second
		a.mu.Lock()
		connected := a.st.Vita != nil || a.st.Drive != ""
		a.mu.Unlock()
		if connected {
			wait = 6 * time.Second
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
	}
}

// Scan refreshes drives and network Vitas once.
func (a *App) Scan(ctx context.Context) {
	if !a.scanMu.TryLock() {
		return
	}
	defer a.scanMu.Unlock()
	if a.Busy() {
		return
	}
	drives := device.Drives()
	a.mu.Lock()
	current, driveBefore := a.st.Vita, a.st.Drive
	a.mu.Unlock()
	defer func() {
		a.mu.Lock()
		changed := a.st.Drive != driveBefore
		a.mu.Unlock()
		if changed {
			a.refreshItems()
		}
	}()
	// A connected Vita only needs a quick check that it is still there.
	if current != nil {
		if alive(current.Host, current.Port) {
			a.update(func(s *State) { s.Drives = drives; a.pickDrive(s) })
			return
		}
	}
	a.update(func(s *State) { s.Scanning = true; s.Drives = drives; a.pickDrive(s) })
	var preferred []string
	if a.lastHost != "" {
		preferred = append(preferred, a.lastHost)
	}
	vitas := device.Scan(ctx, preferred)
	var changedVita *device.Vita
	a.update(func(s *State) {
		s.Scanning, s.Scanned = false, true
		s.Vitas = vitas
		if s.Vita != nil {
			still := false
			for _, v := range vitas {
				if v.Host == s.Vita.Host {
					still = true
				}
			}
			if !still {
				s.Vita = nil
			}
		}
		if s.Vita == nil && len(vitas) > 0 && s.Drive == "" {
			pick := vitas[0]
			for _, v := range vitas {
				if v.Host == a.lastHost {
					pick = v
				}
			}
			s.Vita = &pick
			changedVita = &pick
		}
	})
	if changedVita != nil {
		a.lastHost = changedVita.Host
		a.saveSettings()
		a.refreshItems()
	}
}

func (a *App) pickDrive(s *State) {
	if s.Drive != "" {
		for _, d := range s.Drives {
			if d.Path == s.Drive {
				return
			}
		}
		s.Drive = ""
	}
	if s.Vita == nil && len(s.Drives) == 1 {
		s.Drive = s.Drives[0].Path
	}
}

func alive(host string, port int) bool {
	c, err := net.DialTimeout("tcp", net.JoinHostPort(host, fmt.Sprint(port)), 2*time.Second)
	if err != nil {
		return false
	}
	c.Close()
	return true
}

// Connect selects a Vita by address (typed by the player) or a drive path.
func (a *App) Connect(ctx context.Context, host, drive string) error {
	if drive != "" {
		if !device.LooksLikeVita(drive) {
			return errors.New("that folder is not a Vita memory card (it has no app and data folders)")
		}
		a.update(func(s *State) { s.Drive, s.Vita = drive, nil })
		a.refreshItems()
		return nil
	}
	host = strings.TrimSpace(host)
	if host == "" {
		a.update(func(s *State) { s.Vita, s.Drive = nil, "" })
		return nil
	}
	port := device.FTPPort
	if h, p, err := net.SplitHostPort(host); err == nil {
		host = h
		fmt.Sscan(p, &port)
	}
	v, err := device.Probe(ctx, host, port)
	if err != nil {
		return fmt.Errorf("no Vita answered at %s. On the Vita, open VitaShell and press SELECT, then try again", host)
	}
	a.lastHost = host
	a.saveSettings()
	a.update(func(s *State) {
		s.Vita, s.Drive = v, ""
		found := false
		for _, x := range s.Vitas {
			if x.Host == v.Host {
				found = true
			}
		}
		if !found {
			s.Vitas = append(s.Vitas, *v)
		}
	})
	a.refreshItems()
	return nil
}

// refreshItems re-checks each release against the connected Vita.
func (a *App) refreshItems() {
	a.mu.Lock()
	v, drive := a.st.Vita, a.st.Drive
	items := append([]*Item(nil), a.st.Items...)
	a.mu.Unlock()
	for _, it := range items {
		p, err := install.Check(context.Background(), v, drive, it.Release)
		if err != nil {
			continue
		}
		a.update(func(s *State) {
			it.Installed = p.Installed
			for _, in := range it.Inputs {
				have := p.Inputs[in.ID]
				switch {
				case have && in.Path == "":
					// Already on the Vita: nothing to choose, nothing to send.
					in.State, in.Message = "skip", "Already on your Vita."
				case !have && in.State == "skip":
					in.State, in.Message = "", ""
				}
			}
			setStatus(it)
		})
	}
}

func setStatus(it *Item) {
	if it.Status == "working" || it.Status == "queued" {
		return
	}
	if it.Status == "done" {
		return
	}
	it.Status = "ready"
	if it.Held != "" {
		it.Status = "held"
		return
	}
	for _, in := range it.Inputs {
		if !in.Satisfied() {
			it.Status = "needs"
		}
	}
}

// ---- Adding releases ----

// Add loads a release from a local path.
func (a *App) Add(path string) error {
	a.update(func(s *State) { s.Task = &Task{Text: "Opening " + filepath.Base(path)} })
	defer a.update(func(s *State) { s.Task = nil })
	rs, err := release.Load(path, a.cache)
	if err != nil {
		return err
	}
	var added []*Item
	for _, r := range rs {
		install.AutoFind(r)
		added = append(added, &Item{Release: r})
	}
	a.update(func(s *State) {
		for _, it := range added {
			replaced := false
			for i, old := range s.Items {
				if old.TitleID == it.TitleID {
					s.Items[i] = it
					replaced = true
				}
			}
			if !replaced {
				s.Items = append(s.Items, it)
			}
			setStatus(it)
		}
		s.Notice = ""
	})
	go a.refreshItems()
	return nil
}

// Remove drops a release from the list.
func (a *App) Remove(id string) {
	a.update(func(s *State) {
		for i, it := range s.Items {
			if it.ID == id && it.Status != "working" && it.Status != "queued" {
				s.Items = append(s.Items[:i], s.Items[i+1:]...)
				return
			}
		}
	})
}

// SetInput records the file chosen for one of a release's needs.
func (a *App) SetInput(itemID, inputID, path string) error {
	var target *release.Input
	a.mu.Lock()
	for _, it := range a.st.Items {
		if it.ID == itemID {
			for _, in := range it.Inputs {
				if in.ID == inputID {
					target = in
				}
			}
		}
	}
	a.mu.Unlock()
	if target == nil {
		return errors.New("that item is no longer in the list")
	}
	probe := *target
	install.CheckInput(&probe, path)
	a.update(func(s *State) {
		*target = probe
		for _, it := range s.Items {
			if it.ID == itemID {
				if it.Status == "done" || it.Status == "failed" {
					it.Status = ""
				}
				setStatus(it)
			}
		}
	})
	return nil
}

// ---- Installing ----

// Install runs every ready release in order.
func (a *App) Install(onlyID string) error {
	a.mu.Lock()
	if a.st.Job != nil {
		a.mu.Unlock()
		return errors.New("an installation is already running")
	}
	v, drive := a.st.Vita, a.st.Drive
	var queue []*Item
	for _, it := range a.st.Items {
		if (onlyID == "" || it.ID == onlyID) && (it.Status == "ready" || it.Status == "failed" || (onlyID != "" && it.Status == "done")) {
			queue = append(queue, it)
		}
	}
	a.mu.Unlock()
	if v == nil && drive == "" {
		return errors.New("connect your Vita first")
	}
	if len(queue) == 0 {
		return errors.New("nothing is ready to install yet")
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.cancelJob = cancel
	a.update(func(s *State) {
		for _, it := range queue {
			it.Status, it.Error, it.Result = "queued", "", nil
		}
		s.Job = &Job{Count: len(queue)}
		s.Notice = ""
	})
	go func() {
		defer cancel()
		for i, it := range queue {
			it := it
			a.update(func(s *State) {
				it.Status = "working"
				s.Job = &Job{ItemID: it.ID, Name: it.Name, Index: i, Count: len(queue), Progress: install.Progress{Phase: "prepare", Text: "Starting", ETA: -1}}
			})
			var last time.Time
			res, err := install.Run(ctx, it.Release, install.Options{
				Vita: v, Drive: drive, Cache: filepath.Join(a.cache, "work"),
				OnProgress: func(p install.Progress) {
					if p.Phase == "send" && time.Since(last) < 200*time.Millisecond {
						return
					}
					last = time.Now()
					a.update(func(s *State) {
						if s.Job != nil {
							s.Job.Progress = p
						}
					})
				},
			})
			a.update(func(s *State) {
				if err != nil {
					it.Status = "failed"
					if ctx.Err() != nil {
						it.Error = "Stopped. Press Install to continue where it left off."
					} else {
						it.Error = friendly(err)
					}
				} else {
					it.Status, it.Result = "done", res
					it.Installed = res.Installed || it.Installed
				}
			})
			if ctx.Err() != nil {
				for _, rest := range queue[i+1:] {
					a.update(func(s *State) { rest.Status = ""; setStatus(rest) })
				}
				break
			}
		}
		a.update(func(s *State) { s.Job = nil })
		a.refreshItems()
	}()
	return nil
}

// Cancel stops the running installation.
func (a *App) Cancel() {
	if a.cancelJob != nil {
		a.cancelJob()
	}
}

// Launch starts an installed app on the Vita.
func (a *App) Launch(id string) error {
	a.mu.Lock()
	v := a.st.Vita
	var title string
	for _, it := range a.st.Items {
		if it.ID == id {
			title = it.TitleID
		}
	}
	a.mu.Unlock()
	if v == nil || !v.Companion {
		return errors.New("launching needs Vita Companion on the Vita")
	}
	if err := install.Launch(context.Background(), v, title); err != nil {
		return fmt.Errorf("the Vita could not start the app: %v", err)
	}
	a.update(func(s *State) {
		for _, it := range s.Items {
			if it.ID == id {
				it.Launched = true
			}
		}
	})
	return nil
}

func friendly(err error) string {
	msg := err.Error()
	low := strings.ToLower(msg)
	switch {
	case strings.Contains(low, "connection refused"), strings.Contains(low, "i/o timeout"), strings.Contains(low, "connection reset"),
		strings.Contains(low, "broken pipe"), strings.Contains(low, "no route"), strings.Contains(low, "eof"):
		return "Lost the connection to the Vita. Keep it awake with VitaShell open, then press Install again. Finished files are not sent twice."
	}
	if msg != "" {
		msg = strings.ToUpper(msg[:1]) + msg[1:]
		if !strings.HasSuffix(msg, ".") {
			msg += "."
		}
	}
	return msg
}
