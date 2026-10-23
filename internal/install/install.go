// Package install copies a release to a Vita and installs it.
package install

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/CBOC-Vita-Lab/vita-installer/internal/device"
	"github.com/CBOC-Vita-Lab/vita-installer/internal/ftp"
	"github.com/CBOC-Vita-Lab/vita-installer/internal/prep"
	"github.com/CBOC-Vita-Lab/vita-installer/internal/release"
	"github.com/CBOC-Vita-Lab/vita-installer/internal/vita"
)

// Staging is where the installer puts files before the Vita installs them.
const Staging = "ux0:data/cboc-installer"

// Progress is reported while a job runs.
type Progress struct {
	Phase      string  `json:"phase"` // prepare, check, send, install, done
	Text       string  `json:"text"`
	Done       int64   `json:"done"`
	Total      int64   `json:"total"`
	Files      int     `json:"files"`
	FilesTotal int     `json:"filesTotal"`
	Speed      float64 `json:"speed"` // bytes per second
	ETA        int     `json:"eta"`   // seconds, -1 if unknown
	Current    string  `json:"current,omitempty"`
	Unit       string  `json:"unit,omitempty"` // for prepare: "", "files" or "frames"
}

// Options say where to install.
type Options struct {
	Vita        *device.Vita // install over Wi-Fi
	Drive       string       // or copy to a mounted ux0: drive
	Cache       string       // scratch space on this computer
	Connections int          // parallel FTP transfers (default 3)
	OnProgress  func(Progress)
}

// Result says what happened.
type Result struct {
	Installed bool     `json:"installed"` // the app was installed on the Vita
	Copied    int      `json:"copied"`
	Skipped   int      `json:"skipped"` // files already on the Vita
	Bytes     int64    `json:"bytes"`
	Seconds   float64  `json:"seconds"`
	Steps     []string `json:"steps,omitempty"` // anything the player still has to do on the Vita
	CanLaunch bool     `json:"canLaunch"`
}

type item struct {
	local, remote string
	size          int64
	onlyIfMissing bool
	app           bool
}

