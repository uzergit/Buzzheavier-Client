package main

import (
	"archive/tar"
	"archive/zip"
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

var releasesURL = envOr("BUZZHEAVIER_UPDATE_URL",
	"https://api.github.com/repos/uzergit/Buzzheavier-Client/releases/latest")

const (
	checksumsAsset = "SHA256SUMS.txt"
	maxPackageSize = 200 << 20
)

type releaseAsset struct {
	Name string `json:"name"`
	URL  string `json:"browser_download_url"`
	Size int64  `json:"size"`
}

type release struct {
	Tag        string         `json:"tag_name"`
	Body       string         `json:"body"`
	URL        string         `json:"html_url"`
	Draft      bool           `json:"draft"`
	Prerelease bool           `json:"prerelease"`
	Assets     []releaseAsset `json:"assets"`
}

// updateInfo describes a newer release. pkg is nil when the release has no
// package for this system, in which case only the release page is offered.
type updateInfo struct {
	version string
	notes   string
	page    string
	pkg     *releaseAsset
	sums    *releaseAsset
}

// parseVersion reads "v2.1.0" / "2.1" style versions; anything after - or +
// (e.g. "-dev") is ignored.
func parseVersion(s string) ([3]int, bool) {
	var v [3]int
	s = strings.TrimPrefix(strings.TrimSpace(s), "v")
	if i := strings.IndexAny(s, "-+ "); i >= 0 {
		s = s[:i]
	}
	parts := strings.Split(s, ".")
	if len(parts) == 0 || len(parts) > 3 {
		return v, false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return v, false
		}
		v[i] = n
	}
	return v, true
}

func isNewer(latest, current string) bool {
	l, ok1 := parseVersion(latest)
	c, ok2 := parseVersion(current)
	if !ok1 || !ok2 {
		return false
	}
	for i := range l {
		if l[i] != c[i] {
			return l[i] > c[i]
		}
	}
	return false
}

// packageFor names the release package and the binary inside it for a system.
func packageFor(goos, goarch string) (suffix, binary string) {
	switch {
	case goos == "windows":
		return "-Windows-x64.zip", "BuzzheavierClient.exe"
	case goos == "darwin" && goarch == "arm64":
		return "-macOS-AppleSilicon.zip", "buzzheavier"
	case goos == "darwin" && goarch == "amd64":
		return "-macOS-Intel.zip", "buzzheavier"
	case goos == "linux" && goarch == "amd64":
		return "-Linux-Universal.tar.gz", "buzzheavier-x86_64"
	case goos == "linux" && goarch == "arm64":
		return "-Linux-Universal.tar.gz", "buzzheavier-arm64"
	}
	return "", ""
}

func ghRequest(ctx context.Context, url string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "BuzzheavierClient/"+version)
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, friendlyNetErr(err)
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("GitHub answered HTTP %d", resp.StatusCode)
	}
	return resp, nil
}

// checkForUpdate returns nil when this is already the newest version.
func checkForUpdate(ctx context.Context) (*updateInfo, error) {
	resp, err := ghRequest(ctx, releasesURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var rel release
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&rel); err != nil {
		return nil, fmt.Errorf("could not read release info: %w", err)
	}
	if rel.Draft || rel.Prerelease || !isNewer(rel.Tag, version) {
		return nil, nil
	}
	u := &updateInfo{version: strings.TrimPrefix(rel.Tag, "v"), notes: rel.Body, page: rel.URL}
	suffix, _ := packageFor(runtime.GOOS, runtime.GOARCH)
	for i := range rel.Assets {
		a := &rel.Assets[i]
		switch {
		case a.Name == checksumsAsset:
			u.sums = a
		case suffix != "" && strings.HasSuffix(a.Name, suffix):
			u.pkg = a
		}
	}
	if u.sums == nil {
		u.pkg = nil // never install something that cannot be verified
	}
	return u, nil
}

