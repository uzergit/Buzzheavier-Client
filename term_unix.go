//go:build darwin || linux || freebsd || netbsd || openbsd || dragonfly

package main

import (
	"os"
	"syscall"
	"unsafe"
)

// isTerminal asks the terminal driver for its settings; only a real terminal
// answers. A plain character-device check would also accept /dev/null.
func isTerminal(f *os.File) bool {
	var t syscall.Termios
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, f.Fd(), ioctlGetTermios, uintptr(unsafe.Pointer(&t)))
	return errno == 0
}

func enableVT() bool { return true }
