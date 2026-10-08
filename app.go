package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
)

type App struct {
	in  *bufio.Reader
	st  *Settings
	cl  *Client
	env string // BUZZHEAVIER_ACCOUNT_ID, overrides the saved Account ID

	menu      bool // the interactive menu is running; closed input quits
	inputDone bool // stdin reached its end

	updMu   sync.Mutex
	upd     *updateInfo // newer release found by the startup check
	updDone chan struct{}

	rootID  string
	rootFor string // Account ID that rootID belongs to
	locs    []Location
}

func newApp(st *Settings) *App {
	return &App{
		in:  bufio.NewReader(os.Stdin),
		st:  st,
		cl:  newClient(""),
		env: strings.TrimSpace(os.Getenv("BUZZHEAVIER_ACCOUNT_ID")),
	}
}

func (a *App) accountID() string {
	if a.env != "" {
		return a.env
	}
	return a.st.AccountID
}

func (a *App) client() *Client {
	a.cl.token = a.accountID()
	return a.cl
}

func (a *App) root(ctx context.Context) (string, error) {
	if a.rootID != "" && a.rootFor == a.accountID() {
		return a.rootID, nil
	}
	r, err := a.client().Root(ctx)
	if err != nil {
		return "", err
	}
	a.rootID, a.rootFor = r.ID, a.accountID()
	return a.rootID, nil
}

func (a *App) locations(ctx context.Context) ([]Location, error) {
	if a.locs != nil {
		return a.locs, nil
	}
	locs, err := a.client().Locations(ctx)
	if err == nil {
		a.locs = locs
	}
	return locs, err
}

// ---- Input helpers ----

func (a *App) ask(label string) string {
	fmt.Print("  " + label)
	line, err := a.in.ReadString('\n')
	if err != nil && line == "" {
		fmt.Println()
		if a.menu {
			os.Exit(0) // stdin closed (Ctrl+D / Ctrl+Z)
		}
		a.inputDone = true
	}
	return strings.TrimSpace(line)
}

func (a *App) pause() {
	fmt.Println()
	a.ask(dim("Press Enter to continue…"))
}

func (a *App) confirm(label string) bool {
	in := strings.ToLower(a.ask(label + " [y/N] "))
	return in == "y" || in == "yes"
}

func (a *App) confirmYES(label string) bool {
	return a.ask(label+" Type YES to confirm: ") == "YES"
}

func ctxAPI() context.Context { return context.Background() }

// ---- Main menu ----

func (a *App) run() {
	a.menu = true
	cleanupOldBinary()
	a.startUpdateCheck()
	a.waitForUpdateCheck(1500 * time.Millisecond)
	if u := a.availableUpdate(); u != nil && u.pkg != nil && a.st.AutoUpdate {
		a.updateScreen(u, true)
	}
	if a.st.imported != "" {
		header("Welcome to Buzzheavier Client " + version)
		okf("Imported your Account ID from the v1 settings file:")
		infof("%s", a.st.imported)
		a.pause()
	}
	for {
		header(fmt.Sprintf("Buzzheavier Client  %s", dim("v"+version)))
		menuItem("1", "Anonymous upload")
		menuItem("2", "Upload to my account")
		menuItem("3", "Account & file manager")
		menuItem("4", "Settings")
		menuItem("5", "Upload history")
		menuItem("6", "Quit")
		upd := a.availableUpdate()
		if upd != nil {
			menuItem("u", bold(green("Update to v"+upd.version)), "new version available")
		}
		rule()
		if id := a.accountID(); id != "" {
			fmt.Printf("  Account ID     %s\n", green("set")+" "+dim(maskSecret(id)))
		} else {
			fmt.Printf("  Account ID     %s\n", yellow("not set")+" "+dim("(Settings → 1)"))
		}
		fmt.Printf("  Quick upload   %s\n", a.quickModeLabel())
		fmt.Println()
		infof("Drag files into this window (or paste a path) and press Enter")
		infof("to upload them right away.  %s", repoURL)
		fmt.Println()

		in := a.ask(cyan("› "))
		switch strings.ToLower(in) {
		case "":
		case "1":
			a.uploadFlow(modeAnon)
		case "2":
			a.uploadFlow(modeAccount)
		case "3":
			a.managerMenu()
		case "4":
			a.settingsMenu()
		case "5":
			a.historyScreen()
		case "6", "q", "quit", "exit":
			a.quit()
		case "u", "update":
			if upd != nil {
				a.updateScreen(upd, false)
			} else {
				a.checkUpdatesNow()
			}
		default:
			a.quickUpload(in)
		}
	}
}

