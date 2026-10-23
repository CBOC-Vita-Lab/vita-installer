package prep

import (
	"encoding/binary"
	"errors"
	"os"
	"strings"
)

// PSPGameID reads the game ID (for example ULUS-10054) from a PSP ISO.
func PSPGameID(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	const sector = 2048
	pvd := make([]byte, sector)
	if _, err := f.ReadAt(pvd, 16*sector); err != nil || string(pvd[1:6]) != "CD001" {
		return "", errors.New("this is not a PSP ISO (CSO and PBP files are not supported)")
	}
	root := pvd[156 : 156+34]
	lba := int64(binary.LittleEndian.Uint32(root[2:]))
	size := int64(binary.LittleEndian.Uint32(root[10:]))
	if size <= 0 || size > 1<<20 {
		return "", errors.New("this PSP ISO is damaged")
	}
	dir := make([]byte, size)
	if _, err := f.ReadAt(dir, lba*sector); err != nil {
		return "", errors.New("this PSP ISO is damaged")
	}
	for i := 0; i < len(dir); {
		l := int(dir[i])
		if l == 0 { // records do not cross sector boundaries
			i = (i/sector + 1) * sector
			continue
		}
		if i+l > len(dir) || l < 34 {
			break
		}
		rec := dir[i : i+l]
		nameLen := int(rec[32])
		if 33+nameLen <= len(rec) {
			name := strings.ToUpper(strings.SplitN(string(rec[33:33+nameLen]), ";", 2)[0])
			if name == "UMD_DATA.BIN" {
				b := make([]byte, 64)
				if _, err := f.ReadAt(b, int64(binary.LittleEndian.Uint32(rec[2:]))*sector); err != nil {
					return "", err
				}
				id := strings.SplitN(string(b), "|", 2)[0]
				if len(id) == 10 {
					return id, nil
				}
				break
			}
		}
		i += l
	}
	return "", errors.New("this PSP ISO has no game ID (UMD_DATA.BIN)")
}
