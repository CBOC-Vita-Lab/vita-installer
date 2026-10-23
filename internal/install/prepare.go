package install

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/CBOC-Vita-Lab/vita-installer/internal/catalog"
	"github.com/CBOC-Vita-Lab/vita-installer/internal/device"
	"github.com/CBOC-Vita-Lab/vita-installer/internal/galaxy"
	"github.com/CBOC-Vita-Lab/vita-installer/internal/release"
)

// runPrepare runs a need's preparation steps on src (never changing it) and
// returns the folder to copy to the Vita. Results are cached, so running the
// install again reuses them.
func runPrepare(ctx context.Context, in *release.Input, releaseRoot, src, cache string, report func(Progress)) (string, error) {
	spec, _ := json.Marshal(in.Prepare)
	st, err := os.Stat(src)
	if err != nil {
		return "", err
	}
	key := shortHash(fmt.Sprintf("v1|%s|%d|%s", src, st.ModTime().UnixNano(), spec))
	work := filepath.Join(cache, "prepared", in.ID+"-"+key)
	if b, err := os.ReadFile(filepath.Join(work, ".complete")); err == nil {
		if _, err := os.Stat(string(b)); err == nil {
			return string(b), nil // prepared earlier
		}
	}
	os.RemoveAll(work)
	if err := os.MkdirAll(work, 0o755); err != nil {
		return "", err
	}
	// Enough room on this computer? Conversions need about twice the game.
	size := treeSize(src)
	if free := device.FreeSpace(work); free >= 0 && free < size*5/4+(512<<20) {
		return "", fmt.Errorf("preparing this game needs about %s free on this computer, but only %s is free", human(size*5/4+(512<<20)), human(free))
	}
	begin := time.Now()
	lastText := ""
	var stepMu sync.Mutex // verification reports from several goroutines
	step := func(text string, done, total int64) {
		stepMu.Lock()
		defer stepMu.Unlock()
		if text != lastText {
			lastText, begin = text, time.Now()
		}
		p := Progress{Phase: "prepare", Text: text, Done: done, Total: total, ETA: -1}
		switch {
		case strings.HasPrefix(text, "Converting the game"):
			p.Unit = "files"
		case strings.HasPrefix(text, "Optimising movie"):
			p.Unit = "frames"
		}
		if el := time.Since(begin).Seconds(); el > 3 && done > 0 && total > done {
			p.ETA = int(el / float64(done) * float64(total-done))
		}
		report(p)
	}
	throttle := throttled(step)
	cur, owned := src, false
	own := func() error {
		if owned {
			return nil
		}
		dst := filepath.Join(work, "copy")
		step("Copying your game files", 0, size)
		var done int64
		err := filepath.Walk(src, func(p string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			rel, _ := filepath.Rel(src, p)
			t := filepath.Join(dst, rel)
			if info.IsDir() {
				return os.MkdirAll(t, 0o755)
			}
			if err := copyPlain(p, t); err != nil {
				return err
			}
			done += info.Size()
			throttle("Copying your game files", done, size)
			return nil
		})
		if err != nil {
			return err
		}
		cur, owned = dst, true
		return nil
	}
	for i, s := range in.Prepare {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		label := s.Label
		target := filepath.Join(cur, filepath.FromSlash(s.Path))
		switch s.Step {
		case "galaxy", "galaxy-cook":
			out, err := galaxy.Prepare(cur, filepath.Join(work, fmt.Sprintf("step%d", i)), func(text string, d, t int64) { throttle(text, d, t) })
			if err != nil {
				return "", err
			}
			cur, owned = out, true
		case "yaz0":
			if err := own(); err != nil {
				return "", err
			}
			target = filepath.Join(cur, filepath.FromSlash(s.Path))
			if label == "" {
				label = "Decompressing game files"
			}
			step(label, 0, 0)
			if err := eachFile(target, func(p string) error {
				b, err := os.ReadFile(p)
				if err != nil || len(b) < 4 || string(b[:4]) != "Yaz0" {
					return err
				}
				out, err := galaxy.Yaz0Decompress(b)
				if err != nil {
					return fmt.Errorf("%s: %v", filepath.Base(p), err)
				}
				return os.WriteFile(p, out, 0o644)
			}); err != nil {
				return "", err
			}
		case "thp-le", "thp":
			if err := own(); err != nil {
				return "", err
			}
			target = filepath.Join(cur, filepath.FromSlash(s.Path))
			var movies []string
			eachFile(target, func(p string) error {
				if strings.EqualFold(filepath.Ext(p), ".thp") {
					movies = append(movies, p)
				}
				return nil
			})
			for k, m := range movies {
				if s.Step == "thp-le" {
					step(fmt.Sprintf("Converting movie %d of %d", k+1, len(movies)), int64(k), int64(len(movies)))
					if err := galaxy.CookTHP(m, m); err != nil {
						return "", err
					}
					continue
				}
				q := s.Quality
				if q == 0 {
					q = galaxy.MovieQuality
				}
				text := fmt.Sprintf("Optimising movie %d of %d", k+1, len(movies))
				if _, err := galaxy.ReencodeTHP(m, m+".q", q, 0, func(d, t int) { throttle(text, int64(d), int64(t)) }); err != nil {
					return "", fmt.Errorf("%s: %v", filepath.Base(m), err)
				}
				if err := os.Rename(m+".q", m); err != nil {
					return "", err
				}
			}
		case "buckets":
			if err := own(); err != nil {
				return "", err
			}
			min, per := s.Min, s.Per
			if min == 0 {
				min = 64
			}
			if per == 0 {
				per = 32
			}
			step("Arranging files for the Vita", 0, 0)
			if err := galaxy.ApplyBuckets(cur, min, per); err != nil {
				return "", err
			}
		case "verify":
			c, err := catalog.Load(filepath.Join(releaseRoot, filepath.FromSlash(s.Catalog)))
			if err != nil {
				return "", fmt.Errorf("the release's file list cannot be read: %v", err)
			}
			if err := c.Verify(cur, s.Skip, func(d, t int64) { throttle("Checking your game files", d, t) }); err != nil {
				return "", fmt.Errorf("your game files do not match this release (%v). Use the exact version it asks for", err)
			}
		case "tool":
			out := filepath.Join(work, fmt.Sprintf("step%d", i))
			if err := runTool(ctx, s, releaseRoot, cache, cur, out, step); err != nil {
				return "", err
			}
			cur, owned = out, true
		}
	}
	if err := os.WriteFile(filepath.Join(work, ".complete"), []byte(cur), 0o644); err != nil {
		return "", err
	}
	return cur, nil
}

