package install

import (
	"archive/zip"
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/CBOC-Vita-Lab/vita-installer/internal/release"
)

// A "file" input can be chosen as the file itself, a ZIP that holds it, a
// folder that holds it (or a ZIP of it), or the first piece of a file split
// into .part00, .part01, … (or .001, .002, …) pieces. fileSource says which.
type fileSource struct {
	path   string   // the file, the ZIP, or the first piece
	member string   // inside a ZIP: the file's name
	parts  []string // split pieces, in order
	size   int64
	mod    int64 // modification time, to prefer the newest of several
}

// what describes the source for the page.
func (s *fileSource) what() string {
	switch {
	case s.member != "":
		return "It will be taken out of " + filepath.Base(s.path) + "."
	case len(s.parts) > 1:
		return "Its " + itoa(len(s.parts)) + " pieces will be joined."
	}
	return ""
}

var rePart = regexp.MustCompile(`(?i)^(.+)\.(?:part)?(\d{2,3})$`)

// pieces returns the split pieces that start with path, or nil when path is
// not the first piece of a split file.
func pieces(path string) []string {
	m := rePart.FindStringSubmatch(filepath.Base(path))
	if m == nil {
		return nil
	}
	first, _ := strconv.Atoi(m[2])
	if first > 1 {
		return nil
	}
	dir, base, width := filepath.Dir(path), m[1], len(m[2])
	sep := strings.TrimSuffix(strings.TrimPrefix(filepath.Base(path), base), m[2])
	var out []string
	for i := first; ; i++ {
		n := strconv.Itoa(i)
		for len(n) < width {
			n = "0" + n
		}
		p := filepath.Join(dir, base+sep+n)
		if st, err := os.Stat(p); err != nil || st.IsDir() {
			break
		}
		out = append(out, p)
	}
	if len(out) < 2 {
		return nil
	}
	return out
}

// head reads the first bytes of the source.
func (s *fileSource) head(n int) []byte {
	var r io.ReadCloser
	if s.member != "" {
		z, err := zip.OpenReader(s.path)
		if err != nil {
			return nil
		}
		defer z.Close()
		for _, f := range z.File {
			if f.Name == s.member {
				if r, err = f.Open(); err != nil {
					return nil
				}
			}
		}
		if r == nil {
			return nil
		}
	} else {
		f, err := os.Open(s.path)
		if err != nil {
			return nil
		}
		r = f
	}
	defer r.Close()
	b := make([]byte, n)
	k, _ := io.ReadFull(r, b)
	return b[:k]
}

// fits reports whether the source looks like the file the input asks for:
// by its first bytes when the release says what they are, otherwise by its
// name or extension.
func fits(in *release.Input, s *fileSource, name string, byName bool) bool {
	if in.Magic != "" {
		return bytes.HasPrefix(s.head(len(in.Magic)), []byte(in.Magic))
	}
	if byName && len(in.Names) > 0 {
		for _, n := range in.Names {
			if strings.EqualFold(n, name) {
				return true
			}
		}
		return false
	}
	return extOK(name, in.Ext)
}

// findFileSource works out where the file for in really is.
func findFileSource(in *release.Input, path string) (*fileSource, error) {
	st, err := os.Stat(path)
	if err != nil {
		return nil, errors.New("this file cannot be opened")
	}
	if st.IsDir() {
		return inFolder(in, path)
	}
	if p := pieces(path); p != nil {
		s := &fileSource{path: p[0], parts: p, mod: st.ModTime().Unix()}
		for _, x := range p {
			xs, _ := os.Stat(x)
			s.size += xs.Size()
		}
		name := rePart.FindStringSubmatch(filepath.Base(path))[1]
		if fits(in, s, name, false) {
			return s, nil
		}
		return nil, errors.New("these pieces are not the file this game needs")
	}
	s := &fileSource{path: path, size: st.Size(), mod: st.ModTime().Unix()}
	if in.Magic != "" && fits(in, s, filepath.Base(path), false) {
		return s, nil // the file itself, whatever it is called
	}
	if isZip(path) && (in.Magic != "" || len(in.Ext) == 0 || !extOK(path, in.Ext)) {
		return inZip(in, path)
	}
	if in.Magic != "" {
		return nil, errors.New("this is not the file this game needs")
	}
	if !extOK(path, in.Ext) {
		return nil, errors.New("this file type is not accepted here")
	}
	return s, nil
}

func isZip(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	b := make([]byte, 4)
	io.ReadFull(f, b)
	return string(b) == "PK\x03\x04"
}

