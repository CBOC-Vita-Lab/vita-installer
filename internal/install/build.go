package install

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/CBOC-Vita-Lab/vita-installer/internal/device"
	"github.com/CBOC-Vita-Lab/vita-installer/internal/release"
	"github.com/CBOC-Vita-Lab/vita-installer/internal/vita"
)

// freeSpace reports the free bytes on a volume (replaced in tests).
var freeSpace = device.FreeSpace

// FakeFreeSpace makes the room checks see free bytes everywhere, for tests of
// code above this package; the returned function puts things back.
func FakeFreeSpace(free int64) (restore func()) {
	old := freeSpace
	freeSpace = func(string) int64 { return free }
	return func() { freeSpace = old }
}

// buildSpace is the room a build needs on this computer while it works.
const buildSpace = 1 << 30

// buildApp makes the release's app on this computer from the player's file
// (the need named by Build.From) and returns it. The result is kept in the
// cache, keyed by the kit, the player's file and the arguments, so a retry or
// a reinstall does not build again.
func buildApp(ctx context.Context, r *release.Release, opt Options, report func(Progress)) (*vita.App, error) {
	b := r.Build
	var from *release.Input
	for _, in := range r.Inputs {
		if in.ID == b.From {
			from = in
		}
	}
	if from == nil || from.Path == "" || from.State != "ok" {
		return nil, errors.New("choose your disc first: the game is built from it")
	}
	st, err := os.Stat(from.Path)
	if err != nil {
		return nil, fmt.Errorf("your disc cannot be opened: %v", err)
	}
	kitKey, kf, ok := b.KitFor(runtime.GOOS, runtime.GOARCH)
	if !ok {
		return nil, fmt.Errorf("this game cannot be built on this kind of computer yet (%s)", kitKey)
	}

	appsDir := filepath.Join(opt.Cache, "apps")
	id := fmt.Sprintf("v1|%s|%s|%d|%d|%s|%s|%s", strings.ToLower(kf.SHA256), from.Path, st.Size(), st.ModTime().UnixNano(), b.Run, strings.Join(b.Args, "\x00"), b.App)
	sum := sha256.Sum256([]byte(id))
	key := hex.EncodeToString(sum[:8])
	cached := filepath.Join(appsDir, r.TitleID+"-build-"+key+".vpk")
	if app, err := vita.OpenVPK(cached); err == nil && (r.TitleID == "" || app.TitleID == r.TitleID) {
		now := time.Now()
		os.Chtimes(cached, now, now) // keeps it from being cleaned up while it is in use
		report(Progress{Phase: "prepare", Text: "Using the game you built earlier", ETA: -1})
		return app, nil
	}
	os.Remove(cached)

	// The kit, then room to work.
	if _, err := os.Stat(filepath.Join(opt.Cache, "kits", strings.ToLower(kf.SHA256)[:16], ".complete")); err != nil {
		report(Progress{Phase: "prepare", Text: "Getting the build tools", ETA: -1})
	}
	kit, err := ensureKit(ctx, r, kf, opt.Cache, report)
	if err != nil {
		return nil, err
	}
	work := filepath.Join(appsDir, ".build-"+key)
	os.RemoveAll(work)
	defer os.RemoveAll(work)
	out, tmp := filepath.Join(work, "out"), filepath.Join(work, "tmp")
	for _, d := range []string{out, tmp} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return nil, err
		}
	}
	if free := freeSpace(work); free >= 0 && free < buildSpace {
		return nil, fmt.Errorf("building the game needs about %s free on this computer, but only %s is free. Free up some space and try again", human(buildSpace), human(free))
	}

	// {in}: the player's file (a file inside a ZIP or in pieces is made whole first).
	input := from.Path
	if from.Type == release.NeedFile {
		src, err := findFileSource(from, from.Path)
		if err != nil {
			return nil, errors.New(from.Label + ": " + err.Error())
		}
		local, temp, err := src.local(filepath.Join(opt.Cache, "inputs"))
		if err != nil {
			return nil, err
		}
		if temp {
			defer os.Remove(local)
		}
		input = local
	}
	args := make([]string, len(b.Args))
	for i, a := range b.Args {
		a = strings.ReplaceAll(a, "{in}", input)
		a = strings.ReplaceAll(a, "{out}", out)
		args[i] = strings.ReplaceAll(a, "{work}", tmp)
	}
	if len(args) == 0 {
		args = []string{input, out}
	}

	label := b.Label
	if label == "" {
		label = "Building " + r.Name + " from your disc"
	}
	pr := &buildProgress{report: report, text: label}
	pr.step(label)

	cmd := exec.CommandContext(ctx, kitProgram(kit, b.Run), args...)
	cmd.Dir = kit
	cmd.Env = append(os.Environ(), "TMPDIR="+tmp, "TMP="+tmp, "TEMP="+tmp) // scratch files stay where the room was checked
	stderr := &tailWriter{}
	cmd.Stderr = stderr
	var donePath string
	lines := &lineWriter{fn: func(line string) {
		switch {
		case strings.HasPrefix(line, "@step "):
			pr.step(strings.TrimSpace(line[len("@step "):]))
		case strings.HasPrefix(line, "@progress "):
			f := strings.Fields(line)
			if len(f) >= 3 {
				d, e1 := strconv.ParseInt(f[1], 10, 64)
				t, e2 := strconv.ParseInt(f[2], 10, 64)
				if e1 == nil && e2 == nil && t > 0 && d >= 0 {
					if d > t {
						d = t
					}
					pr.progress(d, t)
				}
			}
		case strings.HasPrefix(line, "@done "):
			donePath = strings.TrimSpace(line[len("@done "):])
		}
	}}
	cmd.Stdout = lines
	buildProc(cmd)
	cmd.WaitDelay = 5 * time.Second // do not wait for compilers a stopped build left behind
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("the build tools could not be started: %v", err)
	}
	err = cmd.Wait()
	lines.flush()
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if err != nil {
		if msg := stderr.last(3); msg != "" {
			return nil, errors.New("the build failed: " + msg)
		}
		return nil, fmt.Errorf("the build stopped unexpectedly (%v)", err)
	}

	built := filepath.Join(out, b.App)
	if _, err := os.Stat(built); err != nil && donePath != "" && filepath.IsAbs(donePath) {
		if rel, err := filepath.Rel(out, donePath); err == nil && !strings.HasPrefix(rel, "..") {
			built = donePath
		}
	}
	app, err := vita.OpenVPK(built)
	if err != nil {
		return nil, fmt.Errorf("the build finished, but did not make a Vita app (%v)", err)
	}
	if r.TitleID != "" && app.TitleID != r.TitleID {
		return nil, fmt.Errorf("the build made %s, but this release is %s", app.TitleID, r.TitleID)
	}
	if err := os.MkdirAll(appsDir, 0o755); err != nil {
		return nil, err
	}
	if err := os.Rename(built, cached); err != nil {
		if err := copyPlain(built, cached); err != nil {
			return nil, err
		}
	}
	// Older builds of this game are not needed any more.
	if old, _ := filepath.Glob(filepath.Join(appsDir, r.TitleID+"-build-*.vpk")); len(old) > 0 {
		for _, o := range old {
			if o != cached {
				os.Remove(o)
			}
		}
	}
	return vita.OpenVPK(cached)
}

