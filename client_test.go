package main

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

const testToken = "goodtoken123"

// fakeBuzz mimics the behaviour observed on the live service, including the
// quirk that a token without a parent folder results in an anonymous upload.
type fakeBuzz struct {
	mu      sync.Mutex
	folders map[string][]Item // folder ID -> children
	uploads []fakeUpload
}

type fakeUpload struct {
	parent, name, auth, note, location string
	size                               int64
}

func newFakeBuzz(t *testing.T) *fakeBuzz {
	f := &fakeBuzz{folders: map[string][]Item{
		"root123": {{ID: "sub1", Name: "Photos", IsDirectory: true}},
		"sub1":    {},
	}}
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	old := [3]string{siteBase, apiBase, uploadBase}
	siteBase, apiBase, uploadBase = "https://buzzheavier.test", srv.URL+"/api", srv.URL+"/up"
	t.Cleanup(func() { siteBase, apiBase, uploadBase = old[0], old[1], old[2] })
	return f
}

func reply(w http.ResponseWriter, code int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	if code >= 300 {
		json.NewEncoder(w).Encode(map[string]any{"code": code, "error": data})
		return
	}
	json.NewEncoder(w).Encode(map[string]any{"code": code, "data": data})
}

func (f *fakeBuzz) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	authed := r.Header.Get("Authorization") == "Bearer "+testToken
	path := r.URL.EscapedPath()

	if rest, ok := strings.CutPrefix(path, "/up/"); ok && r.Method == http.MethodPut {
		if r.ContentLength <= 0 {
			reply(w, 411, "Length required")
			return
		}
		body, _ := io.ReadAll(r.Body)
		parts := strings.Split(rest, "/")
		var parent, name string
		if len(parts) == 2 {
			parent = parts[0]
			name = parts[1]
		} else {
			name = parts[0]
		}
		name, _ = url.PathUnescape(name)
		note := ""
		if n := r.URL.Query().Get("note"); n != "" {
			b, err := base64.StdEncoding.DecodeString(n)
			if err != nil {
				reply(w, 400, "bad note")
				return
			}
			note = string(b)
		}
		if parent != "" {
			if !authed {
				reply(w, 401, "Unauthorized")
				return
			}
			kids, ok := f.folders[parent]
			if !ok {
				reply(w, 404, "Not found")
				return
			}
			for _, k := range kids {
				if k.Name == name {
					reply(w, 409, "File already exists with the same name")
					return
				}
			}
		}
		id := fmt.Sprintf("file%d", len(f.uploads)+1)
		f.uploads = append(f.uploads, fakeUpload{parent: parent, name: name, auth: r.Header.Get("Authorization"),
			note: note, location: r.URL.Query().Get("locationId"), size: int64(len(body))})
		if parent != "" {
			f.folders[parent] = append(f.folders[parent], Item{ID: id, Name: name})
		}
		reply(w, 201, Item{ID: id, Name: name, Size: int64(len(body)), Note: note})
		return
	}

	switch {
	case strings.HasPrefix(path, "/api/fs/") && r.Method == http.MethodPut:
		reply(w, 405, "Method Not Allowed") // the live API refuses PUT despite the docs
	case path == "/api/locations":
		reply(w, 200, []Location{{ID: "loc1", Name: "Eastern US"}, {ID: "loc2", Name: "Western Europe"}})
	case !authed:
		reply(w, 401, "Unauthorized")
	case path == "/api/account":
		reply(w, 200, map[string]any{"id": "acc1", "createdAt": "2025-01-02T03:04:05Z"})
	case path == "/api/fs" && r.Method == http.MethodGet:
		reply(w, 200, Item{ID: "root123", Name: "root", Children: f.folders["root123"]})
	case strings.HasPrefix(path, "/api/fs/") && r.Method == http.MethodGet:
		id := strings.TrimPrefix(path, "/api/fs/")
		kids, ok := f.folders[id]
		if !ok {
			reply(w, 404, "Not found")
			return
		}
		reply(w, 200, Item{ID: id, Name: id, IsDirectory: true, Children: kids})
	case strings.HasPrefix(path, "/api/fs/") && r.Method == http.MethodPost:
		var body map[string]string
		json.NewDecoder(r.Body).Decode(&body)
		id := "dir" + body["name"]
		f.folders[id] = nil
		reply(w, 201, Item{ID: id, Name: body["name"], IsDirectory: true})
	case strings.HasPrefix(path, "/api/fs/") && (r.Method == http.MethodPatch || r.Method == http.MethodDelete):
		reply(w, 200, nil)
	default:
		reply(w, 405, "Method Not Allowed")
	}
}

