package install

import (
	"context"
	"time"

	"github.com/CBOC-Vita-Lab/vita-installer/internal/device"
	"github.com/CBOC-Vita-Lab/vita-installer/internal/ftp"
	"github.com/CBOC-Vita-Lab/vita-installer/internal/release"
)

// SharkTitleID is ShaRKBR33D's title ID: the free Vita app that gets the
// shader compiler for the player. The installer never ships the compiler; it
// is Sony's.
const SharkTitleID = "SHRKBR33D"

// ShaderStatus is what the Vita has for the shader compiler.
type ShaderStatus struct {
	Known   bool `json:"known"`   // the installer could look (it can't see ur0: on a drive)
	Present bool `json:"present"` // ur0:data/libshacccg.suprx is there
	Helper  bool `json:"helper"`  // ShaRKBR33D is installed
}

// CheckShader looks at the Vita over FTP. A drive only shows ux0:, so there
// the compiler can't be seen, only whether ShaRKBR33D is installed.
func CheckShader(ctx context.Context, v *device.Vita, drive string) ShaderStatus {
	var fs vitaFiles
	switch {
	case v != nil:
		c, err := ftp.Dial(ctx, v.Addr(), 8*time.Second)
		if err != nil {
			return ShaderStatus{}
		}
		defer c.Close()
		fs = ftpFiles{c}
		return ShaderStatus{Known: true, Present: fs.exists(release.ShaderPath), Helper: fs.exists("ux0:app/" + SharkTitleID + "/eboot.bin")}
	case drive != "":
		fs = driveFiles{&driveTarget{root: drive}}
		return ShaderStatus{Helper: fs.exists("ux0:app/" + SharkTitleID + "/eboot.bin")}
	}
	return ShaderStatus{}
}