// buildProgress turns the builder's step and progress lines into Progress,
// with an estimate of the time left in the current step.
type buildProgress struct {
	mu     sync.Mutex
	report func(Progress)
	text   string
	begin  time.Time
	last   time.Time
}

func (p *buildProgress) step(text string) {
	p.mu.Lock()
	p.text, p.begin = text, time.Now()
	p.mu.Unlock()
	p.report(Progress{Phase: "prepare", Text: text, ETA: -1})
}

func (p *buildProgress) progress(done, total int64) {
	p.mu.Lock()
	if done != total && time.Since(p.last) < 250*time.Millisecond {
		p.mu.Unlock()
		return
	}
	p.last = time.Now()
	text, el := p.text, time.Since(p.begin).Seconds()
	p.mu.Unlock()
	pg := Progress{Phase: "prepare", Text: text, Done: done, Total: total, ETA: -1, Unit: "count"}
	if el > 3 && done > 0 && total > done {
		pg.ETA = int(el / float64(done) * float64(total-done))
	}
	p.report(pg)
}

// lineWriter hands whole lines (up to 64 KB each) to fn.
type lineWriter struct {
	fn  func(string)
	buf []byte
}

func (w *lineWriter) Write(b []byte) (int, error) {
	for _, c := range b {
		if c == '\n' {
			w.fn(strings.TrimRight(string(w.buf), "\r"))
			w.buf = w.buf[:0]
		} else if len(w.buf) < 64<<10 {
			w.buf = append(w.buf, c)
		}
	}
	return len(b), nil
}

func (w *lineWriter) flush() {
	if len(w.buf) > 0 {
		w.fn(strings.TrimRight(string(w.buf), "\r"))
		w.buf = w.buf[:0]
	}
}

// tailWriter keeps the last few KB of what a program wrote.
type tailWriter struct {
	mu  sync.Mutex
	buf []byte
}

func (w *tailWriter) Write(b []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.buf = append(w.buf, b...)
	if len(w.buf) > 4096 {
		w.buf = append([]byte(nil), w.buf[len(w.buf)-4096:]...)
	}
	return len(b), nil
}

// last returns the last n non-empty lines, joined by spaces.
func (w *tailWriter) last(n int) string {
	w.mu.Lock()
	defer w.mu.Unlock()
	var lines []string
	for _, l := range strings.Split(string(w.buf), "\n") {
		if l = strings.TrimSpace(l); l != "" {
			lines = append(lines, l)
		}
	}
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	s := strings.Join(lines, " ")
	if len(s) > 400 {
		s = s[len(s)-400:]
	}
	return s
}
