//go:build !windows

package device

import "syscall"

// FreeSpace returns free bytes on the volume holding path, or -1.
func FreeSpace(path string) int64 {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return -1
	}
	return int64(st.Bavail) * int64(st.Bsize)
}