func (a *App) quit() {
	clearScreen()
	fmt.Println("  Bye!")
	os.Exit(0)
}

func (a *App) quickModeLabel() string {
	m := a.st.QuickUploadMode()
	label := m
	if m == modeAccount {
		label = "to my account"
	}
	switch {
	case a.st.QuickMode == modeLast:
		label += dim(" (follows last used mode)")
	case a.st.QuickMode == modeAccount && m == modeAnon:
		label += dim(" (account mode needs an Account ID)")
	}
	return label
}

func (a *App) afterUpload() {
	fmt.Println()
	if strings.EqualFold(a.ask(dim("Enter = back to menu · q = quit: ")), "q") {
		a.quit()
	}
}

// ---- Uploading ----

func (a *App) uploadFlow(mode string) {
	title := "Anonymous upload"
	if mode == modeAccount {
		title = "Upload to my account"
	}
	header(title)
	if mode == modeAccount && a.accountID() == "" {
		warnf("No Account ID is set yet.")
		infof("You can find it at https://buzzheavier.com/settings")
		fmt.Println()
		id := a.ask("Paste your Account ID now (Enter = back): ")
		if id == "" || !a.saveAccountID(id) {
			if id != "" {
				a.pause()
			}
			return
		}
		fmt.Println()
	}
	infof("Drag files here, or type/paste their paths. Separate several files with spaces.")
	in := a.ask("File(s): ")
	if in == "" {
		return
	}
	files := collectFiles(splitPaths(in))
	if len(files) == 0 {
		a.pause()
		return
	}
	fmt.Println()
	plan, ok := a.askOptions(mode)
	if !ok {
		return
	}
	header(title)
	a.runUploads(files, plan, true, true)
	a.afterUpload()
}

func (a *App) quickUpload(in string) {
	paths := splitPaths(in)
	anyExists := false
	for _, p := range paths {
		anyExists = anyExists || exists(p)
	}
	header("Quick upload")
	if !anyExists {
		errf("\"%s\" is not a menu option or a file that exists.", truncate(in, 60))
		a.pause()
		return
	}
	files := collectFiles(paths)
	if len(files) == 0 {
		a.pause()
		return
	}
	mode := a.st.QuickUploadMode()
	plan := uploadPlan{mode: mode, locationID: a.st.DefaultLocationID, locationName: a.st.DefaultLocationName}
	if a.st.QuickAsksOptions {
		var ok bool
		if plan, ok = a.askOptions(mode); !ok {
			return
		}
		header("Quick upload")
	}
	a.runUploads(files, plan, true, true)
	a.afterUpload()
}

// askOptions asks for the optional folder, storage location and note.
func (a *App) askOptions(mode string) (uploadPlan, bool) {
	plan := uploadPlan{mode: mode}
	if mode == modeAccount {
		in := a.ask("Folder " + dim("(Enter = root folder, b = browse, or paste a folder ID)") + ": ")
		if strings.EqualFold(in, "b") {
			id, _, ok := a.browse(true)
			if !ok {
				return plan, false
			}
			in = id
		}
		plan.folderID = in
	}
	plan.locationID, plan.locationName = a.pickLocation(false)
	for {
		plan.note = a.ask("Note " + dim("(optional, Enter = none)") + ": ")
		if err := validateNote(plan.note); err != nil {
			errf("%v", err)
			continue
		}
		break
	}
	return plan, true
}

