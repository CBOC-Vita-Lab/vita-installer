package install

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/CBOC-Vita-Lab/vita-installer/internal/release"
	"github.com/CBOC-Vita-Lab/vita-installer/internal/vita"
)

// kitDownloadURL is where a release's kit archive comes from: a file of the
// same GitHub release. Tests replace it.
var kitDownloadURL = func(repo, tag, file string) string {
	return "https://github.com/" + repo + "/releases/download/" + url.PathEscape(tag) + "/" + url.PathEscape(file)
}

var kitClient = &http.Client{Transport: &http.Transport{
	Proxy:                 http.ProxyFromEnvironment,
	DialContext:           (&net.Dialer{Timeout: 15 * time.Second}).DialContext,
	TLSHandshakeTimeout:   15 * time.Second,
	ResponseHeaderTimeout: 30 * time.Second,
}}

// ensureKit returns the folder holding the unpacked kit for this computer.
// The archive is looked for next to the release first (an offline copy), then
// downloaded from the release's GitHub page, checked against its SHA-256 and
// unpacked once; later builds reuse the folder.
func ensureKit(ctx context.Context, r *release.Release, kf release.KitFile, cache string, report func(Progress)) (string, error) {
	want := strings.ToLower(kf.SHA256)
	dir := filepath.Join(cache, "kits", want[:16])
	if _, err := os.Stat(filepath.Join(dir, ".complete")); err == nil {
		if _, err := os.Stat(kitProgram(dir, r.Build.Run)); err == nil {
			return dir, nil
		}
	}
	os.RemoveAll(dir)
	if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
		return "", err
	}

	// Room for the archive and what it unpacks to (kits are mostly compressed
	// programs, so allow about three times their size).
	if free := freeSpace(filepath.Dir(dir)); free >= 0 && kf.Size > 0 && free < kf.Size*4+(512<<20) {
		return "", fmt.Errorf("the build tools need about %s free on this computer, but only %s is free. Free up some space and try again", human(kf.Size*4+(512<<20)), human(free))
	}

	archive, temp := filepath.Join(r.Root, kf.File), false
	if st, err := os.Stat(archive); err != nil || st.IsDir() {
		if r.Repo == "" || r.Tag == "" {
			return "", fmt.Errorf("the build tools (%s) are not in this download. Download the complete release from GitHub, or add the game by its GitHub link", kf.File)
		}
		archive, temp = dir+".zip", true
		if err := downloadKit(ctx, kitDownloadURL(r.Repo, r.Tag, kf.File), archive, want, kf.Size, report); err != nil {
			return "", err
		}
	} else {
		report(Progress{Phase: "prepare", Text: "Checking the build tools", ETA: -1})
		sum, err := hashFile(ctx, archive)
		if err != nil {
			return "", err
		}
		if sum != want {
			return "", errors.New("the build tools in this release failed their integrity check. Download the release again")
		}
	}
	if temp {
		defer os.Remove(archive)
	}

	part := dir + ".partial"
	os.RemoveAll(part)
	var last time.Time
	err := unpackKit(ctx, archive, part, func(done, total int64) {
		if done != total && time.Since(last) < 250*time.Millisecond {
			return
		}
		last = time.Now()
		report(Progress{Phase: "prepare", Text: "Unpacking the build tools", Done: done, Total: total, ETA: -1})
	})
	if err == nil {
		if _, serr := os.Stat(kitProgram(part, r.Build.Run)); serr != nil {
			err = errors.New("the build tools are incomplete: the program to run is missing")
		}
	}
	if err == nil {
		err = os.WriteFile(filepath.Join(part, ".complete"), nil, 0o644)
	}
	if err == nil {
		err = os.Rename(part, dir)
	}
	if err != nil {
		os.RemoveAll(part)
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return "", fmt.Errorf("could not unpack the build tools: %v", err)
	}
	return dir, nil
}

// kitProgram is the path of the program to run inside an unpacked kit.
func kitProgram(dir, run string) string {
	p := filepath.Join(dir, filepath.FromSlash(run))
	if runtime.GOOS == "windows" && !strings.HasSuffix(strings.ToLower(p), ".exe") {
		p += ".exe"
	}
	return p
}

