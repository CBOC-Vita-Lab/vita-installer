// Package catalog checks a folder of files against a list of expected sizes
// and SHA-256 hashes.
package catalog

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
)

// Entry is one expected file. A file may be allowed several hashes.
type Entry struct {
	Path   string
	Bytes  int64
	SHA256 []string
}

// Catalog is a list of expected files.
type Catalog struct{ Files []Entry }

// Parse reads either {"files":[{"path","bytes","sha256"}]} or the compact
// {"files":[[path, bytes, sha256]]} form; gzip is detected.
func Parse(b []byte) (*Catalog, error) {
	if len(b) > 2 && b[0] == 0x1f && b[1] == 0x8b {
		zr, err := gzip.NewReader(bytes.NewReader(b))
		if err != nil {
			return nil, err
		}
		if b, err = io.ReadAll(zr); err != nil {
			return nil, err
		}
	}
	var raw struct {
		Files    []json.RawMessage `json:"files"`
		Metadata []json.RawMessage `json:"metadata"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, fmt.Errorf("file list is not valid JSON: %v", err)
	}
	c := &Catalog{}
	for _, list := range [][]json.RawMessage{raw.Files, raw.Metadata} {
		for _, m := range list {
			var e Entry
			var obj struct {
				Path    string   `json:"path"`
				Bytes   int64    `json:"bytes"`
				SHA256  string   `json:"sha256"`
				SHA256s []string `json:"sha256s"`
			}
			var arr []json.RawMessage
			if json.Unmarshal(m, &obj) == nil && obj.Path != "" {
				e = Entry{obj.Path, obj.Bytes, obj.SHA256s}
				if obj.SHA256 != "" {
					e.SHA256 = append(e.SHA256, obj.SHA256)
				}
			} else if json.Unmarshal(m, &arr) == nil && len(arr) == 3 {
				json.Unmarshal(arr[0], &e.Path)
				json.Unmarshal(arr[1], &e.Bytes)
				var one string
				if json.Unmarshal(arr[2], &one) == nil {
					e.SHA256 = []string{one}
				} else {
					json.Unmarshal(arr[2], &e.SHA256)
				}
			} else {
				return nil, fmt.Errorf("file list entry %s is not understood", string(m))
			}
			if e.Path == "" || strings.Contains(e.Path, "..") {
				return nil, fmt.Errorf("file list has an unsafe path %q", e.Path)
			}
			c.Files = append(c.Files, e)
		}
	}
	return c, nil
}

// Load reads a catalog file.
func Load(path string) (*Catalog, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Parse(b)
}

func skipped(p string, skip []string) bool {
	for _, s := range skip {
		if strings.HasPrefix(p, s) {
			return true
		}
	}
	return false
}

// Problem describes files that are missing or different.
type Problem struct {
	Missing, Different []string
}

func (p *Problem) Error() string {
	var parts []string
	if n := len(p.Missing); n > 0 {
		parts = append(parts, fmt.Sprintf("%d missing (for example %s)", n, p.Missing[0]))
	}
	if n := len(p.Different); n > 0 {
		parts = append(parts, fmt.Sprintf("%d different (for example %s)", n, p.Different[0]))
	}
	return "files are " + strings.Join(parts, " and ")
}

// CheckSizes quickly checks that every file exists with the right size.
func (c *Catalog) CheckSizes(dir string, skip []string) error {
	p := &Problem{}
	for _, e := range c.Files {
		if skipped(e.Path, skip) {
			continue
		}
		st, err := os.Stat(filepath.Join(dir, filepath.FromSlash(e.Path)))
		if err != nil {
			p.Missing = append(p.Missing, e.Path)
		} else if st.Size() != e.Bytes {
			p.Different = append(p.Different, e.Path)
		}
	}
	if len(p.Missing)+len(p.Different) > 0 {
		sort.Strings(p.Missing)
		sort.Strings(p.Different)
		return p
	}
	return nil
}

// Verify hashes every file (in parallel) and compares it with the catalog.
// progress receives bytes hashed so far and the total.
func (c *Catalog) Verify(dir string, skip []string, progress func(done, total int64)) error {
	if err := c.CheckSizes(dir, skip); err != nil {
		return err
	}
	var total int64
	var todo []Entry
	for _, e := range c.Files {
		if !skipped(e.Path, skip) {
			todo = append(todo, e)
			total += e.Bytes
		}
	}
	sort.Slice(todo, func(i, j int) bool { return todo[i].Bytes > todo[j].Bytes })
	var done int64
	var mu sync.Mutex
	p := &Problem{}
	ch := make(chan Entry)
	var wg sync.WaitGroup
	for w := 0; w < runtime.NumCPU(); w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			buf := make([]byte, 1<<20)
			for e := range ch {
				ok := false
				if f, err := os.Open(filepath.Join(dir, filepath.FromSlash(e.Path))); err == nil {
					h := sha256.New()
					for {
						n, rerr := f.Read(buf)
						h.Write(buf[:n])
						if progress != nil && n > 0 {
							progress(atomic.AddInt64(&done, int64(n)), total)
						}
						if rerr != nil {
							break
						}
					}
					f.Close()
					sum := hex.EncodeToString(h.Sum(nil))
					for _, want := range e.SHA256 {
						if strings.EqualFold(sum, want) {
							ok = true
						}
					}
				}
				if !ok {
					mu.Lock()
					p.Different = append(p.Different, e.Path)
					mu.Unlock()
				}
			}
		}()
	}
	for _, e := range todo {
		ch <- e
	}
	close(ch)
	wg.Wait()
	if len(p.Different) > 0 {
		sort.Strings(p.Different)
		return p
	}
	return nil
}
