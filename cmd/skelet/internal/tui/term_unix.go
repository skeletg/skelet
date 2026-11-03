//go:build linux || darwin

package tui

import (
	"sync"
	"syscall"
	"unsafe"
)

var (
	rawMu   sync.Mutex
	rawUndo func()
)

func ioctl(fd, req uintptr, arg unsafe.Pointer) error {
	if _, _, e := syscall.Syscall(syscall.SYS_IOCTL, fd, req, uintptr(arg)); e != 0 {
		return e
	}
	return nil
}

// Raw hands every key over as it's pressed, unechoed - Ctrl-C included, as a byte rather than a signal
func Raw(fd uintptr) (func(), error) {
	var old syscall.Termios
	if err := ioctl(fd, getTermios, unsafe.Pointer(&old)); err != nil {
		return nil, err
	}
	t := old
	t.Iflag &^= syscall.ICRNL | syscall.IXON
	t.Lflag &^= syscall.ICANON | syscall.ECHO | syscall.ISIG | syscall.IEXTEN
	t.Cc[syscall.VMIN], t.Cc[syscall.VTIME] = 1, 0
	if err := ioctl(fd, setTermios, unsafe.Pointer(&t)); err != nil {
		return nil, err
	}
	undo := func() { _ = ioctl(fd, setTermios, unsafe.Pointer(&old)) }
	rawMu.Lock()
	rawUndo = undo
	rawMu.Unlock()
	return func() {
		rawMu.Lock()
		rawUndo = nil
		rawMu.Unlock()
		undo()
	}, nil
}

func Restore() {
	rawMu.Lock()
	undo := rawUndo
	rawUndo = nil
	rawMu.Unlock()
	if undo != nil {
		undo()
	}
}

// Hush stops echo while something runs and drops the keys typed meanwhile, so none answers the next question
func Hush(fd uintptr) (func(), error) {
	var old syscall.Termios
	if err := ioctl(fd, getTermios, unsafe.Pointer(&old)); err != nil {
		return nil, err
	}
	t := old
	t.Lflag &^= syscall.ECHO
	if err := ioctl(fd, setTermios, unsafe.Pointer(&t)); err != nil {
		return nil, err
	}
	return func() { restoreFlushed(fd, &old) }, nil
}

func Width(fd uintptr) int {
	var ws struct{ Row, Col, X, Y uint16 }
	if ioctl(fd, syscall.TIOCGWINSZ, unsafe.Pointer(&ws)) != nil || ws.Col == 0 {
		return 80
	}
	return int(ws.Col)
}
