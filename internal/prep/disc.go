// Package prep checks and prepares the game files players supply.
package prep

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Disc identifies a GameCube or Wii disc image or extracted disc folder.
type Disc struct {
	Path     string `json:"path"`
	GameID   string `json:"gameId"`
	Revision int    `json:"revision"`
	Folder   bool   `json:"folder"` // already extracted (sys/ and files/)
	Root     string `json:"-"`      // for folders: the directory holding sys/ and files/
}

// DiscExtensions are the image formats nodtool can read.
var DiscExtensions = []string{".iso", ".rvz", ".wia", ".wbfs", ".ciso", ".gcm", ".gcz", ".tgc", ".nfs"}

// IsDiscImage reports whether name has a disc image extension.
func IsDiscImage(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	for _, e := range DiscExtensions {
		if ext == e {
			return true
		}
	}
	return false
}

// IdentifyDisc reads the game ID and revision from a disc image or folder.
func IdentifyDisc(path string) (*Disc, error) {
	st, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if st.IsDir() {
		root, err := FindExtractedRoot(path)
		if err != nil {
			return nil, err
		}
		hdr := make([]byte, 8)
		f, err := os.Open(filepath.Join(root, "sys", "boot.bin"))
		if err != nil {
			return nil, err
		}
		defer f.Close()
		if _, err := io.ReadFull(f, hdr); err != nil {
			return nil, errors.New("sys/boot.bin is damaged")
		}
		d := &Disc{Path: path, Root: root, Folder: true, GameID: string(hdr[:6]), Revision: -1}
		// sys/boot.bin is the game partition's header, whose version byte is
		// not the disc revision. The disc's own header is kept in disc/header.bin.
		for _, h := range []string{filepath.Join(root, "disc", "header.bin"), filepath.Join(filepath.Dir(root), "disc", "header.bin")} {
			if b, err := os.ReadFile(h); err == nil && len(b) >= 8 && string(b[:6]) == d.GameID {
				d.Revision = int(b[7])
				break
			}
		}
		if d.Revision < 0 && string(hdr[:1]) == "G" { // GameCube: boot.bin is the disc header
			d.Revision = int(hdr[7])
		}
		return d, nil
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	head := make([]byte, 0x300)
	n, _ := io.ReadFull(f, head)
	head = head[:n]
	if n < 0x60 {
		return nil, errors.New("this file is too small to be a disc image")
	}
	off := -1
	switch {
	case string(head[:3]) == "RVZ" || string(head[:3]) == "WIA":
		off = 0x58 // header 2 begins at 0x48; its copy of the disc header starts 0x10 later
	case string(head[:4]) == "WBFS":
		off = 0x200
	case string(head[:4]) == "CISO":
		var b [8]byte
		if _, err := f.ReadAt(b[:], 0x8000); err == nil {
			return disc(path, b[:])
		}
	case binary.BigEndian.Uint32(head[0x18:]) == 0x5D1C9EA3 || binary.BigEndian.Uint32(head[0x1C:]) == 0xC2339F3D:
		off = 0 // plain Wii or GameCube image
	}
	if off < 0 || off+8 > len(head) {
		return nil, errors.New("this is not a GameCube or Wii disc image that can be read")
	}
	return disc(path, head[off:off+8])
}

func disc(path string, hdr []byte) (*Disc, error) {
	id := string(hdr[:6])
	for _, c := range id {
		if !(c >= 'A' && c <= 'Z' || c >= '0' && c <= '9') {
			return nil, errors.New("this is not a GameCube or Wii disc image that can be read")
		}
	}
	return &Disc{Path: path, GameID: id, Revision: int(hdr[7])}, nil
}

// FindExtractedRoot finds the folder that holds sys/ and files/ in an
// extracted disc: the folder itself, its DATA or P-xxxx partition folder, or
// a single subfolder.
func FindExtractedRoot(dir string) (string, error) {
	ok := func(d string) bool {
		a, err1 := os.Stat(filepath.Join(d, "sys", "boot.bin"))
		b, err2 := os.Stat(filepath.Join(d, "files"))
		return err1 == nil && err2 == nil && !a.IsDir() && b.IsDir()
	}
	if ok(dir) {
		return dir, nil
	}
	entries, _ := os.ReadDir(dir)
	var found []string
	for _, e := range entries {
		if e.IsDir() && ok(filepath.Join(dir, e.Name())) {
			found = append(found, filepath.Join(dir, e.Name()))
		}
	}
	for _, f := range found {
		if strings.EqualFold(filepath.Base(f), "DATA") {
			return f, nil
		}
	}
	if len(found) == 1 {
		return found[0], nil
	}
	return "", errors.New("this folder is not an extracted disc (it needs sys/ and files/ folders)")
}

// CheckDisc compares a disc against what a release needs.
func CheckDisc(d *Disc, gameID string, revision *int) error {
	if gameID != "" && !strings.EqualFold(d.GameID, gameID) {
		return fmt.Errorf("this is game %s%s, but this release needs %s", d.GameID, regionHint(d.GameID), gameID+regionHint(gameID))
	}
	if revision != nil && d.Revision >= 0 && d.Revision != *revision {
		return fmt.Errorf("this is revision %d of the game, but this release needs revision %d", d.Revision, *revision)
	}
	return nil
}

func regionHint(id string) string {
	if len(id) < 4 {
		return ""
	}
	switch id[3] {
	case 'E':
		return " (USA)"
	case 'P':
		return " (Europe)"
	case 'J':
		return " (Japan)"
	case 'K':
		return " (Korea)"
	}
	return ""
}

// VerifyFiles checks SHA-256 hashes of files inside an extracted disc.
func VerifyFiles(root string, want map[string]string) error {
	for rel, sum := range want {
		f, err := os.Open(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			return fmt.Errorf("%s is missing from the extracted game", rel)
		}
		h := sha256.New()
		_, err = io.Copy(h, f)
		f.Close()
		if err != nil {
			return err
		}
		if !strings.EqualFold(hex.EncodeToString(h.Sum(nil)), sum) {
			return fmt.Errorf("%s does not match the supported version of the game", rel)
		}
	}
	return nil
}
