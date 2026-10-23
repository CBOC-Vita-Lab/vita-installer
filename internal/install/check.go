package install

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"strings"
	"time"

	"github.com/CBOC-Vita-Lab/vita-installer/internal/device"
	"github.com/CBOC-Vita-Lab/vita-installer/internal/ftp"
	"github.com/CBOC-Vita-Lab/vita-installer/internal/release"
)

// Present is what is already on the Vita for a release.
type Present struct {
	Installed bool            // the app is installed
	Inputs    map[string]bool // inputs whose files are already in place (by input ID)
}

// vitaFiles reads a few things from the Vita, over FTP or from a drive.
type vitaFiles interface {
	exists(path string) bool
	read(path string, limit int64) ([]byte, error)
}

type ftpFiles struct{ c *ftp.Conn }

func (f ftpFiles) exists(p string) bool { ok, _ := f.c.Exists(p); return ok }
func (f ftpFiles) read(p string, limit int64) ([]byte, error) {
	return f.c.Retrieve(p, limit)
}

type driveFiles struct{ d *driveTarget }

func (f driveFiles) exists(p string) bool { return f.d.exists(p) }
func (f driveFiles) read(p string, limit int64) ([]byte, error) {
	lp, err := f.d.local(p)
	if err != nil {
		return nil, err
	}
	fh, err := os.Open(lp)
	if err != nil {
		return nil, err
	}
	defer fh.Close()
	return io.ReadAll(io.LimitReader(fh, limit))
}

// Check looks at the Vita (or a mounted drive) to see whether the app is
// installed and which of the player's files are already there, so a
// reinstall or update does not ask for the game again.
func Check(ctx context.Context, v *device.Vita, drive string, r *release.Release) (*Present, error) {
	var fs vitaFiles
	if v != nil {
		c, err := ftp.Dial(ctx, v.Addr(), 8*time.Second)
		if err != nil {
			return nil, err
		}
		defer c.Close()
		fs = ftpFiles{c}
	} else if drive != "" {
		fs = driveFiles{&driveTarget{root: drive}}
	} else {
		return &Present{Inputs: map[string]bool{}}, nil
	}
	p := &Present{Inputs: map[string]bool{}}
	p.Installed = fs.exists("ux0:app/" + r.TitleID + "/eboot.bin")
	for _, in := range r.Inputs {
		p.Inputs[in.ID] = inPlace(fs, in, drive != "")
	}
	return p, nil
}

func inPlace(fs vitaFiles, in *release.Input, drive bool) bool {
	to := strings.TrimSuffix(in.To, "/")
	switch in.Type {
	case release.NeedShader:
		if drive {
			return false // ur0: is not visible on a drive
		}
		return fs.exists(release.ShaderPath)
	case release.NeedPSPISO, release.NeedFile:
		return fs.exists(to)
	case release.NeedWiiDisc, release.NeedGCDisc, release.NeedFolder:
		if len(in.Prepare) > 0 {
			b, err := fs.read(to+"/"+markerName, 256)
			return err == nil && string(b) == PreparedTag(in)
		}
		if in.Type == release.NeedFolder {
			return false
		}
		boot, err := fs.read(to+"/sys/boot.bin", 64)
		if err != nil || len(boot) < 6 || !fs.exists(to+"/files") {
			return false
		}
		if in.GameID != "" && !strings.EqualFold(string(boot[:6]), in.GameID) {
			return false
		}
		for rel, want := range in.Verify {
			data, err := fs.read(to+"/"+rel, 256<<20)
			if err != nil {
				return false
			}
			sum := sha256.Sum256(data)
			if !strings.EqualFold(hex.EncodeToString(sum[:]), want) {
				return false
			}
		}
		return true
	}
	return false
}

// markerName is written next to prepared game data once it is all on the Vita.
const markerName = ".cboc-prepared"

// PreparedTag identifies a need's prepared data (game and recipe).
func PreparedTag(in *release.Input) string {
	spec, _ := json.Marshal(in.Prepare)
	h := sha256.Sum256(append([]byte(in.GameID+"|"), spec...))
	return hex.EncodeToString(h[:16])
}