// Run installs r.
func Run(ctx context.Context, r *release.Release, opt Options) (*Result, error) {
	start := time.Now()
	if r.Held != "" {
		return nil, errors.New(r.Held)
	}
	for _, in := range r.Inputs {
		if !in.Satisfied() {
			return nil, fmt.Errorf("%s is still needed", in.Label)
		}
	}
	if opt.Connections <= 0 {
		opt.Connections = 3
	}
	report := opt.OnProgress
	if report == nil {
		report = func(Progress) {}
	}
	os.MkdirAll(opt.Cache, 0o755)

	// 1. Connect first, so a sleeping Vita is reported before any slow work.
	var t target
	var ft *ftpTarget
	report(Progress{Phase: "prepare", Text: "Connecting", ETA: -1})
	if opt.Vita != nil {
		var err error
		ft, err = newFTPTarget(ctx, opt.Vita.Addr(), opt.Connections)
		if err != nil {
			return nil, err
		}
		t = ft
		if opt.Vita.Companion {
			device.Companion(ctx, opt.Vita.Host, "nosleep on", 2*time.Second)
			defer device.Companion(context.Background(), opt.Vita.Host, "nosleep off", 2*time.Second)
		}
	} else if opt.Drive != "" {
		if !device.LooksLikeVita(opt.Drive) {
			return nil, errors.New("the Vita drive is no longer connected")
		}
		t = &driveTarget{root: opt.Drive}
	} else {
		return nil, errors.New("connect your Vita first")
	}
	defer t.close()

	// 2. Prepare the player's files.
	var items []item
	var cleanup []string
	for _, in := range r.Inputs {
		got, tmp, err := prepareInput(ctx, in, r.Root, opt, t, report)
		if err != nil {
			return nil, err
		}
		items = append(items, got...)
		cleanup = append(cleanup, tmp...)
	}
	for _, b := range r.Copies {
		items = append(items, item{local: b.Local, remote: b.Remote, size: b.Size})
	}
	var kept []item
	for _, it := range items {
		if !r.IsKept(it.remote) {
			kept = append(kept, it)
		}
	}
	items = kept

	// 3. The app itself.
	mode := "manual"
	var appItems []item
	appRemote := ""
	switch {
	case opt.Vita != nil && opt.Vita.Companion:
		mode = "companion"
		dir := filepath.Join(opt.Cache, "apps", r.TitleID+"-"+shortHash(r.VPK))
		if err := unpackVPK(ctx, r.VPK, dir, report); err != nil {
			return nil, err
		}
		cleanup = append(cleanup, dir)
		appRemote = Staging + "/" + r.TitleID
		filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
			if err == nil && !info.IsDir() && !strings.HasSuffix(p, ".complete") {
				rel, _ := filepath.Rel(dir, p)
				appItems = append(appItems, item{local: p, remote: appRemote + "/" + filepath.ToSlash(rel), size: info.Size(), app: true})
			}
			return nil
		})
	case opt.Vita != nil:
		mode = "prom"
		appRemote = Staging + "/" + r.TitleID + ".vpk"
		appItems = []item{{local: r.VPK, remote: appRemote, size: r.App.Size, app: true}}
	default:
		appRemote = "ux0:VPK/" + safeName(r.Name, r.TitleID) + ".vpk"
		appItems = []item{{local: r.VPK, remote: appRemote, size: r.App.Size, app: true}}
	}

	// 4. Skip what the Vita already has.
	report(Progress{Phase: "check", Text: "Checking what is already on your Vita", ETA: -1})
	all := append(appItems, items...)
	todo, skipped, err := compare(ctx, t, all, mode == "companion")
	if err != nil {
		return nil, err
	}
	if opt.Drive != "" {
		var need int64
		for _, it := range todo {
			need += it.size
		}
		if free := device.FreeSpace(opt.Drive); free >= 0 && need > free {
			return nil, fmt.Errorf("the Vita needs %s free but has %s. Free up space and try again", human(need), human(free))
		}
	}

	// 5. Send.
	res := &Result{Skipped: skipped}
	if err := send(ctx, t, todo, report); err != nil {
		return nil, err
	}
	for _, it := range todo {
		res.Copied++
		res.Bytes += it.size
	}
	// Mark prepared game data as complete, so later installs know it is there.
	for _, in := range r.Inputs {
		if len(in.Prepare) == 0 || in.Path == "" || in.State == "skip" {
			continue
		}
		mark := filepath.Join(opt.Cache, "marker-"+in.ID)
		os.WriteFile(mark, []byte(PreparedTag(in)), 0o644)
		if err := t.put(ctx, 0, mark, strings.TrimSuffix(in.To, "/")+"/"+markerName, func(int64) {}); err != nil {
			return nil, fmt.Errorf("could not finish: %v", err)
		}
		os.Remove(mark)
	}

	// 6. Install.
	report(Progress{Phase: "install", Text: "Installing " + r.Name + " on your Vita", ETA: -1})
	switch mode {
	case "companion":
		out, err := device.Companion(ctx, opt.Vita.Host, "promote "+appRemote, 15*time.Minute)
		if err != nil || !strings.HasPrefix(out, "Promoted") {
			if out == "" && err != nil {
				out = err.Error()
			}
			return nil, fmt.Errorf("the Vita could not install the app: %s", strings.TrimPrefix(out, "Error: "))
		}
		removeTree(ft, appRemote)
		res.Installed = true
		res.CanLaunch = true
	case "prom":
		var code int
		var msg string
		err := ft.control(ctx, func(c *ftp.Conn) error {
			var err error
			code, msg, err = c.CmdWait("PROM "+appRemote, 15*time.Minute)
			return err
		})
		if err == nil && code == 200 {
			t.remove(appRemote)
			res.Installed = true
		} else {
			_ = msg
			// VitaShell could not install it remotely: leave it in ux0:VPK for one button press.
			manual := "ux0:VPK/" + safeName(r.Name, r.TitleID) + ".vpk"
			ft.control(ctx, func(c *ftp.Conn) error {
				c.Mkdir("ux0:VPK")
				c.Delete(manual)
				return c.Rename(appRemote, manual)
			})
			res.Steps = append(res.Steps, "In VitaShell, open "+manual+" and press X to install it.")
		}
	default:
		res.Steps = append(res.Steps, "Eject the drive safely and leave USB mode (or put the card back in the Vita).",
			"In VitaShell, open "+appRemote+" and press X to install it.")
	}
	if res.Installed && ft != nil && !ft.exists("ux0:app/"+r.TitleID+"/eboot.bin") {
		res.Installed = false
		res.Steps = append(res.Steps, "The Vita reported success but the app was not found. Restart the Vita and try again.")
	}
	for _, in := range r.Inputs {
		if in.Type == release.NeedShader && in.State == "ok" && opt.Drive != "" && in.Path != "" {
			res.Steps = append(res.Steps, "In VitaShell, copy ux0:VPK/libshacccg.suprx to ur0:data/ (Triangle > Copy, then Paste in ur0:data).")
		}
	}
	for _, d := range cleanup {
		os.RemoveAll(d)
	}
	res.Seconds = time.Since(start).Seconds()
	report(Progress{Phase: "done", Text: "Done", Done: res.Bytes, Total: res.Bytes, Files: res.Copied, FilesTotal: res.Copied})
	return res, nil
}

