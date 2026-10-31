//go:build !windows

package install

import (
	"os/exec"
	"syscall"
)

func hideConsole(*exec.Cmd) {}

// buildProc lets stopping a build also stop the compilers it started: the
// program leads its own process group, and cancelling ends the whole group.
func buildProc(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); err != nil {
			return cmd.Process.Kill()
		}
		return nil
	}
}
