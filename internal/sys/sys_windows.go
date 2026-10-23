package sys

import (
	"os/exec"
	"strings"
	"syscall"
	"unsafe"
)

func hide(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
}

// pickWindows uses PowerShell's WinForms dialogs, which exist on Windows 7 and later.
func pickWindows(kind, prompt string) (string, bool) {
	p := strings.ReplaceAll(prompt, "'", "''")
	script := `Add-Type -AssemblyName System.Windows.Forms;` +
		`$o=New-Object System.Windows.Forms.Form;$o.TopMost=$true;$o.ShowInTaskbar=$false;$o.Opacity=0;$o.Show();$o.Activate();`
	if kind == "folder" {
		script += `$d=New-Object System.Windows.Forms.FolderBrowserDialog;$d.Description='` + p + `';$d.ShowNewFolderButton=$false;` +
			`if($d.ShowDialog($o) -eq 'OK'){[Console]::OutputEncoding=[Text.Encoding]::UTF8;Write-Output $d.SelectedPath}`
	} else {
		script += `$d=New-Object System.Windows.Forms.OpenFileDialog;$d.Title='` + p + `';$d.Filter='All files (*.*)|*.*';` +
			`if($d.ShowDialog($o) -eq 'OK'){[Console]::OutputEncoding=[Text.Encoding]::UTF8;Write-Output $d.FileName}`
	}
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-STA", "-ExecutionPolicy", "Bypass", "-Command", script)
	hide(cmd)
	out, err := cmd.Output()
	if err != nil {
		if _, isExit := err.(*exec.ExitError); isExit {
			return "", true
		}
		return "", false
	}
	return strings.TrimSpace(string(out)), true
}

// Alert shows a message box (used when the app cannot start).
func Alert(title, msg string) {
	t, _ := syscall.UTF16PtrFromString(title)
	m, _ := syscall.UTF16PtrFromString(msg)
	syscall.NewLazyDLL("user32.dll").NewProc("MessageBoxW").Call(0, uintptr(unsafe.Pointer(m)), uintptr(unsafe.Pointer(t)), 0x40)
}

// Init stops Windows from showing "no disk" dialogs when empty card readers are checked.
func Init() {
	syscall.NewLazyDLL("kernel32.dll").NewProc("SetErrorMode").Call(0x0001 | 0x8000)
}