// downloadAndVerify fetches the package and checks it against SHA256SUMS.txt.
func (u *updateInfo) downloadAndVerify(ctx context.Context, showProgress bool) ([]byte, error) {
	resp, err := ghRequest(ctx, u.sums.URL)
	if err != nil {
		return nil, fmt.Errorf("checksums: %w", err)
	}
	want := ""
	sc := bufio.NewScanner(io.LimitReader(resp.Body, 1<<20))
	for sc.Scan() {
		if f := strings.Fields(sc.Text()); len(f) == 2 && strings.TrimPrefix(f[1], "*") == u.pkg.Name {
			want = strings.ToLower(f[0])
		}
	}
	resp.Body.Close()
	if want == "" {
		return nil, errors.New("the release has no checksum for this package")
	}

	resp, err = ghRequest(ctx, u.pkg.URL)
	if err != nil {
		return nil, fmt.Errorf("download: %w", err)
	}
	defer resp.Body.Close()
	var body io.Reader = io.LimitReader(resp.Body, maxPackageSize)
	var pr *progressReader
	if showProgress && u.pkg.Size > 0 {
		pr = newProgress(body, u.pkg.Size)
		body = pr
	}
	data, err := io.ReadAll(body)
	if pr != nil {
		pr.Stop(err == nil)
	}
	if err != nil {
		return nil, fmt.Errorf("download: %w", friendlyNetErr(err))
	}
	sum := sha256.Sum256(data)
	if hex.EncodeToString(sum[:]) != want {
		return nil, errors.New("the download is damaged (checksum mismatch), nothing was changed")
	}
	return data, nil
}

// extractBinary pulls the program for this system out of a release package.
func extractBinary(pkg []byte, pkgName, binary string) ([]byte, error) {
	read := func(r io.Reader) ([]byte, error) { return io.ReadAll(io.LimitReader(r, maxPackageSize)) }
	if strings.HasSuffix(pkgName, ".zip") {
		zr, err := zip.NewReader(bytes.NewReader(pkg), int64(len(pkg)))
		if err != nil {
			return nil, err
		}
		for _, f := range zr.File {
			if !f.FileInfo().IsDir() && path.Base(f.Name) == binary {
				rc, err := f.Open()
				if err != nil {
					return nil, err
				}
				defer rc.Close()
				return read(rc)
			}
		}
	} else {
		gz, err := gzip.NewReader(bytes.NewReader(pkg))
		if err != nil {
			return nil, err
		}
		tr := tar.NewReader(gz)
		for {
			h, err := tr.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				return nil, err
			}
			if h.Typeflag == tar.TypeReg && path.Base(h.Name) == binary {
				return read(tr)
			}
		}
	}
	return nil, fmt.Errorf("%s was not found in %s", binary, pkgName)
}

func currentExecutable() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	if real, err := filepath.EvalSymlinks(exe); err == nil {
		exe = real
	}
	return exe, nil
}

// installBinary swaps the program file for the new one. Windows cannot
// overwrite a running .exe but can rename it, so the old one is moved aside
// and removed on the next start.
func installBinary(exe string, data []byte) error {
	tmp := exe + ".new"
	if err := os.WriteFile(tmp, data, 0o755); err != nil {
		return permissionHint(err, exe)
	}
	if runtime.GOOS == "windows" {
		old := exe + ".old"
		os.Remove(old)
		if err := os.Rename(exe, old); err != nil {
			os.Remove(tmp)
			return permissionHint(err, exe)
		}
		if err := os.Rename(tmp, exe); err != nil {
			os.Rename(old, exe)
			os.Remove(tmp)
			return permissionHint(err, exe)
		}
		return nil
	}
	if err := os.Rename(tmp, exe); err != nil {
		os.Remove(tmp)
		return permissionHint(err, exe)
	}
	return nil
}

func permissionHint(err error, exe string) error {
	if errors.Is(err, os.ErrPermission) {
		return fmt.Errorf("no permission to replace %s. Run the update with sudo / as administrator, or download it from the releases page", exe)
	}
	return err
}

// cleanupOldBinary removes what a previous Windows update left behind.
func cleanupOldBinary() {
	if exe, err := currentExecutable(); err == nil {
		os.Remove(exe + ".old")
		os.Remove(exe + ".new")
	}
}

// applyUpdate downloads, verifies and installs u over the running program.
func applyUpdate(ctx context.Context, u *updateInfo, showProgress bool) error {
	if u.pkg == nil {
		return errors.New("this release has no verified package for your system")
	}
	exe, err := currentExecutable()
	if err != nil {
		return err
	}
	return applyUpdateTo(ctx, u, exe, showProgress)
}

func applyUpdateTo(ctx context.Context, u *updateInfo, exe string, showProgress bool) error {
	_, binary := packageFor(runtime.GOOS, runtime.GOARCH)
	pkg, err := u.downloadAndVerify(ctx, showProgress)
	if err != nil {
		return err
	}
	bin, err := extractBinary(pkg, u.pkg.Name, binary)
	if err != nil {
		return err
	}
	return installBinary(exe, bin)
}