// pickLocation shows the storage locations. forSettings switches the prompt
// from "use the saved default" to "choose a new default".
func (a *App) pickLocation(forSettings bool) (id, name string) {
	locs, err := a.locations(ctxAPI())
	if err != nil {
		warnf("Could not load storage locations: %v", err)
	}
	fmt.Println("  Storage location:")
	for i, l := range locs {
		mark := ""
		if l.ID == a.st.DefaultLocationID {
			mark = "default"
		}
		menuItem(strconv.Itoa(i+1), l.Name, mark)
	}
	def := "server default"
	if a.st.DefaultLocationID != "" {
		def = a.st.DefaultLocationName
		if def == "" {
			def = a.st.DefaultLocationID
		}
	}
	label := "Location " + dim("(Enter = "+def+")") + ": "
	if forSettings {
		label = "Location " + dim("(number, 0 = no preference, Enter = keep current)") + ": "
	}
	for {
		in := a.ask(label)
		if in == "" {
			return a.st.DefaultLocationID, a.st.DefaultLocationName
		}
		if forSettings && in == "0" {
			return "", ""
		}
		if n, err := strconv.Atoi(in); err == nil && n >= 1 && n <= len(locs) {
			return locs[n-1].ID, locs[n-1].Name
		}
		for _, l := range locs {
			if strings.EqualFold(in, l.ID) || strings.EqualFold(in, l.Name) {
				return l.ID, l.Name
			}
		}
		if len(locs) == 0 && !strings.ContainsFunc(in, unicode.IsSpace) {
			return in, "" // offline: trust a pasted location ID
		}
		errf("Pick a number from the list.")
	}
}

// ---- Account & file manager ----

func (a *App) needAccount() bool {
	if a.accountID() != "" {
		return true
	}
	errf("This needs your Account ID. Add it under Settings → 1.")
	a.pause()
	return false
}

func (a *App) managerMenu() {
	for {
		header("Account & file manager")
		menuItem("1", "Browse my files")
		menuItem("2", "Account info")
		menuItem("3", "Storage locations")
		menuItem("4", "Open folder by ID")
		menuItem("5", "Create folder")
		menuItem("6", "Rename file or folder")
		menuItem("7", "Move file or folder")
		menuItem("8", "Change file note")
		menuItem("9", "Delete file or folder")
		menuItem("10", "Back")
		rule()
		if a.accountID() == "" {
			warnf("No Account ID set: only \"Storage locations\" works without one.")
		}
		switch strings.ToLower(a.ask(cyan("› "))) {
		case "1":
			if a.needAccount() {
				a.browse(false)
			}
		case "2":
			if a.needAccount() {
				a.accountInfo()
			}
		case "3":
			a.showLocations()
		case "4":
			if a.needAccount() {
				header("Open folder by ID")
				if id := a.ask("Folder ID: "); id != "" {
					a.browseFrom(id, false)
				}
			}
		case "5":
			if a.needAccount() {
				a.createFolderByID()
			}
		case "6":
			if a.needAccount() {
				header("Rename file or folder")
				if id := a.ask("File or folder ID: "); id != "" {
					a.renameItem(id)
					a.pause()
				}
			}
		case "7":
			if a.needAccount() {
				header("Move file or folder")
				if id := a.ask("File or folder ID: "); id != "" {
					a.moveItem(id)
					a.pause()
				}
			}
		case "8":
			if a.needAccount() {
				header("Change file note")
				if id := a.ask("File ID: "); id != "" {
					a.noteItem(id)
					a.pause()
				}
			}
		case "9":
			if a.needAccount() {
				header("Delete file or folder")
				if id := a.ask("File or folder ID: "); id != "" {
					a.deleteByID(id)
					a.pause()
				}
			}
		case "10", "b", "q":
			return
		}
	}
}

func (a *App) accountInfo() {
	header("Account info")
	acc, err := a.client().Account(ctxAPI())
	if err != nil {
		errf("%v", err)
		a.pause()
		return
	}
	keys := make([]string, 0, len(acc))
	for k := range acc {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Printf("  %s %s\n", pad(humanKey(k), 16), formatValue(k, acc[k]))
	}
	a.pause()
}

func humanKey(k string) string {
	var b strings.Builder
	for i, r := range k {
		if i > 0 && unicode.IsUpper(r) {
			b.WriteRune(' ')
			r = unicode.ToLower(r)
		}
		if i == 0 {
			r = unicode.ToUpper(r)
		}
		b.WriteRune(r)
	}
	s := b.String()
	if s == "Id" {
		return "ID"
	}
	return strings.ReplaceAll(s, " id", " ID")
}

