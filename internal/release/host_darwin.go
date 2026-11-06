package release

import (
	"os/exec"
	"strconv"
	"strings"
)

// macMajor is the macOS major version (14 for Sonoma), or 0 when it can't be
// told. macOS 10.x reports 10.
func macMajor() int {
	out, err := exec.Command("/usr/bin/sw_vers", "-productVersion").Output()
	if err != nil {
		return 0
	}
	n, _ := strconv.Atoi(strings.SplitN(strings.TrimSpace(string(out)), ".", 2)[0])
	return n
}