func shortHash(s string) string {
	h := sha1.Sum([]byte(s))
	return hex.EncodeToString(h[:4])
}

func safeName(name, fallback string) string {
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		case r == ' ' || r == '.':
			b.WriteRune('-')
		}
	}
	s := strings.Trim(b.String(), "-")
	for strings.Contains(s, "--") {
		s = strings.ReplaceAll(s, "--", "-")
	}
	if s == "" {
		return fallback
	}
	return s
}

func unpackVPK(ctx context.Context, vpk, dir string, report func(Progress)) error {
	if _, err := os.Stat(filepath.Join(dir, ".complete")); err == nil {
		return nil
	}
	os.RemoveAll(dir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	report(Progress{Phase: "prepare", Text: "Unpacking the app", ETA: -1})
	if err := vita.ExtractZip(vpk, dir, nil); err != nil {
		os.RemoveAll(dir)
		return err
	}
	return os.WriteFile(filepath.Join(dir, ".complete"), nil, 0o644)
}

// prepareInput turns one chosen input into files to copy.
func prepareInput(ctx context.Context, in *release.Input, releaseRoot string, opt Options, t target, report func(Progress)) ([]item, []string, error) {
	if in.Path == "" || in.State == "skip" || in.Type == release.NeedConsent {
		return nil, nil, nil
	}
	switch in.Type {
	case release.NeedWiiDisc, release.NeedGCDisc:
		d, err := prep.IdentifyDisc(in.Path)
		if err != nil {
			return nil, nil, err
		}
		root := d.Root
		var tmp []string
		if !d.Folder {
			st, _ := os.Stat(in.Path)
			out := filepath.Join(opt.Cache, "discs", d.GameID+"-"+shortHash(fmt.Sprintf("%s|%d|%d", in.Path, st.Size(), st.ModTime().UnixNano())))
			if r, err := prep.FindExtractedRoot(out); err == nil {
				root = r
			} else {
				os.RemoveAll(out)
				report(Progress{Phase: "prepare", Text: "Unpacking your game disc (one time only)", ETA: -1})
				var last time.Time
				begin := time.Now()
				root, err = prep.ExtractDisc(ctx, opt.Cache, in.Path, out, func(done, total int64) {
					if time.Since(last) < 300*time.Millisecond {
						return
					}
					last = time.Now()
					p := Progress{Phase: "prepare", Text: "Unpacking your game disc (one time only)", Done: done, Total: total, ETA: -1}
					if el := time.Since(begin).Seconds(); el > 2 && done > 0 {
						p.Speed = float64(done) / el
						if total > done {
							p.ETA = int(float64(total-done) / p.Speed)
						}
					}
					report(p)
				})
				if err != nil {
					return nil, nil, err
				}
			}
			tmp = append(tmp, out)
			if err := prep.CheckDisc(&prep.Disc{GameID: d.GameID, Revision: d.Revision}, in.GameID, in.Revision); err != nil {
				return nil, nil, err
			}
		}
		if len(in.Verify) > 0 {
			report(Progress{Phase: "prepare", Text: "Checking your game files", ETA: -1})
			if err := prep.VerifyFiles(root, in.Verify); err != nil {
				return nil, nil, err
			}
		}
		var out []item
		if len(in.Prepare) > 0 {
			dir, err := runPrepare(ctx, in, releaseRoot, root, opt.Cache, report)
			if err != nil {
				return nil, nil, err
			}
			out = expandDir(dir, in.To)
		} else {
			for _, sub := range []string{"sys", "files"} {
				out = append(out, expandDir(filepath.Join(root, sub), in.To+"/"+sub)...)
			}
		}
		if len(out) == 0 {
			return nil, nil, errors.New("the game disc has no files")
		}
		return out, tmp, nil
	case release.NeedPSPISO, release.NeedFile:
		st, err := os.Stat(in.Path)
		if err != nil {
			return nil, nil, err
		}
		return []item{{local: in.Path, remote: in.To, size: st.Size()}}, nil, nil
	case release.NeedShader:
		st, err := os.Stat(in.Path)
		if err != nil {
			return nil, nil, err
		}
		if opt.Drive != "" { // a drive only exposes ux0:
			return []item{{local: in.Path, remote: "ux0:VPK/libshacccg.suprx", size: st.Size(), onlyIfMissing: true}}, nil, nil
		}
		return []item{{local: in.Path, remote: release.ShaderPath, size: st.Size(), onlyIfMissing: true}}, nil, nil
	case release.NeedFolder:
		if len(in.Prepare) > 0 {
			dir, err := runPrepare(ctx, in, releaseRoot, in.Path, opt.Cache, report)
			if err != nil {
				return nil, nil, err
			}
			return expandDir(dir, in.To), nil, nil
		}
		return expandDir(in.Path, in.To), nil, nil
	case release.NeedFiles:
		var out []item
		for _, f := range listFiles(in.Path, in.Ext) {
			st, _ := os.Stat(f)
			out = append(out, item{local: f, remote: strings.TrimSuffix(in.To, "/") + "/" + filepath.Base(f), size: st.Size(), onlyIfMissing: true})
		}
		return out, nil, nil
	}
	return nil, nil, nil
}

func expandDir(local, remote string) []item {
	var out []item
	filepath.Walk(local, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || strings.HasPrefix(info.Name(), "._") || info.Name() == ".DS_Store" {
			return nil
		}
		rel, _ := filepath.Rel(local, p)
		out = append(out, item{local: p, remote: strings.TrimSuffix(remote, "/") + "/" + filepath.ToSlash(rel), size: info.Size()})
		return nil
	})
	return out
}

