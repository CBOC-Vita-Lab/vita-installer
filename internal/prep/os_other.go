//go:build !windows

package prep

import (
	"os/exec"
	"runtime"
	"strconv"
	"strings"
)

func hideWindow(*exec.Cmd) {}

// tooOldForNod reports macOS releases older than 12, which nodtool does not support.
func tooOldForNod() bool {
	if runtime.GOOS != "darwin" {
		return false
	}
	out, err := exec.Command("sw_vers", "-productVersion").Output()
	if err != nil {
		return false
	}
	major, _ := strconv.Atoi(strings.SplitN(strings.TrimSpace(string(out)), ".", 2)[0])
	return major > 0 && major < 12
}
