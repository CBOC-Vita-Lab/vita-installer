package galaxy

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
)

var (
	textExt   = map[string]bool{".txt": true, ".ini": true, ".csv": true, ".csv#": true, ".xnim": true, ".dtp": true, ".dck": true}
	unusedExt = map[string]bool{".dvmat": true, ".dvw": true, ".prj": true, ".backup": true, ".bgrv": true, ".dat": true, ".gst": true, ".bin": true}
	bcsvExt   = map[string]bool{".bcsv": true, ".pa": true, ".bcam": true, ".banmt": true, ".tbl": true}
)

// splitExt mirrors Python's os.path.splitext on a file name.
func splitExt(name string) string {
	i := strings.LastIndexByte(name, '.')
	if i <= 0 {
		return ""
	}
	if strings.Trim(name[:i], ".") == "" {
		return ""
	}
	return name[i:]
}

func asciiLower(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			b[i] = c + 32
		}
	}
	return string(b)
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// cookBlob converts one file's bytes in place and reports whether it was a
// format that needed converting.
func cookBlob(path string, b []byte) (converted bool) {
	name := path
	if i := strings.LastIndexByte(path, '/'); i >= 0 {
		name = path[i+1:]
	}
	name = asciiLower(name)
	ext := splitExt(name)
	magic := tag(b, 0, 4)
	switch {
	case magic == "J3D2":
		swapModel(b)
	case magic == "J3D1":
		swapAnim(b)
	case ext == ".kcl" && looksLikeKCL(b):
		swapKCL(b)
	case ext == ".bas":
		swapSoundAnim(b, 0)
	case magic == "RLYT":
		swapRLYT(b)
	case magic == "RLAN":
		swapRLAN(b)
	case magic == "RFNT":
		swapRFNT(b)
	case magic == "MESG":
		swapBMG(b)
	case ext == ".tpl" && u32(b, 0) == tplVersion:
		swapTPL(b)
	case ext == ".bti":
		swapTIMG(b, 0)
	case magic == "ANDO":
		sw32(b, 8, floorDiv(len(b)-8, 4))
	case magic == "JPAC":
		swapJPC(b)
	case magic == "AA_<":
		swapBAA(b)
	case ext == ".cit":
		swapCIT(b)
	case ext == ".bmt" && magic != "J3D2":
		swapMETable(b)
	case ext == ".bme":
		swapMESeq(b)
	case ext == ".brs":
		swapWords(b)
	case ext == ".bct":
		swapSpkTable(b)
	case ext == ".csw":
		swapSpkWave(b)
	case ext == ".ast" && magic == "STRM":
		swapAST(b)
	case (bcsvExt[ext] || ext == "" || isDigits(strings.TrimPrefix(ext, "."))) && looksLikeBCSV(b):
		swapBCSV(b)
	default:
		return false // text, unused or unknown: kept as is
	}
	return true
}

func cookRARC(data []byte, replace map[string][]byte) []byte {
	var cook cookFileFunc
	cook = func(path string, blob []byte, flags int) ([]byte, *replacement) {
		if r, ok := replace[path]; ok {
			b := append([]byte(nil), r...)
			cookBlob(path, b)
			return nil, &replacement{b, flags &^ (flagCompressed | flagYaz0)}
		}
		if flags&flagCompressed == 0 {
			cookBlob(path, blob)
			return blob, nil
		}
		if flags&flagYaz0 == 0 || tag(blob, 0, 4) != "Yaz0" {
			return nil, nil
		}
		inner, err := Yaz0Decompress(blob)
		if err != nil {
			panic(err)
		}
		plain := flags &^ (flagCompressed | flagYaz0)
		if tag(inner, 0, 4) == "RARC" {
			return nil, &replacement{cookRARC(inner, nil), plain}
		}
		if cookBlob(path, inner) {
			return nil, &replacement{inner, plain}
		}
		return nil, nil
	}
	return swapRarc(data, cook)
}

func cookArchive(src, rel string) ([]byte, error) {
	raw, err := os.ReadFile(src)
	if err != nil {
		return nil, err
	}
	data, err := Yaz0Decompress(raw)
	if err != nil {
		return nil, err
	}
	var out []byte
	err = guard(func() {
		if tag(data, 0, 4) == "AA_<" {
			cookBlob(filepath.ToSlash(src), data)
			out = data
			return
		}
		if tag(data, 0, 4) != "RARC" {
			out = data // copied decompressed
			return
		}
		var replace map[string][]byte
		for arc, model := range fromModel {
			if strings.HasSuffix("/"+rel, "/"+arc) {
				files := map[string][]byte{}
				for _, e := range rarcEntries(data) {
					if e.flags&flagCompressed == 0 {
						files[e.path] = append([]byte(nil), data[e.off:e.off+e.size]...)
					}
				}
				replace = rebuildCollision(files, model)
			}
		}
		out = cookRARC(data, replace)
	})
	return out, err
}

// guard runs fn, turning a panic (a read outside a file, a bad structure)
// into an error, like the reference cooker's per-file exception handling.
func guard(fn func()) (err error) {
	defer func() {
		if r := recover(); r != nil {
			if e, ok := r.(error); ok {
				err = e
			} else {
				err = fmt.Errorf("%v", r)
			}
		}
	}()
	fn()
	return nil
}

// Options control a cook.
type Options struct {
	Movies   bool // include files/MovieData (the THP movies)
	Workers  int
	Progress func(done, total int)
}

