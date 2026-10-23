// Package selfupdate downloads a newer installer, checks it, and swaps it in
// place of the running one: the program file on Windows and Linux, the whole
// .app bundle on a Mac.
package selfupdate

import (
	"archive/tar"
	"archive/zip"
	"bufio"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// AssetSuffix is the end of this computer's download name in a release.
func AssetSuffix() string {
	switch runtime.GOOS {
	case "darwin":
		return "-mac.zip"
	case "windows":
		switch runtime.GOARCH {
		case "386":
			return "-windows-x86.exe"
		case "arm64":
			return "-windows-arm64.exe"
		}
		return "-windows-x64.exe"
	case "linux":
		return "-linux-" + runtime.GOARCH + ".tar.gz"
	}
	return ""
}

// Target is what gets replaced: the running program, or on a Mac its .app.
func Target() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	if p, err := filepath.EvalSymlinks(exe); err == nil {
		exe = p
	}
	if runtime.GOOS == "darwin" {
		app := filepath.Dir(filepath.Dir(filepath.Dir(exe))) // X.app/Contents/MacOS/x
		if !strings.HasSuffix(app, ".app") {
			return "", errors.New("not running from an app bundle")
		}
		return app, nil
	}
	return exe, nil
}

// Possible reports whether the installer can replace itself, and why not.
func Possible() (bool, string) {
	if AssetSuffix() == "" {
		return false, "this computer is not supported for automatic updates"
	}
	t, err := Target()
	if err != nil {
		return false, err.Error()
	}
	if strings.Contains(t, "/AppTranslocation/") {
		return false, "move the app to your Applications folder to get automatic updates"
	}
	dir := filepath.Dir(t)
	f, err := os.CreateTemp(dir, ".cboc-update-check-")
	if err != nil {
		return false, "the installer's folder cannot be written to"
	}
	f.Close()
	os.Remove(f.Name())
	return true, ""
}

// Staged is a downloaded, checked update waiting to be applied.
type Staged struct {
	Version string
	Dir     string // holds the new program (or .app)
	Program string // new program, or the .app on a Mac
}

const readyFile = ".ready"

// Stage downloads asset (checking it against the release's SHA256SUMS.txt)
// and unpacks it into cache/update/<version>. A finished stage is reused.
func Stage(ctx context.Context, cache, version, assetName, assetURL, sumsURL string, progress func(done, total int64)) (*Staged, error) {
	dir := filepath.Join(cache, "update", version)
	if st := find(dir, version); st != nil {
		return st, nil
	}
	os.RemoveAll(dir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	sums, err := fetch(ctx, sumsURL, 1<<20, nil)
	if err != nil {
		return nil, fmt.Errorf("could not get the checksums: %v", err)
	}
	want := ""
	sc := bufio.NewScanner(strings.NewReader(string(sums)))
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) == 2 && strings.TrimPrefix(f[1], "*") == assetName {
			want = strings.ToLower(f[0])
		}
	}
	if len(want) != 64 {
		return nil, errors.New("the release does not list a checksum for this download")
	}
	data, err := fetch(ctx, assetURL, 200<<20, progress)
	if err != nil {
		return nil, fmt.Errorf("download failed: %v", err)
	}
	sum := sha256.Sum256(data)
	if hex.EncodeToString(sum[:]) != want {
		return nil, errors.New("the download is damaged (checksum does not match)")
	}
	prog, err := unpack(data, dir)
	if err != nil {
		os.RemoveAll(dir)
		return nil, err
	}
	// The new program must start and report the expected version.
	if err := checkVersion(prog, version); err != nil {
		os.RemoveAll(dir)
		return nil, err
	}
	rel, _ := filepath.Rel(dir, prog)
	if err := os.WriteFile(filepath.Join(dir, readyFile), []byte(rel), 0o644); err != nil {
		return nil, err
	}
	return &Staged{Version: version, Dir: dir, Program: prog}, nil
}

// MarkFailed remembers that version could not be swapped in, so it is not
// downloaded again; Failed reports it.
func MarkFailed(cache, version string) {
	os.MkdirAll(filepath.Join(cache, "update-failed"), 0o755)
	os.WriteFile(filepath.Join(cache, "update-failed", version), nil, 0o644)
	os.RemoveAll(filepath.Join(cache, "update", version))
}

