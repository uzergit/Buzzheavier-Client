//go:build windows

package main

import (
	"os"
	"os/exec"
)

// restartSelf starts the freshly installed program in this console window
// and exits once it is closed.
func restartSelf() error {
	exe, err := currentExecutable()
	if err != nil {
		return err
	}
	cmd := exec.Command(exe, os.Args[1:]...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Start(); err != nil {
		return err
	}
	cmd.Wait()
	os.Exit(cmd.ProcessState.ExitCode())
	return nil
}
