package vita

import (
	"archive/zip"
	"bytes"
	"compress/zlib"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// App describes the Vita application inside a VPK.
type App struct {
	Path        string `json:"-"`
	TitleID     string `json:"titleId"`
	Title       string `json:"title"`
	Version     string `json:"version"`
	Size        int64  `json:"size"`     // VPK file size
	Unpacked    int64  `json:"unpacked"` // size once installed
	Files       int    `json:"files"`
	Icon        []byte `json:"-"`
	UsesShaders bool   `json:"usesShaders"` // best guess: the app loads the Vita shader compiler
}

var titleIDPattern = regexp.MustCompile(`^[A-Z0-9]{9}$`)

// ValidTitleID reports whether id looks like a Vita title ID (for example PCSE00001).
func ValidTitleID(id string) bool { return titleIDPattern.MatchString(id) }

// OpenVPK checks that path is an installable VPK and reads its details.
func OpenVPK(path string) (*App, error) {
	st, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	z, err := zip.OpenReader(path)
	if err != nil {
		return nil, fmt.Errorf("%s is not a valid VPK (it is not a ZIP archive)", filepath.Base(path))
	}
	defer z.Close()
	app := &App{Path: path, Size: st.Size()}
	var sfo, eboot *zip.File
	for _, f := range z.File {
		name := strings.ToLower(strings.TrimPrefix(f.Name, "/"))
		if !SafeName(f.Name) {
			return nil, fmt.Errorf("%s contains an unsafe path (%q)", filepath.Base(path), f.Name)
		}
		if f.FileInfo().IsDir() {
			continue
		}
		app.Files++
		app.Unpacked += int64(f.UncompressedSize64)
		switch name {
		case "sce_sys/param.sfo":
			sfo = f
		case "eboot.bin":
			eboot = f
		case "sce_sys/icon0.png":
			if f.UncompressedSize64 < 2<<20 {
				app.Icon, _ = readZipFile(f, 2<<20)
			}
		}
	}
	if sfo == nil || eboot == nil {
		return nil, fmt.Errorf("%s is not a Vita app (no eboot.bin or sce_sys/param.sfo)", filepath.Base(path))
	}
	b, err := readZipFile(sfo, 1<<20)
	if err != nil {
		return nil, err
	}
	values, err := ParseSFO(b)
	if err != nil {
		return nil, fmt.Errorf("%s: %v", filepath.Base(path), err)
	}
	app.TitleID = strings.TrimSpace(values["TITLE_ID"])
	app.Title = cleanTitle(values["STITLE"], values["TITLE"])
	app.Version = strings.TrimLeft(strings.TrimSpace(values["APP_VER"]), "0")
	if strings.HasPrefix(app.Version, ".") {
		app.Version = "0" + app.Version
	}
	if !ValidTitleID(app.TitleID) {
		return nil, fmt.Errorf("%s has an invalid title ID %q", filepath.Base(path), app.TitleID)
	}
	if app.Title == "" {
		app.Title = app.TitleID
	}
	if eb, err := readZipFile(eboot, 64<<20); err == nil {
		app.UsesShaders = containsFold(eb, "libshacccg") || containsFold(eb, "vitagl")
	}
	return app, nil
}

func cleanTitle(short, long string) string {
	t := strings.TrimSpace(long)
	if t == "" {
		t = strings.TrimSpace(short)
	}
	return strings.Join(strings.Fields(t), " ")
}

// containsFold looks for s in an eboot, including inside the zlib-compressed
// segments that vita-make-fself writes.
func containsFold(b []byte, s string) bool {
	if strings.Contains(strings.ToLower(string(b)), s) {
		return true
	}
	tries := 0
	for i := 0; i+2 < len(b) && tries < 16; i++ {
		if b[i] != 0x78 || (b[i+1] != 0x9c && b[i+1] != 0xda && b[i+1] != 0x01 && b[i+1] != 0x5e) {
			continue
		}
		zr, err := zlib.NewReader(bytes.NewReader(b[i:]))
		if err != nil {
			continue
		}
		tries++
		out, _ := io.ReadAll(io.LimitReader(zr, 64<<20))
		zr.Close()
		if len(out) > 0 && strings.Contains(strings.ToLower(string(out)), s) {
			return true
		}
	}
	return false
}

func readZipFile(f *zip.File, limit int64) ([]byte, error) {
	r, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer r.Close()
	b, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > limit {
		return nil, errors.New("file inside archive is too large")
	}
	return b, nil
}

// SafeName rejects archive member names that could escape the destination.
func SafeName(name string) bool {
	n := strings.ReplaceAll(name, "\\", "/")
	if n == "" || strings.HasPrefix(n, "/") || strings.Contains(n, ":") || strings.ContainsRune(n, 0) {
		return false
	}
	for _, part := range strings.Split(n, "/") {
		if part == ".." {
			return false
		}
	}
	return true
}

// ExtractZip unpacks archive into dir (which must exist), calling progress
// with bytes written. Paths are checked so nothing lands outside dir.
func ExtractZip(archive, dir string, progress func(int64)) error {
	z, err := zip.OpenReader(archive)
	if err != nil {
		return err
	}
	defer z.Close()
	buf := make([]byte, 1<<20)
	for _, f := range z.File {
		if !SafeName(f.Name) {
			return fmt.Errorf("unsafe path in archive: %q", f.Name)
		}
		if f.Mode()&os.ModeSymlink != 0 {
			continue
		}
		target := filepath.Join(dir, filepath.FromSlash(strings.ReplaceAll(f.Name, "\\", "/")))
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		if err := extractOne(f, target, buf, progress); err != nil {
			return err
		}
	}
	return nil
}

func extractOne(f *zip.File, target string, buf []byte, progress func(int64)) error {
	r, err := f.Open()
	if err != nil {
		return err
	}
	defer r.Close()
	w, err := os.Create(target)
	if err != nil {
		return err
	}
	for {
		n, rerr := r.Read(buf)
		if n > 0 {
			if _, err := w.Write(buf[:n]); err != nil {
				w.Close()
				return err
			}
			if progress != nil {
				progress(int64(n))
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			w.Close()
			return rerr
		}
	}
	return w.Close()
}