func formatValue(key string, v any) string {
	switch x := v.(type) {
	case nil:
		return dim("-")
	case string:
		if t, err := time.Parse(time.RFC3339Nano, x); err == nil {
			return t.Local().Format("2 Jan 2006 15:04")
		}
		if x == "" {
			return dim("-")
		}
		return x
	case bool:
		if x {
			return "yes"
		}
		return "no"
	case float64:
		if strings.Contains(strings.ToLower(key), "size") || strings.Contains(strings.ToLower(key), "storage") {
			return humanBytes(int64(x))
		}
		return strconv.FormatFloat(x, 'f', -1, 64)
	case []any:
		var names []string
		for _, e := range x {
			if m, ok := e.(map[string]any); ok {
				if n, ok := m["name"].(string); ok {
					names = append(names, n)
					continue
				}
			}
			b, _ := json.Marshal(e)
			names = append(names, string(b))
		}
		if len(names) == 0 {
			return dim("none")
		}
		return strings.Join(names, ", ")
	default:
		b, _ := json.Marshal(x)
		return string(b)
	}
}

func (a *App) showLocations() {
	header("Storage locations")
	locs, err := a.locations(ctxAPI())
	if err != nil {
		errf("%v", err)
		a.pause()
		return
	}
	fmt.Printf("  %s  %s\n", dim(pad("Name", 20)), dim("Location ID"))
	for _, l := range locs {
		mark := ""
		if l.ID == a.st.DefaultLocationID {
			mark = "  " + green("(your default)")
		}
		fmt.Printf("  %s  %s%s\n", pad(l.Name, 20), l.ID, mark)
	}
	a.pause()
}

func (a *App) createFolderByID() {
	header("Create folder")
	parent := a.ask("Parent folder ID " + dim("(Enter = root folder)") + ": ")
	if parent == "" {
		var err error
		if parent, err = a.root(ctxAPI()); err != nil {
			errf("%v", err)
			a.pause()
			return
		}
	}
	a.createFolderIn(parent)
	a.pause()
}

func (a *App) createFolderIn(parentID string) {
	name := sanitizeName(a.ask("New folder name: "))
	if name == "" {
		return
	}
	it, err := a.client().CreateDir(ctxAPI(), parentID, name)
	if err != nil {
		errf("Could not create folder: %v", err)
		return
	}
	if it != nil && it.ID != "" {
		okf("Folder \"%s\" created. ID: %s", name, it.ID)
	} else {
		okf("Folder \"%s\" created.", name)
	}
}

func (a *App) renameItem(id string) bool {
	name := a.ask("New name: ")
	if name == "" {
		return false
	}
	if clean := sanitizeName(name); clean != name {
		infof("Using \"%s\" (Buzzheavier does not allow # ; | \\ / in names).", clean)
		name = clean
	}
	if err := validateName(name); err != nil {
		errf("%v", err)
		return false
	}
	if err := a.client().Rename(ctxAPI(), id, name); err != nil {
		errf("Rename failed: %v", err)
		return false
	}
	okf("Renamed to \"%s\".", name)
	return true
}

func (a *App) moveItem(id string) bool {
	parent := a.ask("Move into folder ID " + dim("(Enter = root folder, b = browse)") + ": ")
	switch {
	case strings.EqualFold(parent, "b"):
		var ok bool
		if parent, _, ok = a.browse(true); !ok {
			return false
		}
	case parent == "":
		var err error
		if parent, err = a.root(ctxAPI()); err != nil {
			errf("%v", err)
			return false
		}
	}
	if err := a.client().Move(ctxAPI(), id, parent); err != nil {
		errf("Move failed: %v", err)
		return false
	}
	okf("Moved.")
	return true
}

func (a *App) noteItem(id string) bool {
	note := a.ask("New note " + dim("(Enter = cancel)") + ": ")
	if note == "" {
		return false
	}
	if err := validateNote(note); err != nil {
		errf("%v", err)
		return false
	}
	if err := a.client().SetNote(ctxAPI(), id, note); err != nil {
		errf("Could not save note: %v", err)
		return false
	}
	okf("Note saved.")
	return true
}

func (a *App) deleteItem(id, what string, folder bool) bool {
	if folder {
		warnf("This permanently deletes %s and everything inside it.", what)
	} else {
		warnf("This permanently deletes %s.", what)
	}
	if !a.confirmYES("") {
		infof("Cancelled.")
		return false
	}
	if err := a.client().Delete(ctxAPI(), id); err != nil {
		errf("Delete failed: %v", err)
		return false
	}
	okf("Deleted.")
	return true
}

func (a *App) deleteByID(id string) {
	it, err := a.client().Get(ctxAPI(), id)
	if err != nil {
		errf("%v", err)
		return
	}
	if root, err := a.root(ctxAPI()); err == nil && id == root {
		errf("The root folder cannot be deleted.")
		return
	}
	a.deleteItem(id, "\""+it.Name+"\"", it.IsDirectory)
}

