//go:build !windows

package install

import "os/exec"

func hideConsole(*exec.Cmd) {}