// Failed reports whether version could not be swapped in earlier.
func Failed(cache, version string) bool {
	_, err := os.Stat(filepath.Join(cache, "update-failed", version))
	return err == nil
}

// Pending returns a finished stage newer than current, if there is one.
func Pending(cache, current string, newer func(a, b string) bool) *Staged {
	entries, _ := os.ReadDir(filepath.Join(cache, "update"))
	var best *Staged
	for _, e := range entries {
		if !e.IsDir() || !newer(e.Name(), current) {
			continue
		}
		if st := find(filepath.Join(cache, "update", e.Name()), e.Name()); st != nil && (best == nil || newer(st.Version, best.Version)) {
			best = st
		}
	}
	return best
}

// Clean removes stages that are not newer than current, and anything left
// over from an earlier swap.
func Clean(cache, current string, newer func(a, b string) bool) {
	entries, _ := os.ReadDir(filepath.Join(cache, "update"))
	for _, e := range entries {
		if !newer(e.Name(), current) {
			os.RemoveAll(filepath.Join(cache, "update", e.Name()))
		}
	}
	if t, err := Target(); err == nil {
		os.RemoveAll(t + ".old")
		os.RemoveAll(filepath.Join(filepath.Dir(t), ".cboc-update"))
	}
}

func find(dir, version string) *Staged {
	b, err := os.ReadFile(filepath.Join(dir, readyFile))
	if err != nil {
		return nil
	}
	prog := filepath.Join(dir, filepath.FromSlash(strings.TrimSpace(string(b))))
	if _, err := os.Stat(prog); err != nil {
		return nil
	}
	return &Staged{Version: version, Dir: dir, Program: prog}
}

// Apply swaps the staged program in for the running one and starts it with
// args. The caller should exit straight away.
func Apply(st *Staged, args []string) error {
	t, err := Target()
	if err != nil {
		return err
	}
	old := t + ".old"
	os.RemoveAll(old)
	if runtime.GOOS == "darwin" {
		// Copy the new bundle next to the old one first (same disk), then
		// swap the two with renames.
		tmp := filepath.Join(filepath.Dir(t), ".cboc-update")
		os.RemoveAll(tmp)
		if err := copyTree(st.Program, filepath.Join(tmp, filepath.Base(t))); err != nil {
			os.RemoveAll(tmp)
			return err
		}
		if err := os.Rename(t, old); err != nil {
			os.RemoveAll(tmp)
			return err
		}
		if err := os.Rename(filepath.Join(tmp, filepath.Base(t)), t); err != nil {
			os.Rename(old, t)
			os.RemoveAll(tmp)
			return err
		}
		os.RemoveAll(tmp)
		return exec.Command("open", append([]string{"-n", t, "--args"}, args...)...).Start()
	}
	// Windows and Linux allow renaming a running program.
	newPath := t + ".new"
	if err := copyFile(st.Program, newPath, 0o755); err != nil {
		os.Remove(newPath)
		return err
	}
	if err := os.Rename(t, old); err != nil {
		os.Remove(newPath)
		return err
	}
	if err := os.Rename(newPath, t); err != nil {
		os.Rename(old, t)
		return err
	}
	cmd := exec.Command(t, args...)
	cmd.Dir = filepath.Dir(t)
	detach(cmd)
	return cmd.Start()
}

func checkVersion(prog, version string) error {
	exe := prog
	if strings.HasSuffix(prog, ".app") {
		matches, _ := filepath.Glob(filepath.Join(prog, "Contents", "MacOS", "*"))
		if len(matches) != 1 {
			return errors.New("the new app is incomplete")
		}
		exe = matches[0]
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, exe, "-version")
	detach(cmd)
	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("the new version does not start on this computer: %v", err)
	}
	if strings.TrimSpace(string(out)) != version {
		return fmt.Errorf("the new version reports %q, expected %q", strings.TrimSpace(string(out)), version)
	}
	return nil
}