// ---- File browser ----

type crumb struct{ id, name string }

func (a *App) browse(pick bool) (string, string, bool) {
	root, err := a.root(ctxAPI())
	if err != nil {
		errf("Could not open your files: %v", err)
		a.pause()
		return "", "", false
	}
	return a.browseFrom(root, pick)
}

// browseFrom lets the user walk folders. In pick mode it returns the folder
// the user chose with "u".
func (a *App) browseFrom(startID string, pick bool) (string, string, bool) {
	stack := []crumb{{id: startID}}
	for {
		cur := stack[len(stack)-1]
		dir, err := a.client().Get(ctxAPI(), cur.id)
		if err != nil {
			errf("Could not open folder: %v", err)
			a.pause()
			if len(stack) == 1 {
				return "", "", false
			}
			stack = stack[:len(stack)-1]
			continue
		}
		if dir.ID == "" {
			dir.ID = cur.id
		}
		if !dir.IsDirectory && dir.Name != "" && dir.Children == nil && len(stack) == 1 && cur.id != a.rootID {
			// An ID of a file rather than a folder: show the file instead.
			a.itemActions(dir)
			return "", "", false
		}
		stack[len(stack)-1].name = dir.Name

		title := "My files"
		if pick {
			title = "Choose a folder"
		}
		header(title)
		fmt.Println("  " + bold(breadcrumb(stack, a.rootID)))
		infof("Folder ID: %s", dir.ID)
		rule()

		children := dir.Children
		sort.SliceStable(children, func(i, j int) bool {
			if children[i].IsDirectory != children[j].IsDirectory {
				return children[i].IsDirectory
			}
			return strings.ToLower(children[i].Name) < strings.ToLower(children[j].Name)
		})
		if len(children) == 0 {
			infof("(this folder is empty)")
		}
		for i, c := range children {
			kind, size := "     ", humanBytes(c.Size)
			if c.IsDirectory {
				kind, size = cyan("DIR  "), ""
			}
			fmt.Printf("  %s %s %s %s  %s\n", cyan(fmt.Sprintf("%3d", i+1)), kind,
				pad(truncate(c.Name, 34), 34), dim(pad(size, 9)), dim(c.ID))
		}
		rule()
		hints := []string{"number = open", ".. = up", "n = new folder"}
		if pick {
			hints = append(hints, bold("u = use this folder"))
		} else if len(stack) > 1 || dir.ID != a.rootID {
			hints = append(hints, "i = this folder's actions")
		}
		hints = append(hints, "r = refresh", "b = back")
		infof("%s", strings.Join(hints, " · "))

		in := strings.ToLower(a.ask(cyan("› ")))
		switch {
		case in == "b" || in == "q":
			return "", "", false
		case in == "..":
			if len(stack) > 1 {
				stack = stack[:len(stack)-1]
			} else if dir.ParentID != "" {
				stack[0] = crumb{id: dir.ParentID}
			}
		case in == "n":
			a.createFolderIn(dir.ID)
			a.pause()
		case in == "u" && pick:
			return dir.ID, dir.Name, true
		case in == "i" && !pick:
			if a.itemActions(dir) == actionDeleted {
				if len(stack) == 1 {
					return "", "", false
				}
				stack = stack[:len(stack)-1]
			}
		default:
			n, err := strconv.Atoi(in)
			if err != nil || n < 1 || n > len(children) {
				continue
			}
			c := children[n-1]
			if c.IsDirectory {
				stack = append(stack, crumb{id: c.ID, name: c.Name})
			} else if !pick {
				a.itemActions(&c)
			}
		}
	}
}

func breadcrumb(stack []crumb, rootID string) string {
	parts := []string{}
	for i, c := range stack {
		if i == 0 && c.id == rootID {
			continue
		}
		parts = append(parts, c.name)
	}
	if len(parts) == 0 {
		return "/ (root folder)"
	}
	return "/ " + strings.Join(parts, " / ")
}

type actionResult int

const (
	actionNone actionResult = iota
	actionChanged
	actionDeleted
)

