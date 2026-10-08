package main

import (
	"errors"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// copyToClipboard uses the platform's own clipboard tool so the binary stays
// dependency free.
func copyToClipboard(text string) error {
	var candidates [][]string
	switch runtime.GOOS {
	case "darwin":
		candidates = [][]string{{"pbcopy"}}
	case "windows":
		candidates = [][]string{{"clip"}}
	default:
		if os.Getenv("WAYLAND_DISPLAY") != "" {
			candidates = append(candidates, []string{"wl-copy"})
		}
		candidates = append(candidates,
			[]string{"xclip", "-selection", "clipboard"},
			[]string{"xsel", "--clipboard", "--input"},
			[]string{"wl-copy"},
		)
	}
	for _, c := range candidates {
		path, err := exec.LookPath(c[0])
		if err != nil {
			continue
		}
		cmd := exec.Command(path, c[1:]...)
		cmd.Stdin = strings.NewReader(text)
		if err := cmd.Run(); err == nil {
			return nil
		}
	}
	if runtime.GOOS == "linux" {
		return errors.New("no clipboard tool found (install wl-clipboard, xclip or xsel)")
	}
	return errors.New("clipboard is not available")
}
