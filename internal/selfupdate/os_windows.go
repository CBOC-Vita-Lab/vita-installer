package selfupdate

import (
	"os/exec"
	"syscall"
)

// detach starts the program without a console window.
func detach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x00000008} // DETACHED_PROCESS
}