// inZip finds the file inside a ZIP.
func inZip(in *release.Input, path string) (*fileSource, error) {
	z, err := zip.OpenReader(path)
	if err != nil {
		return nil, errors.New("this ZIP cannot be read")
	}
	var names []string
	sizes := map[string]int64{}
	for _, f := range z.File {
		base := filepath.Base(f.Name)
		if f.FileInfo().IsDir() || strings.HasPrefix(base, ".") || strings.HasPrefix(f.Name, "__MACOSX/") {
			continue
		}
		if f.Method != zip.Store && f.Method != zip.Deflate {
			continue
		}
		names = append(names, f.Name)
		sizes[f.Name] = int64(f.UncompressedSize64)
	}
	st, _ := os.Stat(path)
	z.Close()
	var best *fileSource
	for _, byName := range []bool{true, false} {
		for _, n := range names {
			s := &fileSource{path: path, member: n, size: sizes[n], mod: st.ModTime().Unix()}
			if !fits(in, s, filepath.Base(n), byName) {
				continue
			}
			if best == nil || s.size > best.size {
				best = s
			}
		}
		if best != nil || in.Magic != "" {
			break
		}
	}
	if best == nil {
		return nil, errors.New("the file this game needs is not in this ZIP")
	}
	return best, nil
}

// inFolder finds the file in a folder (looking two folders deep, and in
// ZIPs and split pieces there). With several, the newest wins.
func inFolder(in *release.Input, dir string) (*fileSource, error) {
	var found []*fileSource
	var zips []string
	filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(dir, p)
		depth := strings.Count(filepath.ToSlash(rel), "/")
		if info.IsDir() {
			if p != dir && (strings.HasPrefix(info.Name(), ".") || depth >= 2) {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasPrefix(info.Name(), ".") || info.Size() == 0 {
			return nil
		}
		if m := rePart.FindStringSubmatch(info.Name()); m != nil {
			if n, _ := strconv.Atoi(m[2]); n <= 1 {
				if s, err := findFileSource(in, p); err == nil {
					found = append(found, s)
				}
			}
			return nil
		}
		s := &fileSource{path: p, size: info.Size(), mod: info.ModTime().Unix()}
		if fits(in, s, info.Name(), true) {
			found = append(found, s)
		} else if strings.EqualFold(filepath.Ext(p), ".zip") {
			zips = append(zips, p)
		}
		return nil
	})
	if len(found) == 0 {
		for _, z := range zips {
			if s, err := inZip(in, z); err == nil {
				found = append(found, s)
			}
		}
	}
	if len(found) == 0 {
		return nil, errors.New("the file this game needs is not in this folder")
	}
	// A whole file beats the same file in pieces; otherwise the newest wins.
	sort.SliceStable(found, func(i, j int) bool {
		if (len(found[i].parts) > 0) != (len(found[j].parts) > 0) {
			return len(found[i].parts) == 0
		}
		return found[i].mod > found[j].mod
	})
	return found[0], nil
}

// local returns a plain file with the source's contents, unpacking or
// joining it into cache when needed. temp is true when the caller should
// remove the file afterwards.
func (s *fileSource) local(cache string) (path string, temp bool, err error) {
	if s.member == "" && len(s.parts) == 0 {
		return s.path, false, nil
	}
	if err := os.MkdirAll(cache, 0o755); err != nil {
		return "", false, err
	}
	out, err := os.CreateTemp(cache, "input-*.bin")
	if err != nil {
		return "", false, err
	}
	fail := func(e error) (string, bool, error) {
		out.Close()
		os.Remove(out.Name())
		return "", false, e
	}
	buf := make([]byte, 1<<20)
	if s.member != "" {
		z, err := zip.OpenReader(s.path)
		if err != nil {
			return fail(err)
		}
		defer z.Close()
		var done bool
		for _, f := range z.File {
			if f.Name != s.member {
				continue
			}
			r, err := f.Open()
			if err != nil {
				return fail(err)
			}
			_, err = io.CopyBuffer(out, r, buf)
			r.Close()
			if err != nil {
				return fail(err)
			}
			done = true
			break
		}
		if !done {
			return fail(errors.New(s.member + " is no longer in " + filepath.Base(s.path)))
		}
	} else {
		for _, p := range s.parts {
			f, err := os.Open(p)
			if err != nil {
				return fail(err)
			}
			_, err = io.CopyBuffer(out, f, buf)
			f.Close()
			if err != nil {
				return fail(err)
			}
		}
	}
	if err := out.Close(); err != nil {
		os.Remove(out.Name())
		return "", false, err
	}
	return out.Name(), true, nil
}
