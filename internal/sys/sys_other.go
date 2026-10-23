//go:build !windows

package sys

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
)

func hide(*exec.Cmd) {}

func pickWindows(string, string) (string, bool) { return "", false }

// Alert reports a startup error.
func Alert(title, msg string) {
	fmt.Fprintln(os.Stderr, title+": "+msg)
	if runtime.GOOS == "darwin" {
		exec.Command("osascript", "-e", "display alert "+quoteAS(title)+" message "+quoteAS(msg)).Run()
	}
}

// Init does nothing outside Windows.
func Init() {}