func hashFile(ctx context.Context, path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	buf := make([]byte, 1<<20)
	for {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		n, err := f.Read(buf)
		h.Write(buf[:n])
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// downloadKit saves url to dest, checking its SHA-256 on the way. A download
// that goes quiet for a minute is given up on.
func downloadKit(ctx context.Context, u, dest, want string, size int64, report func(Progress)) error {
	const text = "Downloading the build tools"
	report(Progress{Phase: "prepare", Text: text, Total: size, ETA: -1})
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	var stalled bool
	var mu sync.Mutex
	watch := time.AfterFunc(60*time.Second, func() { mu.Lock(); stalled = true; mu.Unlock(); cancel() })
	defer watch.Stop()
	fail := func(err error) error {
		mu.Lock()
		defer mu.Unlock()
		switch {
		case stalled:
			return errors.New("the download of the build tools stopped (nothing arrived for a minute). Check your internet connection and try again")
		case ctx.Err() != nil:
			return ctx.Err()
		}
		return fmt.Errorf("could not download the build tools: %v. Check your internet connection and try again", err)
	}

	req, _ := http.NewRequestWithContext(ctx, "GET", u, nil)
	req.Header.Set("User-Agent", "cboc-vita-installer")
	resp, err := kitClient.Do(req)
	if err != nil {
		return fail(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		if resp.StatusCode == 404 {
			return errors.New("the build tools were not found on GitHub (this release may be incomplete)")
		}
		return fmt.Errorf("could not download the build tools (HTTP %d). Try again in a moment", resp.StatusCode)
	}
	total := resp.ContentLength
	if total <= 0 {
		total = size
	}
	tmp := dest + ".part"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	defer os.Remove(tmp)
	h := sha256.New()
	buf := make([]byte, 1<<20)
	var done int64
	begin, last := time.Now(), time.Time{}
	for {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			watch.Reset(60 * time.Second)
			h.Write(buf[:n])
			if _, err := f.Write(buf[:n]); err != nil {
				f.Close()
				return err
			}
			done += int64(n)
			if time.Since(last) > 250*time.Millisecond {
				last = time.Now()
				p := Progress{Phase: "prepare", Text: text, Done: done, Total: total, ETA: -1}
				if el := time.Since(begin).Seconds(); el > 1 {
					p.Speed = float64(done) / el
					if el > 2 && total > done {
						p.ETA = int(float64(total-done) / p.Speed)
					}
				}
				report(p)
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			f.Close()
			return fail(rerr)
		}
	}
	if err := f.Close(); err != nil {
		return err
	}
	if hex.EncodeToString(h.Sum(nil)) != want {
		return errors.New("the build tools were damaged on the way (checksum mismatch). Try again")
	}
	os.Remove(dest)
	return os.Rename(tmp, dest)
}

// unpackKit extracts a kit archive into dir. Paths are checked so nothing
// lands outside dir, and the programs inside get their executable bit back
// from the archive (ZIP files keep Unix permissions).
func unpackKit(ctx context.Context, archive, dir string, progress func(done, total int64)) error {
	z, err := zip.OpenReader(archive)
	if err != nil {
		return errors.New("the build tools are not a valid ZIP file")
	}
	defer z.Close()
	var total int64
	for _, f := range z.File {
		total += int64(f.UncompressedSize64)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	root := filepath.Clean(dir) + string(filepath.Separator)
	inside := func(p string) bool { return strings.HasPrefix(filepath.Clean(p)+string(filepath.Separator), root) }
	buf := make([]byte, 1<<20)
	var done int64
	for _, f := range z.File {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if !vita.SafeName(f.Name) {
			return fmt.Errorf("unsafe path in archive: %q", f.Name)
		}
		target := filepath.Join(dir, filepath.FromSlash(strings.ReplaceAll(f.Name, "\\", "/")))
		if !inside(target) {
			return fmt.Errorf("unsafe path in archive: %q", f.Name)
		}
		mode := f.Mode()
		switch {
		case f.FileInfo().IsDir():
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case mode&os.ModeSymlink != 0:
			if runtime.GOOS == "windows" {
				continue
			}
			rc, err := f.Open()
			if err != nil {
				return err
			}
			link, err := io.ReadAll(io.LimitReader(rc, 4096))
			rc.Close()
			if err != nil {
				return err
			}
			dest := string(link)
			if dest == "" || filepath.IsAbs(dest) || !inside(filepath.Join(filepath.Dir(target), dest)) {
				return fmt.Errorf("unsafe link in archive: %q", f.Name)
			}
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			if err := os.Symlink(dest, target); err != nil {
				return err
			}
		default:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			perm := os.FileMode(0o644)
			if mode&0o111 != 0 {
				perm = 0o755
			}
			if err := extractFile(ctx, f, target, perm, buf, func(n int64) {
				done += n
				progress(done, total)
			}); err != nil {
				return err
			}
		}
	}
	progress(total, total)
	return nil
}

func extractFile(ctx context.Context, f *zip.File, target string, perm os.FileMode, buf []byte, add func(int64)) error {
	r, err := f.Open()
	if err != nil {
		return err
	}
	defer r.Close()
	w, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	for {
		if ctx.Err() != nil {
			w.Close()
			return ctx.Err()
		}
		n, rerr := r.Read(buf)
		if n > 0 {
			if _, err := w.Write(buf[:n]); err != nil {
				w.Close()
				return err
			}
			add(int64(n))
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			w.Close()
			return rerr
		}
	}
	if err := w.Close(); err != nil {
		return err
	}
	return os.Chmod(target, perm) // the umask may have cleared the executable bit
}
