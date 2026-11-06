package prep

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

const isoSector = 2048

// Errors a PSP disc image can give. They are written for the player.
var (
	ErrNotPSPISO   = errors.New("this is not a PSP ISO (CSO and PBP files are not supported)")
	ErrDamagedISO  = errors.New("this PSP ISO is damaged")
	errNoGameIDMsg = "this PSP ISO has no game ID (UMD_DATA.BIN)"
)

// pspISO is a minimal ISO 9660 reader: enough to find a file by its path.
type pspISO struct {
	f    *os.File
	size int64 // size of the image file
	root extent
}

type extent struct {
	lba, size int64
	dir       bool
}

func openPSPISO(path string) (*pspISO, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	st, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}
	pvd := make([]byte, isoSector)
	if _, err := f.ReadAt(pvd, 16*isoSector); err != nil || string(pvd[1:6]) != "CD001" {
		f.Close()
		return nil, ErrNotPSPISO
	}
	root := pvd[156 : 156+34]
	p := &pspISO{f: f, size: st.Size(), root: extent{
		lba:  int64(binary.LittleEndian.Uint32(root[2:])),
		size: int64(binary.LittleEndian.Uint32(root[10:])),
		dir:  true,
	}}
	if p.root.size <= 0 || p.root.size > 1<<20 {
		f.Close()
		return nil, ErrDamagedISO
	}
	// The image says how many sectors it has; a shorter file was cut off.
	if blocks := int64(binary.LittleEndian.Uint32(pvd[80:])); blocks > 0 && blocks*isoSector > p.size {
		f.Close()
		return nil, fmt.Errorf("this PSP ISO is damaged or incomplete (it should be %s but is %s)", sizeText(blocks*isoSector), sizeText(p.size))
	}
	return p, nil
}

func sizeText(n int64) string {
	if n >= 1<<30 {
		return fmt.Sprintf("%.1f GB", float64(n)/(1<<30))
	}
	return fmt.Sprintf("%d MB", n>>20)
}

func (p *pspISO) close() { p.f.Close() }

// find looks a name up in one directory. Names are compared without the
// ";1" version suffix, ignoring case.
func (p *pspISO) find(dir extent, name string) (extent, bool, error) {
	data := make([]byte, dir.size)
	if _, err := p.f.ReadAt(data, dir.lba*isoSector); err != nil {
		return extent{}, false, ErrDamagedISO
	}
	for i := 0; i < len(data); {
		l := int(data[i])
		if l == 0 { // records do not cross sector boundaries
			i = (i/isoSector + 1) * isoSector
			continue
		}
		if i+l > len(data) || l < 34 {
			break
		}
		rec := data[i : i+l]
		nameLen := int(rec[32])
		if 33+nameLen <= len(rec) {
			got := strings.ToUpper(strings.SplitN(string(rec[33:33+nameLen]), ";", 2)[0])
			if got == name {
				return extent{
					lba:  int64(binary.LittleEndian.Uint32(rec[2:])),
					size: int64(binary.LittleEndian.Uint32(rec[10:])),
					dir:  rec[25]&2 != 0,
				}, true, nil
			}
		}
		i += l
	}
	return extent{}, false, nil
}

// lookup finds a file by its path inside the image ("PSP_GAME/SYSDIR/BOOT.BIN").
func (p *pspISO) lookup(path string) (extent, bool, error) {
	cur := p.root
	parts := strings.Split(strings.Trim(path, "/"), "/")
	for i, part := range parts {
		e, ok, err := p.find(cur, strings.ToUpper(part))
		if err != nil || !ok {
			return extent{}, false, err
		}
		if i < len(parts)-1 {
			if !e.dir || e.size <= 0 || e.size > 1<<20 {
				return extent{}, false, nil
			}
		} else if e.dir {
			return extent{}, false, nil
		}
		cur = e
	}
	return cur, true, nil
}

// PSPGameID reads the game ID (for example ULUS-10054) from a PSP ISO.
func PSPGameID(path string) (string, error) {
	p, err := openPSPISO(path)
	if err != nil {
		return "", err
	}
	defer p.close()
	e, ok, err := p.lookup("UMD_DATA.BIN")
	if err != nil {
		return "", err
	}
	if ok {
		b := make([]byte, 64)
		if _, err := p.f.ReadAt(b, e.lba*isoSector); err != nil {
			return "", ErrDamagedISO
		}
		if id := strings.SplitN(string(b), "|", 2)[0]; len(id) == 10 {
			return id, nil
		}
	}
	return "", errors.New(errNoGameIDMsg)
}

// ErrNotInISO is returned by PSPFileSHA256 when the image has no such file.
var ErrNotInISO = errors.New("the file is not in this PSP ISO")

// maxISOCheckFile keeps a damaged directory entry from making the check read
// a whole disc.
const maxISOCheckFile = 256 << 20

// PSPFileSHA256 returns the SHA-256 of a file inside a PSP ISO. A cut-off or
// unreadable image gives ErrDamagedISO (or a message that says so); a file the
// image does not hold gives ErrNotInISO.
func PSPFileSHA256(path, file string) (string, error) {
	p, err := openPSPISO(path)
	if err != nil {
		return "", err
	}
	defer p.close()
	e, ok, err := p.lookup(file)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", ErrNotInISO
	}
	if e.size > maxISOCheckFile || e.lba*isoSector+e.size > p.size {
		return "", ErrDamagedISO
	}
	h := sha256.New()
	if n, err := io.Copy(h, io.NewSectionReader(p.f, e.lba*isoSector, e.size)); err != nil || n != e.size {
		return "", ErrDamagedISO
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