func testApp(t *testing.T, token, input string) *App {
	t.Setenv("BUZZHEAVIER_CONFIG_DIR", t.TempDir())
	t.Setenv("BUZZHEAVIER_ACCOUNT_ID", "")
	st, err := loadSettings()
	if err != nil {
		t.Fatal(err)
	}
	st.AccountID = token
	a := newApp(st)
	a.in = bufio.NewReader(strings.NewReader(input))
	return a
}

func tempFile(t *testing.T, name, content string) localFile {
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return localFile{path: p, name: name, size: int64(len(content))}
}

func TestAccountUploadGoesToRootFolder(t *testing.T) {
	fake := newFakeBuzz(t)
	a := testApp(t, testToken, "")
	f := tempFile(t, "report final.pdf", "hello")

	res := a.runUploads([]localFile{f}, uploadPlan{mode: modeAccount, note: "for you ✓"}, false, false)
	if res[0].err != nil {
		t.Fatalf("upload failed: %v", res[0].err)
	}
	if len(fake.uploads) != 1 {
		t.Fatalf("expected one upload, got %d", len(fake.uploads))
	}
	u := fake.uploads[0]
	if u.parent != "root123" {
		t.Errorf("account upload must target the root folder ID, got parent %q", u.parent)
	}
	if u.auth != "Bearer "+testToken {
		t.Errorf("missing auth header, got %q", u.auth)
	}
	if u.name != "report final.pdf" || u.note != "for you ✓" || u.size != 5 {
		t.Errorf("unexpected upload %+v", u)
	}
	if got := res[0].item.Link(); got != "https://buzzheavier.test/file1" {
		t.Errorf("link = %s", got)
	}
	if a.st.LastMode != modeAccount {
		t.Errorf("last mode not remembered")
	}
	if lines, _ := a.st.readLog(10); len(lines) != 1 || !strings.Contains(lines[0], "file1") {
		t.Errorf("history not written: %v", lines)
	}
}

func TestAccountUploadIntoSubfolder(t *testing.T) {
	fake := newFakeBuzz(t)
	a := testApp(t, testToken, "")
	f := tempFile(t, "a.txt", "x")
	res := a.runUploads([]localFile{f}, uploadPlan{mode: modeAccount, folderID: "sub1", locationID: "loc2"}, false, false)
	if res[0].err != nil {
		t.Fatal(res[0].err)
	}
	if u := fake.uploads[0]; u.parent != "sub1" || u.location != "loc2" {
		t.Errorf("unexpected upload %+v", u)
	}
}

func TestAccountUploadBadToken(t *testing.T) {
	fake := newFakeBuzz(t)
	a := testApp(t, "wrong", "")
	f := tempFile(t, "a.txt", "x")
	res := a.runUploads([]localFile{f}, uploadPlan{mode: modeAccount}, false, false)
	if !isUnauthorized(res[0].err) {
		t.Fatalf("expected unauthorized, got %v", res[0].err)
	}
	if len(fake.uploads) != 0 {
		t.Errorf("a rejected token must not fall back to an anonymous upload")
	}
}

func TestAccountUploadUnknownFolder(t *testing.T) {
	fake := newFakeBuzz(t)
	a := testApp(t, testToken, "")
	f := tempFile(t, "a.txt", "x")
	res := a.runUploads([]localFile{f}, uploadPlan{mode: modeAccount, folderID: "nope"}, false, false)
	if res[0].err == nil || len(fake.uploads) != 0 {
		t.Fatalf("expected failure before uploading, got %v", res[0].err)
	}
}