// itemActions shows details for one file or folder and offers what can be
// done with it.
func (a *App) itemActions(it *Item) actionResult {
	result := actionNone
	for {
		kind := "File"
		if it.IsDirectory {
			kind = "Folder"
		}
		header(kind + ": " + truncate(it.Name, 40))
		row := func(k, v string) {
			if v != "" {
				fmt.Printf("  %s %s\n", dim(pad(k, 12)), v)
			}
		}
		row("ID", it.ID)
		if !it.IsDirectory {
			row("Link", cyan(it.Link()))
			row("Size", humanBytes(it.Size))
			row("Downloads", strconv.FormatInt(it.Downloads, 10))
			row("Views", strconv.FormatInt(it.Views, 10))
			if it.Expiry != nil {
				row("Expires", it.Expiry.Local().Format("2 Jan 2006 15:04"))
			}
			row("Note", it.Note)
		}
		if it.CreatedAt != nil {
			row("Created", it.CreatedAt.Local().Format("2 Jan 2006 15:04"))
		}
		rule()
		if !it.IsDirectory {
			menuItem("c", "Copy link")
		}
		menuItem("r", "Rename")
		menuItem("m", "Move")
		if !it.IsDirectory {
			menuItem("n", "Change note")
			menuItem("d", "Delete file")
		} else {
			menuItem("d", "Delete folder")
		}
		menuItem("b", "Back")
		switch strings.ToLower(a.ask(cyan("› "))) {
		case "c":
			if !it.IsDirectory {
				if err := copyToClipboard(it.Link()); err != nil {
					errf("%v", err)
				} else {
					okf("Link copied.")
				}
				a.pause()
			}
		case "r":
			ok := a.renameItem(it.ID)
			a.pause()
			if ok {
				return actionChanged
			}
		case "m":
			ok := a.moveItem(it.ID)
			a.pause()
			if ok {
				return actionChanged
			}
		case "n":
			if !it.IsDirectory && a.noteItem(it.ID) {
				result = actionChanged
			}
			a.pause()
		case "d":
			ok := a.deleteItem(it.ID, "\""+it.Name+"\"", it.IsDirectory)
			a.pause()
			if ok {
				return actionDeleted
			}
		case "b", "q", "":
			return result
		}
	}
}

// ---- Settings ----

func (a *App) saveAccountID(id string) bool {
	id = strings.TrimSpace(id)
	infof("Checking the Account ID with Buzzheavier…")
	_, err := newClient(id).Account(ctxAPI())
	switch {
	case err == nil:
		okf("Account verified.")
	case isUnauthorized(err):
		errf("Buzzheavier does not recognise this Account ID.")
		infof("Copy it again from https://buzzheavier.com/settings")
		return false
	default:
		warnf("Could not verify it right now: %v", err)
		if !a.confirm("Save it anyway?") {
			return false
		}
	}
	a.st.AccountID = id
	a.rootID = ""
	if err := a.st.Save(); err != nil {
		errf("Could not save settings: %v", err)
		return false
	}
	okf("Account ID saved.")
	return true
}

func onOff(b bool) string {
	if b {
		return "on"
	}
	return "off"
}

func (a *App) settingsMenu() {
	for {
		header("Settings")
		acc := "not set"
		if a.st.AccountID != "" {
			acc = "set " + maskSecret(a.st.AccountID)
		}
		loc := "server default"
		if a.st.DefaultLocationID != "" {
			loc = a.st.DefaultLocationName
		}
		qm := map[string]string{modeAnon: "anonymous", modeAccount: "my account", modeLast: "last used"}[a.st.QuickMode]
		menuItem("1", pad("Set / change Account ID", 30), "["+acc+"]")
		menuItem("2", pad("Quick upload mode", 30), "["+qm+"]")
		menuItem("3", pad("Default storage location", 30), "["+loc+"]")
		menuItem("4", pad("Quick upload asks for options", 30), "["+onOff(a.st.QuickAsksOptions)+"]")
		menuItem("5", "Clear Account ID")
		menuItem("6", "Show settings & file locations")
		menuItem("7", pad("Automatic updates", 30), "["+onOff(a.st.AutoUpdate)+"]")
		menuItem("8", "Check for updates")
		menuItem("9", "Reset to defaults")
		menuItem("10", "Back")
		rule()
		if a.env != "" {
			infof("BUZZHEAVIER_ACCOUNT_ID is set and overrides the saved Account ID.")
		}
		switch strings.ToLower(a.ask(cyan("› "))) {
		case "1":
			header("Set Account ID")
			infof("Find it at https://buzzheavier.com/settings")
			if id := a.ask("Account ID (Enter = cancel): "); id != "" {
				a.saveAccountID(id)
				a.pause()
			}
		case "2":
			header("Quick upload mode")
			infof("Used when you drag a file into the main menu.")
			menuItem("1", "Anonymous")
			menuItem("2", "My account")
			menuItem("3", "Whatever I used last")
			switch a.ask(cyan("› ")) {
			case "1":
				a.st.QuickMode = modeAnon
			case "2":
				a.st.QuickMode = modeAccount
			case "3":
				a.st.QuickMode = modeLast
			default:
				continue
			}
			a.saveSettings()
		case "3":
			header("Default storage location")
			a.st.DefaultLocationID, a.st.DefaultLocationName = a.pickLocation(true)
			a.saveSettings()
		case "4":
			a.st.QuickAsksOptions = !a.st.QuickAsksOptions
			a.saveSettings()
		case "5":
			header("Clear Account ID")
			if a.st.AccountID == "" {
				infof("No Account ID is saved.")
			} else if a.confirm("Remove the saved Account ID?") {
				a.st.AccountID, a.rootID = "", ""
				if a.saveSettings() {
					okf("Account ID cleared.")
				}
			}
			a.pause()
		case "6":
			a.showSettings()
		case "7":
			a.st.AutoUpdate = !a.st.AutoUpdate
			a.saveSettings()
		case "8":
			a.checkUpdatesNow()
		case "9":
			a.resetSettings()
		case "10", "b", "q":
			return
		}
	}
}