func throttled(fn func(string, int64, int64)) func(string, int64, int64) {
	var mu sync.Mutex
	var last time.Time
	return func(text string, d, t int64) {
		mu.Lock()
		if d != t && time.Since(last) < 250*time.Millisecond {
			mu.Unlock()
			return
		}
		last = time.Now()
		mu.Unlock()
		fn(text, d, t)
	}
}

func eachFile(dir string, fn func(string) error) error {
	return filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		return fn(p)
	})
}

func copyPlain(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.CopyBuffer(out, in, make([]byte, 1<<20)); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// runTool runs a converter shipped with the release, after checking its hash.
func runTool(ctx context.Context, s release.Step, releaseRoot, cache, in, out string, step func(string, int64, int64)) error {
	key := runtime.GOOS + "-" + runtime.GOARCH
	prog, ok := s.Program[key]
	if !ok && runtime.GOOS == "darwin" {
		prog, ok = s.Program["darwin-universal"]
		key = "darwin-universal"
	}
	if !ok {
		return fmt.Errorf("this release's converter does not support this computer (%s)", key)
	}
	src := filepath.Join(releaseRoot, filepath.FromSlash(prog))
	data, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("the release's converter is missing: %v", err)
	}
	sum := sha256.Sum256(data)
	if !strings.EqualFold(hex.EncodeToString(sum[:]), s.SHA256[key]) {
		return errors.New("the release's converter failed its integrity check")
	}
	dir := filepath.Join(cache, "tools", hex.EncodeToString(sum[:8]))
	os.MkdirAll(dir, 0o755)
	exe := filepath.Join(dir, filepath.Base(prog))
	if err := os.WriteFile(exe, data, 0o755); err != nil {
		return err
	}
	os.MkdirAll(out, 0o755)
	args := make([]string, len(s.Args))
	for i, a := range s.Args {
		a = strings.ReplaceAll(a, "{in}", in)
		args[i] = strings.ReplaceAll(a, "{out}", out)
	}
	if len(args) == 0 {
		args = []string{in, out}
	}
	label := s.Label
	if label == "" {
		label = "Converting the game"
	}
	step(label, 0, 0)
	cmd := exec.CommandContext(ctx, exe, args...)
	cmd.Dir = dir
	var stderr strings.Builder
	cmd.Stderr = &stderr
	hideConsole(cmd)
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if len(msg) > 300 {
			msg = msg[len(msg)-300:]
		}
		return fmt.Errorf("the release's converter failed: %v %s", err, msg)
	}
	return nil
}
