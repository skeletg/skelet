package tui

import (
	"syscall"
	"unsafe"
)

const getTermios, setTermios = syscall.TCGETS, syscall.TCSETS

const tcflsh = 0x540B // TCFLSH on amd64 and arm64, which syscall names only for the first

func restoreFlushed(fd uintptr, old *syscall.Termios) {
	_ = ioctl(fd, setTermios, unsafe.Pointer(old))
	_, _, _ = syscall.Syscall(syscall.SYS_IOCTL, fd, tcflsh, syscall.TCIFLUSH)
}
