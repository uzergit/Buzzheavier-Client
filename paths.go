package main

import (
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"unicode"
)

// splitPaths turns whatever was typed, pasted or dragged into the terminal
// into file paths. Terminals differ: macOS escapes spaces with backslashes,
// Windows wraps paths in double quotes, some apps paste file:// URLs, and
// several dragged files arrive separated by spaces.
func splitPaths(input string) []string {
	s := strings.TrimSpace(input)
	if s == "" {
		return nil
	}
	if p := cleanPath(s); exists(p) {
		return []string{p}
	}
	toks := tokenize(s, runtime.GOOS != "windows")
	for i := range toks {
		toks[i] = cleanPath(toks[i])
	}
	return toks
}

func tokenize(s string, backslashEscapes bool) []string {
	var (
		out   []string
		cur   strings.Builder
		inTok bool
		quote rune
	)
	rs := []rune(s)
	for i := 0; i < len(rs); i++ {
		r := rs[i]
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
			} else if r == '\\' && quote == '"' && backslashEscapes && i+1 < len(rs) && (rs[i+1] == '"' || rs[i+1] == '\\') {
				i++
				cur.WriteRune(rs[i])
			} else {
				cur.WriteRune(r)
			}
		case r == '"' || r == '\'':
			quote, inTok = r, true
		case r == '\\' && backslashEscapes && i+1 < len(rs):
			i++
			cur.WriteRune(rs[i])
			inTok = true
		case unicode.IsSpace(r):
			if inTok {
				out = append(out, cur.String())
				cur.Reset()
				inTok = false
			}
		default:
			cur.WriteRune(r)
			inTok = true
		}
	}
	if inTok {
		out = append(out, cur.String())
	}
	return out
}

func cleanPath(p string) string {
	if strings.HasPrefix(p, "file://") {
		if u, err := url.Parse(p); err == nil && u.Path != "" {
			p = u.Path
			if runtime.GOOS == "windows" {
				p = strings.TrimPrefix(p, "/")
				p = filepath.FromSlash(p)
			}
		}
	}
	if p == "~" || strings.HasPrefix(p, "~/") || strings.HasPrefix(p, `~\`) {
		if home, err := os.UserHomeDir(); err == nil {
			p = filepath.Join(home, p[1:])
		}
	}
	return p
}

func exists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}
