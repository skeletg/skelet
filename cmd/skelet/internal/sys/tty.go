//go:build linux || darwin

package sys

import (
	"syscall"
	"unsafe"
)

// IsTerminal is bash's [ -t fd ]: the descriptor answers a terminal ioctl. A file mode check would call /dev/null a
// terminal, since it is a character device too
func IsTerminal(fd uintptr) bool {
	var t syscall.Termios
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, fd, getTermios, uintptr(unsafe.Pointer(&t)))
	return errno == 0
}
