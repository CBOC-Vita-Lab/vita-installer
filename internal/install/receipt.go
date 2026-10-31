package install

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/CBOC-Vita-Lab/vita-installer/internal/device"
	"github.com/CBOC-Vita-Lab/vita-installer/internal/ftp"
	"github.com/CBOC-Vita-Lab/vita-installer/internal/release"
)

// ReceiptDir is where the installer records what it installed, one file per
// app. It lets updates tell a changed file from an unchanged one even when
// both have the same size, and tells the page which version is installed.
const ReceiptDir = Staging + "/installed"

// Receipt describes an installed release.
type Receipt struct {
	Name        string            `json:"name"`
	TitleID     string            `json:"titleId"`
	Version     string            `json:"version,omitempty"` // the app's own version (param.sfo)
	Release     string            `json:"release,omitempty"` // the release's tag or version
	Repo        string            `json:"repo,omitempty"`    // GitHub owner/repo it came from
	Source      string            `json:"source,omitempty"`  // file the player chose
	Build       string            `json:"build,omitempty"`   // fingerprint of the app and its files
	InstalledAt time.Time         `json:"installedAt"`
	Files       map[string]string `json:"files"` // Vita path -> SHA-1 of the release's own files
}

func receiptPath(titleID string) string { return ReceiptDir + "/" + titleID + ".json" }

// fileHash returns the SHA-1 of a local file.
func fileHash(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha1.New()
	if _, err := io.CopyBuffer(h, f, make([]byte, 1<<20)); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func parseReceipt(b []byte) *Receipt {
	var r Receipt
	if json.Unmarshal(b, &r) != nil || r.TitleID == "" {
		return nil
	}
	if r.Files == nil {
		r.Files = map[string]string{}
	}
	return &r
}

func readReceipt(ctx context.Context, t target, titleID string) *Receipt {
	b, err := t.read(ctx, receiptPath(titleID), 4<<20)
	if err != nil {
		return nil
	}
	return parseReceipt(b)
}

func writeReceipt(ctx context.Context, t target, cache string, r *Receipt) error {
	b, _ := json.MarshalIndent(r, "", " ")
	tmp := filepath.Join(cache, "receipt-"+r.TitleID+".json")
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	defer os.Remove(tmp)
	if err := t.mkdirs(ctx, missingDirs(t, ReceiptDir)); err != nil {
		return err
	}
	return t.put(ctx, 0, tmp, receiptPath(r.TitleID), func(int64) {})
}

func missingDirs(t target, dir string) []string {
	var out []string
	for d := dir; strings.Contains(d, "/"); d = d[:strings.LastIndexByte(d, '/')] {
		if t.exists(d) {
			break
		}
		out = append([]string{d}, out...)
	}
	return out
}

// Installed lists the receipts on a Vita or drive, newest first.
func Installed(ctx context.Context, v *device.Vita, drive string) ([]*Receipt, error) {
	var out []*Receipt
	if v != nil {
		c, err := ftp.Dial(ctx, v.Addr(), 8*time.Second)
		if err != nil {
			return nil, err
		}
		defer c.Close()
		entries, err := c.List(ReceiptDir)
		if err != nil {
			return nil, nil // nothing installed by this installer yet
		}
		for _, e := range entries {
			if e.IsDir || !strings.HasSuffix(e.Name, ".json") {
				continue
			}
			if b, err := c.Retrieve(ReceiptDir+"/"+e.Name, 4<<20); err == nil {
				if r := parseReceipt(b); r != nil {
					out = append(out, r)
				}
			}
		}
	} else if drive != "" {
		d := &driveTarget{root: drive}
		dir, _ := d.local(ReceiptDir)
		files, _ := filepath.Glob(filepath.Join(dir, "*.json"))
		for _, f := range files {
			if b, err := os.ReadFile(f); err == nil {
				if r := parseReceipt(b); r != nil {
					out = append(out, r)
				}
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].InstalledAt.After(out[j].InstalledAt) })
	return out, nil
}

// newReceipt starts the receipt for a release being installed.
func newReceipt(r *release.Release) *Receipt {
	rc := &Receipt{Name: r.Name, TitleID: r.TitleID, Release: r.Tag, Repo: r.Repo, Source: r.Source, InstalledAt: time.Now().UTC(), Files: map[string]string{}}
	if r.App != nil {
		rc.Version = r.App.Version
	}
	if rc.Release == "" {
		rc.Release = r.Version
	}
	return rc
}

// buildHash fingerprints a release: its VPK and the files it ships.
func buildHash(r *release.Release) string {
	h := sha1.New()
	if r.Build != nil {
		// The app is made on the player's computer: it is the same app when it
		// comes from the same recipe, whichever computer built it.
		h.Write([]byte("build\x00" + r.Build.Fingerprint()))
	} else if sum, err := fileHash(r.VPK); err == nil {
		h.Write([]byte(sum))
	}
	for _, c := range r.Copies {
		sum, _ := fileHash(c.Local)
		h.Write([]byte(c.Remote + "\x00" + sum + "\n"))
	}
	if fp := r.Art.Fingerprint(); fp != "" {
		h.Write([]byte("art\x00" + fp))
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
}
