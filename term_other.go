//go:build !windows && !darwin && !linux && !freebsd && !netbsd && !openbsd && !dragonfly

package main

import "os"

func isTerminal(f *os.File) bool {
	fi, err := f.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}

func enableVT() bool { return true }