type job struct {
	src, dst, rel, mode string
	size                int64
}

// Cook converts an extracted Super Mario Galaxy disc (a folder holding
// sys/ and files/, or its DATA subfolder) into out, in the Vita's bucket
// layout.
func Cook(src, out string, opt Options) error {
	if _, err := os.Stat(filepath.Join(src, "sys", "fst.bin")); err != nil {
		inner := filepath.Join(src, "DATA")
		if _, err := os.Stat(filepath.Join(inner, "sys", "fst.bin")); err != nil {
			return errors.New("the game folder has no sys/fst.bin")
		}
		src = inner
	}
	if err := os.MkdirAll(filepath.Join(out, "sys"), 0o755); err != nil {
		return err
	}
	if err := copyFile(filepath.Join(src, "sys", "fst.bin"), filepath.Join(out, "sys", "fst.bin")); err != nil {
		return err
	}
	if err := cookDOL(src, out); err != nil {
		return err
	}
	var jobs []job
	files := filepath.Join(src, "files")
	err := filepath.Walk(files, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(files, p)
		rel = filepath.ToSlash(rel)
		if strings.SplitN(rel, "/", 2)[0] == "MovieData" && !opt.Movies {
			return nil
		}
		mode := "copy"
		switch asciiLower(filepath.Ext(p)) {
		case ".arc", ".szs":
			mode = "archive"
		case ".bcsv", ".tbl", ".ast":
			mode = "file"
		case ".thp":
			mode = "thp"
		}
		jobs = append(jobs, job{p, filepath.Join(out, "files", filepath.FromSlash(rel)), rel, mode, info.Size()})
		return nil
	})
	if err != nil {
		return err
	}
	// Biggest first keeps every worker busy to the end.
	sort.SliceStable(jobs, func(i, j int) bool { return jobs[i].size > jobs[j].size })
	workers := opt.Workers
	if workers <= 0 {
		workers = runtime.NumCPU()
	}
	var done int32
	var mu sync.Mutex
	var errs []string
	ch := make(chan job)
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range ch {
				if err := process(j); err != nil {
					mu.Lock()
					errs = append(errs, j.rel+": "+err.Error())
					mu.Unlock()
				}
				d := atomic.AddInt32(&done, 1)
				if opt.Progress != nil {
					opt.Progress(int(d), len(jobs))
				}
			}
		}()
	}
	for _, j := range jobs {
		ch <- j
	}
	close(ch)
	wg.Wait()
	if len(errs) > 0 {
		sort.Strings(errs)
		return fmt.Errorf("%d game files could not be converted (first: %s)", len(errs), errs[0])
	}
	return ApplyBuckets(out, 64, 32)
}

func process(j job) error {
	if err := os.MkdirAll(filepath.Dir(j.dst), 0o755); err != nil {
		return err
	}
	switch j.mode {
	case "archive":
		data, err := cookArchive(j.src, j.rel)
		if err != nil {
			return err
		}
		return writeFile(j.dst, data)
	case "file":
		data, err := os.ReadFile(j.src)
		if err != nil {
			return err
		}
		if err := guard(func() { cookBlob(filepath.ToSlash(j.src), data) }); err != nil {
			return err
		}
		return writeFile(j.dst, data)
	case "thp":
		return cookTHP(j.src, j.dst)
	default:
		tmp := j.dst + ".tmp"
		if err := copyFile(j.src, tmp); err != nil {
			return err
		}
		return os.Rename(tmp, j.dst)
	}
}

// cookDOL writes the data main.dol holds into out/sys.
func cookDOL(src, out string) error {
	dol, err := os.ReadFile(filepath.Join(src, "sys", "main.dol"))
	if err != nil {
		return errors.New("sys/main.dol is missing: extract the whole disc (its sys/ and files/ folders)")
	}
	results := map[string][]byte{}
	err = guard(func() {
		if arc := findErrorArchive(dol); arc != nil {
			results["ErrorMessageArchive.arc"] = cookRARC(arc, nil)
		}
		for _, t := range dolTables {
			if tb := findTable(dol, t); tb != nil {
				swapBCSV(tb)
				results[t.name+".bcsv"] = tb
			}
		}
	})
	if err != nil {
		return err
	}
	for name, data := range results {
		if err := writeFile(filepath.Join(out, "sys", name), data); err != nil {
			return err
		}
	}
	if len(results) != 1+len(dolTables) {
		return errors.New("this main.dol is not Super Mario Galaxy's (its error messages or tables were not found)")
	}
	return nil
}

// ApplyBuckets moves the files of a prepared disc into their bucket folders.
func ApplyBuckets(out string, minFiles, perBucket int) error {
	moves, err := BucketLayout(filepath.Join(out, "sys", "fst.bin"), minFiles, perBucket)
	if err != nil {
		return err
	}
	missing := 0
	for _, m := range moves {
		s, d := filepath.Join(out, filepath.FromSlash(m.From)), filepath.Join(out, filepath.FromSlash(m.To))
		if _, err := os.Stat(s); err == nil {
			os.MkdirAll(filepath.Dir(d), 0o755)
			os.Remove(d)
			if err := os.Rename(s, d); err != nil {
				return err
			}
		} else if _, err := os.Stat(d); err != nil {
			missing++
		}
	}
	if missing > 0 {
		return fmt.Errorf("%d game files are missing from the extracted disc", missing)
	}
	return nil
}
