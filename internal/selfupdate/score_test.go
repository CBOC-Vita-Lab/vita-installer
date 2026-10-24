package selfupdate

import (
	"runtime"
	"testing"
)

func TestAssetScore(t *testing.T) {
	names := []string{"CBOC-Vita-Installer-1.3-mac.zip", "CBOC-Vita-Installer-1.3-windows-x64.exe", "CBOC-Vita-Installer-1.3-windows-x86.exe",
		"CBOC-Vita-Installer-1.3-windows-arm64.exe", "CBOC-Vita-Installer-1.3-linux-amd64.tar.gz", "CBOC-Vita-Installer-1.3-linux-arm64.tar.gz",
		"SHA256SUMS.txt", "Vita-Installer-macOS.zip", "VitaInstaller_Win64.exe", "vita-installer-linux-x86_64.tgz"}
	best, second := "", ""
	bs := 0
	for _, n := range names {
		if s := AssetScore(n); s > bs {
			best, bs = n, s
		}
	}
	// Renamed downloads alone must still be found.
	renamed := map[string]string{"darwin": "Vita-Installer-macOS.zip", "windows/amd64": "VitaInstaller_Win64.exe", "linux/amd64": "vita-installer-linux-x86_64.tgz"}
	key := runtime.GOOS
	if runtime.GOOS != "darwin" {
		key += "/" + runtime.GOARCH
	}
	if r, ok := renamed[key]; ok {
		second = r
		if AssetScore(r) == 0 {
			t.Errorf("renamed download %s not recognised", r)
		}
	}
	if bs != 100 || AssetScore("SHA256SUMS.txt") != 0 {
		t.Errorf("best %s (%d)", best, bs)
	}
	_ = second
}
