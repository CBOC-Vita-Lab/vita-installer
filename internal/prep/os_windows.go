package prep

import (
	"os/exec"
	"syscall"
	"unsafe"
)

func hideWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000} // CREATE_NO_WINDOW
}

// tooOldForNod reports Windows versions before 10, which nodtool does not support.
func tooOldForNod() bool {
	type osVersionInfo struct {
		size, major, minor, build, platform uint32
		csd                                 [128]uint16
	}
	info := osVersionInfo{}
	info.size = uint32(unsafe.Sizeof(info))
	proc := syscall.NewLazyDLL("ntdll.dll").NewProc("RtlGetVersion")
	if proc.Find() != nil {
		return false
	}
	proc.Call(uintptr(unsafe.Pointer(&info)))
	return info.major > 0 && info.major < 10
}
