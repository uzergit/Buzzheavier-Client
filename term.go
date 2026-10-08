package main

import (
	"fmt"
	"os"
	"strings"
	"unicode/utf8"
)

var (
	stdoutIsTTY bool
	stdinIsTTY  bool
	useColor    bool
)

func initTerminal() {
	stdoutIsTTY = isTerminal(os.Stdout)
	stdinIsTTY = isTerminal(os.Stdin)
	useColor = stdoutIsTTY && enableVT() && os.Getenv("NO_COLOR") == "" && os.Getenv("TERM") != "dumb"
	if useColor {
		fmt.Print("\033]0;Buzzheavier Client\007")
	}
}

func style(code, s string) string {
	if !useColor {
		return s
	}
	return "\033[" + code + "m" + s + "\033[0m"
}

func bold(s string) string   { return style("1", s) }
func dim(s string) string    { return style("2", s) }
func red(s string) string    { return style("31", s) }
func green(s string) string  { return style("32", s) }
func yellow(s string) string { return style("33", s) }
func cyan(s string) string   { return style("36", s) }

func clearScreen() {
	if useColor {
		fmt.Print("\033[H\033[2J\033[3J")
	} else {
		fmt.Println()
	}
}

const ruleWidth = 52

func rule() { fmt.Println("  " + dim(strings.Repeat("─", ruleWidth))) }

func header(title string) {
	clearScreen()
	fmt.Println()
	fmt.Println("  " + bold(cyan(title)))
	rule()
}

func okf(format string, a ...any)   { fmt.Println("  " + green("✓ ") + fmt.Sprintf(format, a...)) }
func errf(format string, a ...any)  { fmt.Println("  " + red("✗ ") + fmt.Sprintf(format, a...)) }
func warnf(format string, a ...any) { fmt.Println("  " + yellow("! ") + fmt.Sprintf(format, a...)) }
func infof(format string, a ...any) { fmt.Println("  " + dim(fmt.Sprintf(format, a...))) }

func menuItem(key, label string, extra ...string) {
	line := fmt.Sprintf("  %s  %s", cyan(fmt.Sprintf("%2s", key)), label)
	if len(extra) > 0 && extra[0] != "" {
		line += "  " + dim(extra[0])
	}
	fmt.Println(line)
}

// truncate shortens s to at most n runes, marking the cut with an ellipsis.
func truncate(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	return string(r[:n-1]) + "…"
}

func pad(s string, n int) string {
	if c := utf8.RuneCountInString(s); c < n {
		return s + strings.Repeat(" ", n-c)
	}
	return s
}

func maskSecret(s string) string {
	r := []rune(s)
	if len(r) <= 8 {
		return strings.Repeat("•", len(r))
	}
	return string(r[:4]) + "…" + string(r[len(r)-4:])
}

func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGTPE"[exp])
}
