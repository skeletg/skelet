package tui

import (
	"syscall"
	"unsafe"
)

const getTermios, setTermios = syscall.TIOCGETA, syscall.TIOCSETA

func restoreFlushed(fd uintptr, old *syscall.Termios) {
	_ = ioctl(fd, syscall.TIOCSETAF, unsafe.Pointer(old))
}