func TestAnonymousUploadSendsNoToken(t *testing.T) {
	fake := newFakeBuzz(t)
	a := testApp(t, testToken, "")
	f := tempFile(t, "a.txt", "x")
	res := a.runUploads([]localFile{f}, uploadPlan{mode: modeAnon}, false, false)
	if res[0].err != nil {
		t.Fatal(res[0].err)
	}
	if u := fake.uploads[0]; u.parent != "" || u.auth != "" {
		t.Errorf("anonymous upload leaked account details: %+v", u)
	}
}

func TestNameConflictPromptsForNewName(t *testing.T) {
	fake := newFakeBuzz(t)
	a := testApp(t, testToken, "\n") // accept the suggested name
	f := tempFile(t, "Photos", "x")
	res := a.runUploads([]localFile{f}, uploadPlan{mode: modeAccount}, true, false)
	if res[0].err != nil {
		t.Fatal(res[0].err)
	}
	if fake.uploads[0].name != "Photos (1)" {
		t.Errorf("expected renamed upload, got %q", fake.uploads[0].name)
	}
}

func TestClientFileManager(t *testing.T) {
	newFakeBuzz(t)
	c := newClient(testToken)
	ctx := ctxAPI()
	root, err := c.Root(ctx)
	if err != nil || root.ID != "root123" || len(root.Children) != 1 {
		t.Fatalf("root: %+v %v", root, err)
	}
	dir, err := c.CreateDir(ctx, root.ID, "New")
	if err != nil || dir.ID != "dirNew" {
		t.Fatalf("create: %+v %v", dir, err)
	}
	for _, err := range []error{
		c.Rename(ctx, "x", "y"), c.Move(ctx, "x", "root123"), c.SetNote(ctx, "x", "n"), c.Delete(ctx, "x"),
	} {
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	}
	if err := c.Move(ctx, "same", "same"); err == nil {
		t.Error("moving a folder into itself should be refused locally")
	}
	if err := c.SetNote(ctx, "x", " "); err == nil {
		t.Error("empty notes should be refused locally")
	}
	if _, err := newClient("bad").Account(ctx); !isUnauthorized(err) {
		t.Errorf("expected unauthorized, got %v", err)
	}
	if locs, err := newClient("").Locations(ctx); err != nil || len(locs) != 2 {
		t.Errorf("locations: %v %v", locs, err)
	}
}

func TestUploadURL(t *testing.T) {
	old := uploadBase
	uploadBase = "https://w.buzzheavier.com"
	defer func() { uploadBase = old }()
	got := uploadURL(UploadOptions{Name: "my file #1?.txt", ParentID: "abc", LocationID: "loc", Note: "hi + bye"})
	want := "https://w.buzzheavier.com/abc/my%20file%20%231%3F.txt?locationId=loc&note=aGkgKyBieWU%3D"
	if got != want {
		t.Errorf("\n got %s\nwant %s", got, want)
	}
}

func TestNumberedName(t *testing.T) {
	for in, want := range map[string]string{
		"photo.jpg":     "photo (1).jpg",
		"photo (1).jpg": "photo (2).jpg",
		"notes":         "notes (1)",
		"a (x).txt":     "a (x) (1).txt",
	} {
		if got := numberedName(in); got != want {
			t.Errorf("numberedName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestDecodeErrors(t *testing.T) {
	rec := httptest.NewRecorder()
	rec.WriteHeader(502)
	rec.WriteString("<html>bad gateway</html>")
	err := decode(rec.Result(), nil)
	if err == nil || strings.Contains(err.Error(), "<html>") {
		t.Errorf("HTML error pages should be hidden, got %v", err)
	}
}

func TestEmptyFileRejected(t *testing.T) {
	_, err := newClient("").Upload(ctxAPI(), strings.NewReader(""), 0, UploadOptions{Name: "a"})
	if err == nil {
		t.Error("expected error for empty file")
	}
}

func TestSanitizeName(t *testing.T) {
	if got := sanitizeName("a#b;c|d\\e/f\tg + ü.txt"); got != "a_b_c_d_e_f_g + ü.txt" {
		t.Errorf("got %q", got)
	}
}