func (a *App) saveSettings() bool {
	if err := a.st.Save(); err != nil {
		errf("Could not save settings: %v", err)
		a.pause()
		return false
	}
	return true
}

func (a *App) showSettings() {
	header("Current settings")
	acc := dim("not set")
	if a.st.AccountID != "" {
		acc = maskSecret(a.st.AccountID)
	}
	row := func(k, v string) { fmt.Printf("  %s %s\n", dim(pad(k, 20)), v) }
	row("Account ID", acc)
	row("Quick upload", a.quickModeLabel())
	row("Last used mode", a.st.LastMode)
	loc := "server default"
	if a.st.DefaultLocationID != "" {
		loc = a.st.DefaultLocationName + dim(" ("+a.st.DefaultLocationID+")")
	}
	row("Default location", loc)
	row("Quick asks options", onOff(a.st.QuickAsksOptions)+dim(" (folder, location, note)"))
	row("Automatic updates", onOff(a.st.AutoUpdate))
	fmt.Println()
	row("Settings file", a.st.path())
	row("Upload history", a.st.logPath())
	row("Version", version)
	fmt.Println()
	if strings.EqualFold(a.ask(dim("o = open settings folder · Enter = back: ")), "o") {
		if err := openInFileManager(a.st.dir); err != nil {
			errf("%v", err)
			a.pause()
		}
	}
}

func (a *App) resetSettings() {
	header("Reset to defaults")
	warnf("This forgets your Account ID and every setting.")
	infof("Only the client's own settings are touched, nothing else on your computer.")
	if !a.confirmYES("") {
		infof("Cancelled.")
		a.pause()
		return
	}
	if err := os.Remove(a.st.path()); err != nil && !os.IsNotExist(err) {
		errf("Could not remove settings: %v", err)
	}
	dir := a.st.dir
	*a.st = *defaultSettings(dir)
	a.rootID = ""
	okf("Settings reset.")
	if a.confirm("Also delete your upload history?") {
		if err := os.Remove(a.st.logPath()); err != nil && !os.IsNotExist(err) {
			errf("Could not delete history: %v", err)
		} else {
			okf("Upload history deleted.")
		}
	}
	a.pause()
}

func openInFileManager(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", dir)
	case "windows":
		cmd = exec.Command("explorer", dir)
	default:
		cmd = exec.Command("xdg-open", dir)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("could not open %s: %v", dir, err)
	}
	go cmd.Wait()
	return nil
}

// ---- History ----

func (a *App) historyScreen() {
	header("Upload history")
	lines, err := a.st.readLog(30)
	switch {
	case err != nil:
		errf("Could not read history: %v", err)
	case len(lines) == 0:
		infof("Nothing uploaded yet.")
	default:
		for _, l := range lines {
			fmt.Println("  " + l)
		}
		fmt.Println()
		infof("Showing the last %d uploads. Full log: %s", len(lines), a.st.logPath())
	}
	a.pause()
}
