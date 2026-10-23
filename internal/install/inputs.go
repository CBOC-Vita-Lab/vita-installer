package install

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/CBOC-Vita-Lab/vita-installer/internal/prep"
	"github.com/CBOC-Vita-Lab/vita-installer/internal/release"
)

// CheckInput validates the file or folder chosen for in and records the result.
func CheckInput(in *release.Input, path string) {
	in.Path, in.State, in.Message, in.Bytes = path, "", "", 0
	if path == "" {
		return
	}
	st, err := os.Stat(path)
	if err != nil {
		in.State, in.Message = "error", "This file cannot be opened."
		return
	}
	fail := func(msg string) { in.State, in.Message = "error", msg }
	switch in.Type {
	case release.NeedWiiDisc, release.NeedGCDisc:
		d, err := prep.IdentifyDisc(path)
		if err != nil {
			fail(capital(err.Error()) + ".")
			return
		}
		if err := prep.CheckDisc(d, in.GameID, in.Revision); err != nil {
			fail(capital(err.Error()) + ".")
			return
		}
		if d.Folder && len(in.Verify) > 0 {
			if err := prep.VerifyFiles(d.Root, in.Verify); err != nil {
				fail(capital(err.Error()) + ".")
				return
			}
		}
		in.State = "ok"
		if d.Folder {
			in.Bytes = treeSize(filepath.Join(d.Root, "sys")) + treeSize(filepath.Join(d.Root, "files"))
		} else {
			in.Bytes = st.Size()
		}
		if d.Folder {
			in.Message = "Extracted game " + d.GameID + " found."
		} else {
			in.Message = "Game " + d.GameID + " found. It will be unpacked automatically."
		}
	case release.NeedPSPISO:
		if st.IsDir() {
			fail("Choose the .iso file, not a folder.")
			return
		}
		id, err := prep.PSPGameID(path)
		if err != nil {
			fail(capital(err.Error()) + ".")
			return
		}
		if in.GameID != "" && !strings.EqualFold(strings.ReplaceAll(id, "-", ""), strings.ReplaceAll(in.GameID, "-", "")) {
			fail("This is " + id + ", but this release needs " + in.GameID + ".")
			return
		}
		in.State, in.Message, in.Bytes = "ok", "Game "+id+" found.", st.Size()
	case release.NeedShader:
		if st.IsDir() || !strings.EqualFold(filepath.Base(path), "libshacccg.suprx") || st.Size() < 100<<10 {
			fail("Choose the file named libshacccg.suprx.")
			return
		}
		in.State, in.Message, in.Bytes = "ok", "Shader compiler found.", st.Size()
	case release.NeedFile:
		if st.IsDir() {
			fail("Choose a file, not a folder.")
			return
		}
		if !extOK(path, in.Ext) {
			fail("This file type is not accepted here.")
			return
		}
		in.State, in.Bytes = "ok", st.Size()
	case release.NeedFolder:
		if !st.IsDir() {
			fail("Choose a folder.")
			return
		}
		in.State, in.Bytes = "ok", treeSize(path)
	case release.NeedFiles:
		files := listFiles(path, in.Ext)
		n := len(files)
		for _, f := range files {
			in.Bytes += treeSize(f)
		}
		if n == 0 {
			fail("No matching files were found.")
			return
		}
		in.State = "ok"
		if n == 1 {
			in.Message = "1 file will be copied."
		} else {
			in.Message = itoa(n) + " files will be copied."
		}
	}
}

func capital(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

func extOK(path string, exts []string) bool {
	if len(exts) == 0 {
		return true
	}
	e := strings.ToLower(filepath.Ext(path))
	for _, x := range exts {
		if strings.ToLower(x) == e {
			return true
		}
	}
	return false
}

// listFiles returns path itself, or matching files directly inside the folder.
func listFiles(path string, exts []string) []string {
	st, err := os.Stat(path)
	if err != nil {
		return nil
	}
	if !st.IsDir() {
		if extOK(path, exts) {
			return []string{path}
		}
		return nil
	}
	entries, _ := os.ReadDir(path)
	var out []string
	for _, e := range entries {
		if !e.IsDir() && !strings.HasPrefix(e.Name(), ".") && extOK(e.Name(), exts) {
			out = append(out, filepath.Join(path, e.Name()))
		}
	}
	return out
}

// AutoFind fills unset inputs with matching files found next to the release
// (the release folder, its parent, and their direct subfolders).
func AutoFind(r *release.Release, extra ...string) {
	dirs := append([]string{r.Root, filepath.Dir(r.Root)}, extra...)
	var candidates []string
	seen := map[string]bool{}
	for _, d := range dirs {
		if d == "" || seen[d] {
			continue
		}
		seen[d] = true
		candidates = append(candidates, d)
		entries, _ := os.ReadDir(d)
		for _, e := range entries {
			if strings.HasPrefix(e.Name(), ".") {
				continue
			}
			p := filepath.Join(d, e.Name())
			candidates = append(candidates, p)
			if e.IsDir() && len(candidates) < 400 {
				sub, _ := os.ReadDir(p)
				for _, s := range sub {
					if !strings.HasPrefix(s.Name(), ".") {
						candidates = append(candidates, filepath.Join(p, s.Name()))
					}
				}
			}
		}
	}
	for _, in := range r.Inputs {
		if in.Path != "" || in.State == "skip" {
			continue
		}
		for _, c := range candidates {
			if !plausible(in, c) {
				continue
			}
			probe := *in
			CheckInput(&probe, c)
			if probe.State == "ok" {
				*in = probe
				break
			}
		}
	}
}

func plausible(in *release.Input, path string) bool {
	st, err := os.Stat(path)
	if err != nil {
		return false
	}
	name := strings.ToLower(filepath.Base(path))
	switch in.Type {
	case release.NeedWiiDisc, release.NeedGCDisc:
		if st.IsDir() {
			_, err := prep.FindExtractedRoot(path)
			return err == nil
		}
		return prep.IsDiscImage(name)
	case release.NeedPSPISO:
		return !st.IsDir() && strings.HasSuffix(name, ".iso")
	case release.NeedShader:
		return name == "libshacccg.suprx"
	case release.NeedFile:
		return !st.IsDir() && name == strings.ToLower(filepath.Base(in.To))
	}
	return false
}

func treeSize(p string) int64 {
	var n int64
	filepath.Walk(p, func(_ string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			n += info.Size()
		}
		return nil
	})
	return n
}
