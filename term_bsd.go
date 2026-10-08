//go:build darwin || freebsd || netbsd || openbsd || dragonfly

package main

import "syscall"

const ioctlGetTermios = syscall.TIOCGETA
