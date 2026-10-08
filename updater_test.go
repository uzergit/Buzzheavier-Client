package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestIsNewer(t *testing.T) {
	cases := []struct {
		latest, current string
		want            bool
	}{
		{"v2.1.0", "2.0.0", true},
		{"v2.0.1", "2.0.0", true},
		{"v10.0.0", "9.9.9", true},
		{"v2.0.0", "2.0.0", false},
		{"v1.9.9", "2.0.0", false},
		{"v2.1", "2.0.9", true},
		{"bugs-included", "2.0.0", false},
		{"v2.1.0", "dev", false},
	}
	for _, c := range cases {
		if got := isNewer(c.latest, c.current); got != c.want {
			t.Errorf("isNewer(%q, %q) = %v", c.latest, c.current, got)
		}
	}
}

func TestPackageFor(t *testing.T) {
	for _, c := range [][4]string{
		{"windows", "amd64", "-Windows-x64.zip", "BuzzheavierClient.exe"},
		{"darwin", "arm64", "-macOS-AppleSilicon.zip", "buzzheavier"},
		{"darwin", "amd64", "-macOS-Intel.zip", "buzzheavier"},
		{"linux", "amd64", "-Linux-Universal.tar.gz", "buzzheavier-x86_64"},
		{"linux", "arm64", "-Linux-Universal.tar.gz", "buzzheavier-arm64"},
	} {
		if s, b := packageFor(c[0], c[1]); s != c[2] || b != c[3] {
			t.Errorf("packageFor(%s, %s) = %s, %s", c[0], c[1], s, b)
		}
	}
}

// buildPackage makes a release package for this system holding binary.
func buildPackage(t *testing.T, name string, binary []byte) []byte {
	_, binName := packageFor(runtime.GOOS, runtime.GOARCH)
	var buf bytes.Buffer
	if filepath.Ext(name) == ".zip" {
		zw := zip.NewWriter(&buf)
		w, _ := zw.Create("Buzzheavier-Client/README.txt")
		w.Write([]byte("readme"))
		w, _ = zw.Create("Buzzheavier-Client/" + binName)
		w.Write(binary)
		zw.Close()
	} else {
		gz := gzip.NewWriter(&buf)
		tw := tar.NewWriter(gz)
		tw.WriteHeader(&tar.Header{Name: "Buzzheavier-Client/bin/" + binName, Mode: 0o755, Size: int64(len(binary)), Typeflag: tar.TypeReg})
		tw.Write(binary)
		tw.Close()
		gz.Close()
	}
	return buf.Bytes()
}

func fakeGitHub(t *testing.T, tag string, pkg []byte, sumOverride string) {
	suffix, _ := packageFor(runtime.GOOS, runtime.GOARCH)
	if suffix == "" {
		t.Skip("no package for this system")
	}
	pkgName := "Buzzheavier-Client-" + tag + suffix
	sum := sha256.Sum256(pkg)
	sums := fmt.Sprintf("%s  %s\n%s  other.zip\n", hex.EncodeToString(sum[:]), pkgName, "00")
	if sumOverride != "" {
		sums = sumOverride + "  " + pkgName + "\n"
	}
	mux := http.NewServeMux()
	var srv *httptest.Server
	mux.HandleFunc("/latest", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"tag_name": tag, "html_url": "https://example.test/release", "body": "## New\n- **Updater**\n",
			"assets": []map[string]any{
				{"name": pkgName, "browser_download_url": srv.URL + "/pkg", "size": len(pkg)},
				{"name": "SHA256SUMS.txt", "browser_download_url": srv.URL + "/sums", "size": len(sums)},
			},
		})
	})
	mux.HandleFunc("/pkg", func(w http.ResponseWriter, r *http.Request) { w.Write(pkg) })
	mux.HandleFunc("/sums", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(sums)) })
	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	old := releasesURL
	releasesURL = srv.URL + "/latest"
	t.Cleanup(func() { releasesURL = old })
}