// compare drops files the Vita already has with the same size and creates
// the folders the rest need. Staged app files are always re-sent unless
// resuming (same size), which is safe because they are installer-owned.
func compare(ctx context.Context, t target, items []item, _ bool) ([]item, int, error) {
	dirs := map[string]bool{}
	for _, it := range items {
		dirs[path.Dir(it.remote)] = true
	}
	list := make([]string, 0, len(dirs))
	for d := range dirs {
		list = append(list, d)
	}
	sort.Strings(list)
	type listing struct {
		files  map[string]int64
		exists bool
	}
	got := make(map[string]listing, len(list))
	var mu sync.Mutex
	var firstErr error
	var retry []string
	jobs := make(chan string)
	var wg sync.WaitGroup
	for w := 0; w < t.workers(); w++ {
		w := w
		wg.Add(1)
		go func() {
			defer wg.Done()
			for d := range jobs {
				files, ok, err := t.sizes(ctx, w, d)
				mu.Lock()
				if err != nil {
					retry = append(retry, d) // listed again below on the first connection
				} else {
					got[d] = listing{files, ok}
				}
				mu.Unlock()
			}
		}()
	}
	for _, d := range list {
		jobs <- d
	}
	close(jobs)
	wg.Wait()
	for _, d := range retry {
		files, ok, err := t.sizes(ctx, 0, d)
		if err != nil {
			firstErr = err
			break
		}
		got[d] = listing{files, ok}
	}
	if firstErr != nil {
		return nil, 0, fmt.Errorf("lost contact with the Vita: %v", firstErr)
	}
	var todo []item
	skipped := 0
	var mk []string
	for _, it := range items {
		l := got[path.Dir(it.remote)]
		size, ok := l.files[path.Base(it.remote)]
		if ok && size >= 0 && (it.onlyIfMissing || size == it.size) {
			skipped++
			continue
		}
		if ok && size < 0 {
			return nil, 0, fmt.Errorf("%s is a folder on the Vita, but a file is needed there", it.remote)
		}
		todo = append(todo, it)
		mk = append(mk, it.remote)
	}
	var create []string
	for _, d := range parentDirs(mk) {
		if l, ok := got[d]; ok && l.exists {
			continue
		}
		if isRoot(d) {
			continue
		}
		create = append(create, d)
	}
	// Skip folders whose parent listing shows they already exist.
	var final []string
	for _, d := range create {
		parent := path.Dir(d)
		if strings.HasSuffix(parent, ":") || parent == "." {
			parent = d[:strings.IndexByte(d, ':')+1]
		}
		if l, ok := got[parent]; ok {
			if _, has := l.files[path.Base(d)+"/"]; has {
				continue
			}
		}
		final = append(final, d)
	}
	if err := t.mkdirs(ctx, final); err != nil {
		return nil, 0, err
	}
	return todo, skipped, nil
}

