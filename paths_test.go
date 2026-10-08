package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestTokenizeUnix(t *testing.T) {
	cases := map[string][]string{
		`/Users/me/My\ File.txt`:                 {"/Users/me/My File.txt"},
		`/a/one.txt /b/two\ words.txt`:           {"/a/one.txt", "/b/two words.txt"},
		`'/a/it''s.txt'`:                         {"/a/its.txt"},
		`"/a/quoted name.txt" '/b/single q.txt'`: {"/a/quoted name.txt", "/b/single q.txt"},
		`/a/back\\slash`:                         {`/a/back\slash`},
	}
	for in, want := range cases {
		if got := tokenize(in, true); !reflect.DeepEqual(got, want) {
			t.Errorf("tokenize(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestTokenizeWindows(t *testing.T) {
	in := `"C:\Users\me\My File.txt" C:\data\b.zip`
	want := []string{`C:\Users\me\My File.txt`, `C:\data\b.zip`}
	if got := tokenize(in, false); !reflect.DeepEqual(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFileURLToPath(t *testing.T) {
	cases := []struct {
		in      string
		windows bool
		want    string
	}{
		{"file:///Users/me/My%20File.txt", false, "/Users/me/My File.txt"},
		{"file:///home/me/a b.txt", false, "/home/me/a b.txt"},
		{"file://localhost/tmp/x.txt", false, "/tmp/x.txt"},
		{"file:///C:/Users/me/My%20File.txt", true, `C:\Users\me\My File.txt`},
		{"file://C:/Users/me/a b.txt", true, `C:\Users\me\a b.txt`},
		{"file://localhost/D:/data/x.zip", true, `D:\data\x.zip`},
		{"file://server/share/x.txt", true, `\\server\share\x.txt`},
	}
	for _, c := range cases {
		got, ok := fileURLToPath(c.in, c.windows)
		if !ok || got != c.want {
			t.Errorf("fileURLToPath(%q, windows=%v) = %q, %v; want %q", c.in, c.windows, got, ok, c.want)
		}
	}
}

func TestSplitPathsPrefersExistingFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "name with spaces.txt")
	os.WriteFile(p, []byte("x"), 0o600)
	if got := splitPaths("  " + p + "  "); !reflect.DeepEqual(got, []string{p}) {
		t.Errorf("got %q", got)
	}
	if got := splitPaths("file://" + filepath.ToSlash(p)); len(got) != 1 || !exists(got[0]) {
		t.Errorf("file URL not resolved: %q", got)
	}
}
