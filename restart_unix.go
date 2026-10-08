//go:build !windows

package main

import (
	"os"
	"syscall"
)

// restartSelf replaces this process with the freshly installed program.
func restartSelf() error {
	exe, err := currentExecutable()
	if err != nil {
		return err
	}
	return syscall.Exec(exe, os.Args, os.Environ())
}
