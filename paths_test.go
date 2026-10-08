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
