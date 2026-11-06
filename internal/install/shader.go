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
	Present bool `json:"present"` // libshacccg.suprx is in ur0:data/ or ur0:data/external/
	Damaged bool `json:"damaged"` // a libshacccg.suprx is there but empty or cut short
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
		present, damaged := shaderOnVita(fs)
		return ShaderStatus{Known: true, Present: present, Damaged: damaged, Helper: fs.exists("ux0:app/" + SharkTitleID + "/eboot.bin")}
	case drive != "":
		fs = driveFiles{&driveTarget{root: drive}}
		return ShaderStatus{Helper: fs.exists("ux0:app/" + SharkTitleID + "/eboot.bin")}
	}
	return ShaderStatus{}
}

// shaderMinBytes is less than any real libshacccg.suprx; a smaller file was
// cut short (a copy that failed, or an extraction that stopped).
const shaderMinBytes = 64 << 10

// shaderOnVita reports whether a usable shader compiler is in one of the
// places the games load it from, and whether a damaged one was seen.
func shaderOnVita(fs vitaFiles) (present, damaged bool) {
	for _, p := range release.ShaderPaths {
		if f, ok := fs.(ftpFiles); ok {
			n, found, err := f.c.Size(p)
			if err != nil || !found {
				continue
			}
			if n < shaderMinBytes {
				damaged = true
				continue
			}
			return true, false
		}
		if fs.exists(p) {
			return true, false
		}
	}
	return false, damaged
}
