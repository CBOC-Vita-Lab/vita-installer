//go:build !windows

package selfupdate

import (
	"os/exec"
	"syscall"
)

// detach starts the program in its own session, so it outlives this one.
func detach(cmd *exec.Cmd) { cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true} }
