package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
)

type localFile struct {
	path string
	name string
	size int64
}

// uploadPlan is everything decided before the first byte is sent.
type uploadPlan struct {
	mode         string
	folderID     string // account uploads: "" means the account's root folder
	locationID   string
	locationName string
	note         string
}

type uploadResult struct {
	file localFile
	name string // name on Buzzheavier (may differ after a rename)
	item *Item
	err  error
}

// collectFiles checks every path and reports the ones that cannot be uploaded.
func collectFiles(paths []string) []localFile {
	var files []localFile
	seen := map[string]bool{}
	for _, p := range paths {
		abs, err := filepath.Abs(p)
		if err != nil {
			abs = p
		}
		fi, err := os.Stat(abs)
		switch {
		case err != nil:
			errf("Not found: %s", p)
		case fi.IsDir():
			errf("%s is a folder. Upload the files inside it, or zip it first.", fi.Name())
		case !fi.Mode().IsRegular():
			errf("%s is not a regular file.", fi.Name())
		case fi.Size() == 0:
			errf("%s is empty. Buzzheavier does not accept empty files.", fi.Name())
		case seen[abs]:
		default:
			seen[abs] = true
			files = append(files, localFile{path: abs, name: fi.Name(), size: fi.Size()})
		}
	}
	return files
}

// resolveFolder turns the plan's folder into a concrete folder ID. This is
// the fix for account uploads: Buzzheavier only attaches an upload to an
// account when it is sent to /{folderId}/{name}; a token alone is ignored and
// the file silently becomes anonymous.
func (a *App) resolveFolder(ctx context.Context, plan *uploadPlan) (string, error) {
	if plan.folderID == "" {
		root, err := a.root(ctx)
		if err != nil {
			return "", err
		}
		plan.folderID = root
		return "root folder", nil
	}
	dir, err := a.client().Get(ctx, plan.folderID)
	if err != nil {
		return "", fmt.Errorf("folder %s: %w", plan.folderID, err)
	}
	if dir.Name == "" {
		return "folder " + plan.folderID, nil
	}
	return "folder \"" + dir.Name + "\"", nil
}

// runUploads uploads the files one after another and prints a summary.
// interactive enables prompts (e.g. renaming on a name clash).
func (a *App) runUploads(files []localFile, plan uploadPlan, interactive, copyLinks bool) []uploadResult {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	where := "anonymously"
	if plan.mode == modeAccount {
		if a.accountID() == "" {
			errf("No Account ID is set. Add it under Settings first.")
			return failAll(files, errors.New("no Account ID"))
		}
		infof("Checking your account…")
		label, err := a.resolveFolder(ctx, &plan)
		if err != nil {
			errf("Could not open your account: %v", err)
			return failAll(files, err)
		}
		where = "to your account (" + label + ")"
	}
	details := []string{}
	if plan.locationName != "" {
		details = append(details, "location: "+plan.locationName)
	} else if plan.locationID != "" {
		details = append(details, "location: "+plan.locationID)
	}
	if plan.note != "" {
		details = append(details, "note: "+truncate(plan.note, 40))
	}
	fmt.Printf("  Uploading %d file%s %s\n", len(files), plural(len(files)), where)
	if len(details) > 0 {
		infof("%s", strings.Join(details, " · "))
	}
	infof("Press Ctrl+C to cancel.")
	fmt.Println()

	var results []uploadResult
	for i, f := range files {
		if ctx.Err() != nil {
			results = append(results, uploadResult{file: f, err: errCancelled})
			continue
		}
		fmt.Printf("  %s %s %s\n", dim(fmt.Sprintf("[%d/%d]", i+1, len(files))), bold(f.name), dim("("+humanBytes(f.size)+")"))
		r := a.uploadOne(ctx, f, plan, interactive)
		switch {
		case r.err == nil:
			okf("%s", cyan(r.item.Link()))
			if err := a.st.appendLog(r.name, r.item.Link(), plan.mode); err != nil {
				warnf("Could not write upload history: %v", err)
			}
		case errors.Is(r.err, errCancelled):
			warnf("Upload cancelled.")
		case errors.Is(r.err, errSkipped):
			infof("Skipped.")
		default:
			errf("Upload failed: %v", r.err)
		}
		fmt.Println()
		results = append(results, r)
	}

	var links []string
	for _, r := range results {
		if r.err == nil {
			links = append(links, r.item.Link())
		}
	}
	rule()
	switch {
	case len(links) == len(files):
		okf("Done: %d of %d uploaded.", len(links), len(files))
	case len(links) > 0:
		warnf("Done: %d of %d uploaded.", len(links), len(files))
	default:
		errf("Nothing was uploaded.")
	}
	if len(links) > 0 && copyLinks {
		if err := copyToClipboard(strings.Join(links, "\n")); err != nil {
			infof("Could not copy to clipboard: %v", err)
		} else if len(links) == 1 {
			okf("Link copied to clipboard.")
		} else {
			okf("All %d links copied to clipboard.", len(links))
		}
	}
	if len(links) > 0 {
		a.st.LastMode = plan.mode
		if err := a.st.Save(); err != nil {
			warnf("Could not save settings: %v", err)
		}
	}
	return results
}

var errSkipped = errors.New("skipped")

func (a *App) uploadOne(ctx context.Context, f localFile, plan uploadPlan, interactive bool) uploadResult {
	name := sanitizeName(f.name)
	if name != f.name {
		infof("Uploading as \"%s\" (Buzzheavier does not allow # ; | \\ / in names).", name)
	}
	for {
		opts := UploadOptions{Name: name, LocationID: plan.locationID, Note: plan.note}
		if plan.mode == modeAccount {
			opts.ParentID = plan.folderID
		}
		fh, err := os.Open(f.path)
		if err != nil {
			return uploadResult{file: f, name: name, err: err}
		}
		pr := newProgress(fh, f.size)
		it, err := a.client().Upload(ctx, pr, f.size, opts)
		pr.Stop(err == nil)
		fh.Close()
		if err == nil {
			return uploadResult{file: f, name: name, item: it}
		}
		if !interactive || !isNameConflict(err) || ctx.Err() != nil {
			return uploadResult{file: f, name: name, err: err}
		}
		suggestion := numberedName(name)
		warnf("This folder already has a file named \"%s\".", name)
		in := a.ask(fmt.Sprintf("New name (Enter = %s, s = skip): ", suggestion))
		if a.inputDone {
			return uploadResult{file: f, name: name, err: err}
		}
		switch strings.ToLower(in) {
		case "":
			name = suggestion
		case "s":
			return uploadResult{file: f, name: name, err: errSkipped}
		default:
			name = sanitizeName(in)
		}
	}
}

// numberedName turns "photo.jpg" into "photo (1).jpg" and "photo (1).jpg"
// into "photo (2).jpg".
func numberedName(name string) string {
	ext := filepath.Ext(name)
	base := strings.TrimSuffix(name, ext)
	n := 1
	if i := strings.LastIndex(base, " ("); i >= 0 && strings.HasSuffix(base, ")") {
		var k int
		if _, err := fmt.Sscanf(base[i:], " (%d)", &k); err == nil && fmt.Sprintf(" (%d)", k) == base[i:] {
			base, n = base[:i], k+1
		}
	}
	return fmt.Sprintf("%s (%d)%s", base, n, ext)
}

func failAll(files []localFile, err error) []uploadResult {
	out := make([]uploadResult, len(files))
	for i, f := range files {
		out[i] = uploadResult{file: f, err: err}
	}
	return out
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}