// ---- Menu side ----

// startUpdateCheck looks for a newer release in the background.
func (a *App) startUpdateCheck() {
	a.updDone = make(chan struct{})
	if os.Getenv("BUZZHEAVIER_NO_UPDATE_CHECK") != "" {
		close(a.updDone)
		return
	}
	go func() {
		defer close(a.updDone)
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		if u, err := checkForUpdate(ctx); err == nil && u != nil {
			a.updMu.Lock()
			a.upd = u
			a.updMu.Unlock()
		}
	}()
}

func (a *App) availableUpdate() *updateInfo {
	a.updMu.Lock()
	defer a.updMu.Unlock()
	return a.upd
}

// waitForUpdateCheck gives the background check a moment so the first menu
// can already show an available update.
func (a *App) waitForUpdateCheck(d time.Duration) {
	select {
	case <-a.updDone:
	case <-time.After(d):
	}
}

func releaseNotesPreview(notes string, maxLines int) []string {
	var out []string
	for _, l := range strings.Split(strings.ReplaceAll(notes, "\r\n", "\n"), "\n") {
		l = strings.TrimSpace(l)
		if l == "" || strings.HasPrefix(l, "|") {
			continue
		}
		l = strings.TrimLeft(l, "#")
		out = append(out, truncate(strings.TrimSpace(strings.ReplaceAll(l, "**", "")), 70))
		if len(out) == maxLines {
			out = append(out, "…")
			break
		}
	}
	return out
}

// updateScreen shows what is new and installs it. With auto set it does not
// ask first. It only returns when the update did not happen.
func (a *App) updateScreen(u *updateInfo, auto bool) {
	header("Update available")
	fmt.Printf("  %s  →  %s\n", dim("v"+version), bold(green("v"+u.version)))
	fmt.Println()
	for _, l := range releaseNotesPreview(u.notes, 12) {
		fmt.Println("  " + dim(l))
	}
	fmt.Println()
	if u.pkg == nil {
		warnf("There is no automatic update for your system in this release.")
		infof("Download it from %s", u.page)
		a.pause()
		return
	}
	if auto {
		infof("Automatic updates are on, installing now…")
	} else if !a.confirm(fmt.Sprintf("Download and install v%s now? (%s)", u.version, humanBytes(u.pkg.Size))) {
		return
	}
	fmt.Println()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	err := applyUpdate(ctx, u, true)
	cancel()
	if err != nil {
		errf("Update failed: %v", err)
		infof("You can also download it from %s", u.page)
		a.pause()
		return
	}
	okf("Updated to v%s. Restarting…", u.version)
	time.Sleep(700 * time.Millisecond)
	if err := restartSelf(); err != nil {
		infof("Please start Buzzheavier Client again to use the new version.")
		a.pause()
		os.Exit(0)
	}
}

func (a *App) checkUpdatesNow() {
	header("Check for updates")
	infof("Asking GitHub for the latest release…")
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	u, err := checkForUpdate(ctx)
	cancel()
	switch {
	case err != nil:
		errf("Could not check for updates: %v", err)
	case u == nil:
		okf("You have the latest version (v%s).", version)
	default:
		a.updMu.Lock()
		a.upd = u
		a.updMu.Unlock()
		a.updateScreen(u, false)
		return
	}
	a.pause()
}

// cliUpdate is "buzzheavier update [--yes]".
func (a *App) cliUpdate(args []string) int {
	yes := len(args) > 0 && (args[0] == "-y" || args[0] == "--yes")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	u, err := checkForUpdate(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Could not check for updates:", err)
		return 1
	}
	if u == nil {
		fmt.Printf("You have the latest version (v%s).\n", version)
		return 0
	}
	fmt.Printf("Update available: v%s → v%s\n", version, u.version)
	if u.pkg == nil {
		fmt.Printf("No automatic update for your system, download it from %s\n", u.page)
		return 1
	}
	if !yes {
		if !stdinIsTTY {
			fmt.Println("Run \"buzzheavier update --yes\" to install it.")
			return 0
		}
		if !a.confirm("Install it now?") {
			return 0
		}
	}
	if err := applyUpdate(ctx, u, stdoutIsTTY); err != nil {
		fmt.Fprintln(os.Stderr, "Update failed:", err)
		return 1
	}
	fmt.Printf("Updated to v%s.\n", u.version)
	return 0
}
