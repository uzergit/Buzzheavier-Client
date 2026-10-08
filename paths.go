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
	if strings.HasPrefix(strings.ToLower(p), "file://") {
		if fp, ok := fileURLToPath(p, runtime.GOOS == "windows"); ok {
			p = fp
		}
	}
	if p == "~" || strings.HasPrefix(p, "~/") || strings.HasPrefix(p, `~\`) {
		if home, err := os.UserHomeDir(); err == nil {
			p = filepath.Join(home, p[1:])
		}
	}
	return p
}

// fileURLToPath turns a file:// URL into a local path. On Windows it handles
// the proper form (file:///C:/x), the sloppy form some apps paste where the
// drive letter ends up as the URL host (file://C:/x), and network shares
// (file://server/share/x). Spaces may be raw or %20-encoded.
func fileURLToPath(raw string, windows bool) (string, bool) {
	u, err := url.Parse(raw)
	if err != nil || !strings.EqualFold(u.Scheme, "file") {
		return "", false
	}
	host, p := u.Host, u.Path
	if strings.EqualFold(host, "localhost") {
		host = ""
	}
	if !windows {
		if p == "" {
			return "", false
		}
		return p, true
	}
	switch {
	case isDriveLetter(host):
		// file://C:/Users/x  ->  host "C:", path "/Users/x"
		p = host + p
	case host != "":
		// file://server/share/x  ->  \\server\share\x
		p = `//` + host + p
	case len(p) >= 3 && p[0] == '/' && isDriveLetter(p[1:3]):
		// file:///C:/Users/x  ->  path "/C:/Users/x"
		p = p[1:]
	}
	if p == "" {
		return "", false
	}
	return strings.ReplaceAll(p, "/", `\`), true
}

func isDriveLetter(s string) bool {
	return len(s) == 2 && s[1] == ':' &&
		(s[0] >= 'a' && s[0] <= 'z' || s[0] >= 'A' && s[0] <= 'Z')
}

func exists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}