func isRoot(d string) bool {
	i := strings.IndexByte(d, ':')
	return i == len(d)-1
}

// send copies items with several transfers at once, biggest files first.
func send(ctx context.Context, t target, items []item, report func(Progress)) error {
	if len(items) == 0 {
		return nil
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].app != items[j].app {
			return items[i].app
		}
		return items[i].size > items[j].size
	})
	var total int64
	for _, it := range items {
		total += it.size
	}
	report(Progress{Phase: "send", Text: "Sending to your Vita", Total: total, FilesTotal: len(items), ETA: -1})
	var done int64
	var files int32
	var current atomic.Value
	current.Store("")
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	queue := append([]item(nil), items...)
	var qmu sync.Mutex
	next := func() (item, bool) {
		qmu.Lock()
		defer qmu.Unlock()
		if len(queue) == 0 || ctx.Err() != nil {
			return item{}, false
		}
		it := queue[0]
		queue = queue[1:]
		return it, true
	}
	errc := make(chan error, t.workers())
	alive := int32(t.workers())
	var wg sync.WaitGroup
	for w := 0; w < t.workers(); w++ {
		w := w
		wg.Add(1)
		go func() {
			defer wg.Done()
			sent := 0
			for {
				it, ok := next()
				if !ok {
					return
				}
				current.Store(path.Base(it.remote))
				err := t.put(ctx, w, it.local, it.remote, func(n int64) { atomic.AddInt64(&done, n) })
				if err != nil {
					if ctx.Err() != nil {
						return
					}
					// An extra connection the Vita won't accept: hand the file
					// back and let the other connections carry on.
					if w > 0 && sent == 0 && atomic.AddInt32(&alive, -1) > 0 {
						qmu.Lock()
						queue = append([]item{it}, queue...)
						qmu.Unlock()
						return
					}
					errc <- fmt.Errorf("could not send %s: %v", path.Base(it.remote), err)
					cancel()
					return
				}
				sent++
				atomic.AddInt32(&files, 1)
			}
		}()
	}
	stop := make(chan struct{})
	go func() {
		tick := time.NewTicker(250 * time.Millisecond)
		defer tick.Stop()
		begin := time.Now()
		var speed float64
		lastDone, lastT := int64(0), begin
		for {
			select {
			case <-stop:
				return
			case now := <-tick.C:
				d := atomic.LoadInt64(&done)
				if dt := now.Sub(lastT).Seconds(); dt >= 1 {
					inst := float64(d-lastDone) / dt
					if speed == 0 {
						speed = inst
					} else {
						speed = speed*0.7 + inst*0.3
					}
					lastDone, lastT = d, now
				}
				p := Progress{Phase: "send", Text: "Sending to your Vita", Done: d, Total: total,
					Files: int(atomic.LoadInt32(&files)), FilesTotal: len(items), Speed: speed, ETA: -1, Current: current.Load().(string)}
				if speed > 0 && now.Sub(begin) > 2*time.Second {
					p.ETA = int(float64(total-d) / speed)
				}
				report(p)
			}
		}
	}()
	wg.Wait()
	close(stop)
	select {
	case err := <-errc:
		return err
	default:
	}
	return ctx.Err()
}

func removeTree(ft *ftpTarget, dir string) {
	if ft == nil {
		return
	}
	ft.control(context.Background(), func(c *ftp.Conn) error {
		var walk func(d string)
		walk = func(d string) {
			entries, err := c.List(d)
			if err != nil {
				return
			}
			for _, e := range entries {
				p := d + "/" + e.Name
				if e.IsDir {
					walk(p)
				} else {
					c.Delete(p)
				}
			}
			c.Rmdir(d)
		}
		walk(dir)
		return nil
	})
}

func human(n int64) string {
	switch {
	case n >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(n)/(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%.0f MB", float64(n)/(1<<20))
	default:
		return fmt.Sprintf("%d KB", n>>10)
	}
}

// Launch starts an installed app (Vita Companion only).
func Launch(ctx context.Context, v *device.Vita, titleID string) error {
	out, err := device.Companion(ctx, v.Host, "launch "+titleID, 10*time.Second)
	if err != nil {
		return err
	}
	if !strings.HasPrefix(out, "Launched") {
		return errors.New(strings.TrimPrefix(out, "Error: "))
	}
	return nil
}
