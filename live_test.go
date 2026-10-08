//go:build live

// Live test against the real Buzzheavier API with a throwaway account:
//
//	BUZZHEAVIER_ACCOUNT_ID=... go test -tags live -run Live -v
//
// It only touches a folder it creates itself and deletes it at the end.
package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

func TestLiveAccount(t *testing.T) {
	token := os.Getenv("BUZZHEAVIER_ACCOUNT_ID")
	if token == "" {
		t.Skip("BUZZHEAVIER_ACCOUNT_ID not set")
	}
	t.Setenv("BUZZHEAVIER_CONFIG_DIR", t.TempDir())
	ctx := ctxAPI()
	c := newClient(token)

	acc, err := c.Account(ctx)
	if err != nil {
		t.Fatalf("account: %v", err)
	}
	t.Logf("account fields: %v", keysOf(acc))

	root, err := c.Root(ctx)
	if err != nil {
		t.Fatalf("root: %v", err)
	}
	t.Logf("root %s with %d children", root.ID, len(root.Children))

	stamp := time.Now().Format("150405")
	dirName := "bhclient-live-" + stamp
	dir, err := c.CreateDir(ctx, root.ID, dirName)
	if err != nil {
		t.Fatalf("create dir: %v", err)
	}
	if dir.ID == "" {
		t.Fatalf("create dir returned no ID")
	}
	t.Logf("created folder %s (%s)", dirName, dir.ID)
	defer func() {
		if err := c.Delete(ctx, dir.ID); err != nil {
			t.Errorf("delete folder: %v", err)
		} else if _, err := c.Get(ctx, dir.ID); err == nil {
			t.Errorf("folder still there after delete")
		} else {
			t.Logf("folder deleted, get now says: %v", err)
		}
	}()

	sub, err := c.CreateDir(ctx, dir.ID, "sub")
	if err != nil {
		t.Fatalf("create subfolder: %v", err)
	}

	// Upload through the same path the menu uses, into the new folder.
	st, _ := loadSettings()
	st.AccountID = token
	a := newApp(st)
	a.in = bufio.NewReader(strings.NewReader(""))
	f := tempFile(t, "live file.txt", "live upload "+stamp)
	res := a.runUploads([]localFile{f}, uploadPlan{mode: modeAccount, folderID: dir.ID, note: "first note"}, false, false)
	if res[0].err != nil {
		t.Fatalf("upload into folder: %v", res[0].err)
	}
	fileID := res[0].item.ID

	got, err := c.Get(ctx, dir.ID)
	if err != nil {
		t.Fatalf("get folder: %v", err)
	}
	if !hasChild(got, fileID) {
		t.Fatalf("uploaded file not listed in folder: %+v", got.Children)
	}

	// Single file lookup (used by the file details screen).
	if fi, err := c.Get(ctx, fileID); err != nil {
		t.Logf("get file by ID: %v", err)
	} else {
		t.Logf("get file by ID: name=%q dir=%v size=%d", fi.Name, fi.IsDirectory, fi.Size)
	}

	must(t, "rename", c.Rename(ctx, fileID, "renamed "+stamp+".txt"))
	must(t, "note", c.SetNote(ctx, fileID, "changed note ✓"))
	must(t, "move", c.Move(ctx, fileID, sub.ID))
	must(t, "rename folder", c.Rename(ctx, sub.ID, "sub-renamed"))

	moved, err := c.Get(ctx, sub.ID)
	if err != nil {
		t.Fatalf("get subfolder: %v", err)
	}
	var item *Item
	for i := range moved.Children {
		if moved.Children[i].ID == fileID {
			item = &moved.Children[i]
		}
	}
	if item == nil {
		t.Fatalf("file not in subfolder after move")
	}
	if item.Name != "renamed "+stamp+".txt" || item.Note != "changed note ✓" {
		t.Errorf("rename/note not applied: %+v", item)
	}
	if moved.Name != "sub-renamed" {
		t.Errorf("folder rename not applied: %q", moved.Name)
	}

	// Name clash inside the account.
	f2 := tempFile(t, "renamed "+stamp+".txt", "dup")
	_, err = c.Upload(ctx, mustOpen(t, f2.path), f2.size, UploadOptions{Name: f2.name, ParentID: sub.ID})
	if !isNameConflict(err) {
		t.Errorf("expected name conflict, got %v", err)
	} else {
		t.Logf("conflict error: %v", err)
	}

	// Does deleting a single file work too?
	if err := c.Delete(ctx, fileID); err != nil {
		t.Logf("deleting a file: %v", err)
	} else {
		t.Logf("deleting a file: ok")
	}

	// Bad folder ID.
	if _, err := c.Get(ctx, "doesnotexist"); err == nil {
		t.Errorf("expected error for unknown folder")
	} else {
		t.Logf("unknown folder: %v", err)
	}
}

func keysOf(m map[string]any) []string {
	var k []string
	for key := range m {
		k = append(k, key)
	}
	return k
}

func hasChild(dir *Item, id string) bool {
	for _, c := range dir.Children {
		if c.ID == id {
			return true
		}
	}
	return false
}

func must(t *testing.T, what string, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("%s: %v", what, err)
	}
}

func mustOpen(t *testing.T, p string) *os.File {
	fh, err := os.Open(p)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { fh.Close() })
	return fh
}

func TestLivePagination(t *testing.T) {
	token := os.Getenv("BUZZHEAVIER_ACCOUNT_ID")
	if token == "" {
		t.Skip("BUZZHEAVIER_ACCOUNT_ID not set")
	}
	ctx := ctxAPI()
	c := newClient(token)
	root, err := c.Root(ctx)
	if err != nil {
		t.Fatal(err)
	}
	dir, err := c.CreateDir(ctx, root.ID, "bhclient-pages-"+time.Now().Format("150405"))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Delete(ctx, dir.ID)

	const n = 130
	sem := make(chan struct{}, 8)
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		sem <- struct{}{}
		go func(i int) {
			defer func() { <-sem }()
			body := fmt.Sprintf("page test %d", i)
			_, err := c.Upload(ctx, strings.NewReader(body), int64(len(body)),
				UploadOptions{Name: fmt.Sprintf("f%03d.txt", i), ParentID: dir.ID})
			errs <- err
		}(i)
	}
	for i := 0; i < n; i++ {
		if err := <-errs; err != nil {
			t.Fatalf("upload: %v", err)
		}
	}

	var first Item
	if err := c.call(ctx, "GET", "/fs/"+dir.ID, nil, &first); err != nil {
		t.Fatal(err)
	}
	t.Logf("first page: %d children, nextCursor=%q", len(first.Children), first.NextCursor)

	full, err := c.Get(ctx, dir.ID)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, ch := range full.Children {
		seen[ch.Name] = true
	}
	if len(full.Children) != n || len(seen) != n {
		t.Fatalf("expected %d children after paging, got %d (%d unique)", n, len(full.Children), len(seen))
	}
	t.Logf("all %d children listed", n)
}