func pkgName() string {
	suffix, _ := packageFor(runtime.GOOS, runtime.GOARCH)
	return "x" + suffix
}

func TestUpdateInstallsNewBinary(t *testing.T) {
	newBin := []byte("#!/bin/sh\necho new version\n")
	fakeGitHub(t, "v99.0.0", buildPackage(t, pkgName(), newBin), "")

	u, err := checkForUpdate(ctxAPI())
	if err != nil || u == nil || u.pkg == nil || u.version != "99.0.0" {
		t.Fatalf("check: %+v %v", u, err)
	}
	exe := filepath.Join(t.TempDir(), "buzzheavier")
	os.WriteFile(exe, []byte("old"), 0o755)
	if err := applyUpdateTo(ctxAPI(), u, exe, false); err != nil {
		t.Fatalf("apply: %v", err)
	}
	got, _ := os.ReadFile(exe)
	if !bytes.Equal(got, newBin) {
		t.Errorf("binary not replaced: %q", got)
	}
	if fi, _ := os.Stat(exe); runtime.GOOS != "windows" && fi.Mode()&0o111 == 0 {
		t.Errorf("new binary is not executable")
	}
	if _, err := os.Stat(exe + ".new"); err == nil {
		t.Errorf("temporary file left behind")
	}
}

func TestUpdateRejectsBadChecksum(t *testing.T) {
	fakeGitHub(t, "v99.0.0", buildPackage(t, pkgName(), []byte("evil")), "deadbeef")
	u, err := checkForUpdate(ctxAPI())
	if err != nil || u == nil {
		t.Fatalf("check: %v", err)
	}
	exe := filepath.Join(t.TempDir(), "buzzheavier")
	os.WriteFile(exe, []byte("old"), 0o755)
	if err := applyUpdateTo(ctxAPI(), u, exe, false); err == nil {
		t.Fatal("expected checksum failure")
	}
	if got, _ := os.ReadFile(exe); string(got) != "old" {
		t.Errorf("binary changed despite bad checksum")
	}
}

func TestUpdateFallsBackToReleasesPage(t *testing.T) {
	suffix, _ := packageFor(runtime.GOOS, runtime.GOARCH)
	if suffix == "" {
		t.Skip("no package for this system")
	}
	newBin := []byte("new from page")
	pkg := buildPackage(t, pkgName(), newBin)
	sum := sha256.Sum256(pkg)
	name := "Buzzheavier-Client-v99.1.0" + suffix
	mux := http.NewServeMux()
	mux.HandleFunc("/api", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusForbidden) })
	mux.HandleFunc("/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/releases/tag/v99.1.0", http.StatusFound)
	})
	mux.HandleFunc("/releases/download/v99.1.0/"+name, func(w http.ResponseWriter, r *http.Request) { w.Write(pkg) })
	mux.HandleFunc("/releases/download/v99.1.0/SHA256SUMS.txt", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "%s  %s\n", hex.EncodeToString(sum[:]), name)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	oldAPI, oldPage := releasesURL, releasesPage
	releasesURL, releasesPage = srv.URL+"/api", srv.URL+"/releases"
	defer func() { releasesURL, releasesPage = oldAPI, oldPage }()

	u, err := checkForUpdate(ctxAPI())
	if err != nil || u == nil || u.version != "99.1.0" || u.pkg == nil {
		t.Fatalf("fallback check: %+v %v", u, err)
	}
	exe := filepath.Join(t.TempDir(), "buzzheavier")
	os.WriteFile(exe, []byte("old"), 0o755)
	if err := applyUpdateTo(ctxAPI(), u, exe, false); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if got, _ := os.ReadFile(exe); !bytes.Equal(got, newBin) {
		t.Errorf("binary not replaced")
	}
}

func TestNoUpdateWhenCurrent(t *testing.T) {
	fakeGitHub(t, "v"+version, []byte("x"), "")
	if u, err := checkForUpdate(ctxAPI()); err != nil || u != nil {
		t.Errorf("expected no update, got %+v %v", u, err)
	}
}
