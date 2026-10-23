// Package sys holds the small platform-specific pieces: opening the browser,
// native file pickers and error boxes.
package sys

import (
	"os/exec"
	"runtime"
	"strings"
)

// OpenURL opens url in the default browser.
func OpenURL(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	hide(cmd)
	return cmd.Start()
}

// Reveal shows a file or folder in Finder / Explorer.
func Reveal(path string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", "-R", path)
	case "windows":
		cmd = exec.Command("explorer", "/select,", path)
	default:
		cmd = exec.Command("xdg-open", path)
	}
	hide(cmd)
	cmd.Start()
}

// Pick shows a native open dialog. kind is "file" or "folder". It returns ""
// when the player cancels, and ok=false when no dialog is available.
func Pick(kind, prompt string) (path string, ok bool) {
	switch runtime.GOOS {
	case "darwin":
		what := "choose file"
		if kind == "folder" {
			what = "choose folder"
		}
		script := []string{"-e", "activate", "-e", "POSIX path of (" + what + " with prompt " + quoteAS(prompt) + ")"}
		out, err := exec.Command("osascript", script...).Output()
		if err != nil {
			return "", isExitErr(err) // an exit status means the player cancelled
		}
		p := strings.TrimSpace(string(out))
		if len(p) > 1 {
			p = strings.TrimRight(p, "/")
		}
		return p, true
	case "windows":
		return pickWindows(kind, prompt)
	default:
		if p, err := exec.LookPath("zenity"); err == nil {
			args := []string{"--file-selection", "--title=" + prompt}
			if kind == "folder" {
				args = append(args, "--directory")
			}
			out, err := exec.Command(p, args...).Output()
			if err != nil {
				return "", true
			}
			return strings.TrimSpace(string(out)), true
		}
		if p, err := exec.LookPath("kdialog"); err == nil {
			flag := "--getopenfilename"
			if kind == "folder" {
				flag = "--getexistingdirectory"
			}
			out, err := exec.Command(p, flag, ".").Output()
			if err != nil {
				return "", true
			}
			return strings.TrimSpace(string(out)), true
		}
		return "", false
	}
}

func isExitErr(err error) bool {
	_, ok := err.(*exec.ExitError)
	return ok
}

func quoteAS(s string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s) + `"`
}