var client = &http.Client{Timeout: 10 * time.Minute}

func fetch(ctx context.Context, url string, limit int64, progress func(done, total int64)) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "cboc-vita-installer")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	if resp.ContentLength > limit {
		return nil, errors.New("too big")
	}
	var buf []byte
	chunk := make([]byte, 256<<10)
	for {
		n, err := resp.Body.Read(chunk)
		buf = append(buf, chunk[:n]...)
		if int64(len(buf)) > limit {
			return nil, errors.New("too big")
		}
		if progress != nil && n > 0 {
			progress(int64(len(buf)), resp.ContentLength)
		}
		if err == io.EOF {
			return buf, nil
		}
		if err != nil {
			return nil, err
		}
	}
}

// unpack writes the download into dir and returns the program (or .app).
func unpack(data []byte, dir string) (string, error) {
	switch {
	case runtime.GOOS == "windows":
		p := filepath.Join(dir, "installer.exe")
		return p, os.WriteFile(p, data, 0o755)
	case runtime.GOOS == "linux":
		zr, err := gzip.NewReader(strings.NewReader(string(data)))
		if err != nil {
			return "", err
		}
		tr := tar.NewReader(zr)
		for {
			h, err := tr.Next()
			if err != nil {
				return "", errors.New("the download has no program in it")
			}
			if h.Typeflag == tar.TypeReg && filepath.Base(h.Name) == "cboc-vita-installer" {
				p := filepath.Join(dir, "cboc-vita-installer")
				f, err := os.OpenFile(p, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
				if err != nil {
					return "", err
				}
				_, err = io.Copy(f, io.LimitReader(tr, 200<<20))
				if cerr := f.Close(); err == nil {
					err = cerr
				}
				return p, err
			}
		}
	default: // a zipped .app
		zr, err := zip.NewReader(strings.NewReader(string(data)), int64(len(data)))
		if err != nil {
			return "", err
		}
		app := ""
		for _, f := range zr.File {
			name := filepath.FromSlash(f.Name)
			if strings.HasPrefix(f.Name, "__MACOSX/") || strings.Contains(f.Name, "..") || filepath.IsAbs(name) {
				continue
			}
			first := strings.SplitN(f.Name, "/", 2)[0]
			if !strings.HasSuffix(first, ".app") {
				continue
			}
			app = filepath.Join(dir, first)
			p := filepath.Join(dir, name)
			mode := f.Mode()
			switch {
			case mode&os.ModeSymlink != 0:
				rc, err := f.Open()
				if err != nil {
					return "", err
				}
				link, _ := io.ReadAll(io.LimitReader(rc, 4096))
				rc.Close()
				os.MkdirAll(filepath.Dir(p), 0o755)
				if err := os.Symlink(string(link), p); err != nil {
					return "", err
				}
			case f.FileInfo().IsDir():
				os.MkdirAll(p, 0o755)
			default:
				perm := mode.Perm()
				if strings.Contains(f.Name, "/Contents/MacOS/") || perm&0o100 != 0 {
					perm = 0o755
				} else {
					perm = 0o644
				}
				os.MkdirAll(filepath.Dir(p), 0o755)
				rc, err := f.Open()
				if err != nil {
					return "", err
				}
				out, err := os.OpenFile(p, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, perm)
				if err != nil {
					rc.Close()
					return "", err
				}
				_, err = io.Copy(out, rc)
				rc.Close()
				if cerr := out.Close(); err == nil {
					err = cerr
				}
				if err != nil {
					return "", err
				}
			}
		}
		if app == "" {
			return "", errors.New("the download has no app in it")
		}
		return app, nil
	}
}

func copyFile(src, dst string, perm os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

func copyTree(src, dst string) error {
	return filepath.Walk(src, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		t := filepath.Join(dst, rel)
		switch {
		case info.Mode()&os.ModeSymlink != 0:
			link, err := os.Readlink(p)
			if err != nil {
				return err
			}
			return os.Symlink(link, t)
		case info.IsDir():
			return os.MkdirAll(t, 0o755)
		default:
			return copyFile(p, t, info.Mode().Perm())
		}
	})
}
