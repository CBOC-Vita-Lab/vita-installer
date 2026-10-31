package install

import (
	"os/exec"
	"strconv"
	"syscall"
)

func hideConsole(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
}

// buildProc hides the console and lets stopping a build also stop the
// compilers it started.
func buildProc(cmd *exec.Cmd) {
	hideConsole(cmd)
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		kill := exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(cmd.Process.Pid))
		hideConsole(kill)
		kill.Run()
		return cmd.Process.Kill()
	}
}
