// Buzzheavier Client: upload files to buzzheavier.com from the terminal.
// Made by uzer - https://github.com/uzergit/Buzzheavier-Client
package main

import (
	"flag"
	"fmt"
	"os"
	"runtime"
	"strings"
)

const repoURL = "https://github.com/uzergit/Buzzheavier-Client"

// version is set at build time with -ldflags "-X main.version=...".
var version = "2.0.0"

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	initTerminal()
	st, err := loadSettings()
	if err != nil {
		warnf("%v", err)
	}
	app := newApp(st)

	if len(args) == 0 {
		if !stdinIsTTY {
			fmt.Fprintln(os.Stderr, "Run without arguments in a terminal for the menu, or see --help.")
			return 2
		}
		app.run()
		return 0
	}
	switch args[0] {
	case "-h", "--help", "help":
		usage()
		return 0
	case "-v", "--version", "version":
		fmt.Printf("Buzzheavier Client %s (%s/%s)\n", version, runtime.GOOS, runtime.GOARCH)
		return 0
	case "upload":
		return app.cliUpload(args[1:])
	case "locations":
		locs, err := app.client().Locations(ctxAPI())
		if err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			return 1
		}
		for _, l := range locs {
			fmt.Printf("%s  %s\n", l.ID, l.Name)
		}
		return 0
	}
	if strings.HasPrefix(args[0], "-") {
		fmt.Fprintf(os.Stderr, "Unknown option %s\n\n", args[0])
		usage()
		return 2
	}
	// Files dropped onto the executable (or passed as arguments).
	code := app.cliUpload(args)
	if runtime.GOOS == "windows" && stdinIsTTY {
		// Keep the window open long enough to read the link.
		app.ask(dim("Press Enter to close…"))
	}
	return code
}

func usage() {
	fmt.Printf(`Buzzheavier Client %s - %s

Usage:
  buzzheavier                      open the interactive menu
  buzzheavier FILE...              upload files with your quick upload mode
  buzzheavier upload [options] FILE...
  buzzheavier locations            list storage locations
  buzzheavier version

Upload options:
  -a, --account        upload into your account
      --anon           upload anonymously
  -f, --folder ID      account folder to upload into (default: root folder)
  -l, --location ID    storage location ID (see "buzzheavier locations")
  -n, --note TEXT      note shown under the download link
      --no-copy        do not copy the links to the clipboard

The Account ID is read from the settings saved by the menu, or from the
BUZZHEAVIER_ACCOUNT_ID environment variable.
`, version, repoURL)
}

func (a *App) cliUpload(args []string) int {
	fs := flag.NewFlagSet("upload", flag.ContinueOnError)
	fs.Usage = usage
	var account, anon, noCopy bool
	var folder, location, note string
	fs.BoolVar(&account, "account", false, "")
	fs.BoolVar(&account, "a", false, "")
	fs.BoolVar(&anon, "anon", false, "")
	fs.BoolVar(&noCopy, "no-copy", false, "")
	fs.StringVar(&folder, "folder", "", "")
	fs.StringVar(&folder, "f", "", "")
	fs.StringVar(&location, "location", "", "")
	fs.StringVar(&location, "l", "", "")
	fs.StringVar(&note, "note", "", "")
	fs.StringVar(&note, "n", "", "")

	// Allow options before and after the file names.
	var paths []string
	for {
		if err := fs.Parse(args); err != nil {
			return 2
		}
		args = fs.Args()
		if len(args) == 0 {
			break
		}
		paths = append(paths, args[0])
		args = args[1:]
	}
	if len(paths) == 0 {
		fmt.Fprintln(os.Stderr, "No files given. See --help.")
		return 2
	}
	if account && anon {
		fmt.Fprintln(os.Stderr, "Choose either --account or --anon, not both.")
		return 2
	}
	if err := validateNote(note); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}

	mode := a.st.QuickUploadMode()
	switch {
	case account || folder != "":
		mode = modeAccount
	case anon:
		mode = modeAnon
	}
	if mode == modeAccount && a.accountID() == "" {
		fmt.Fprintln(os.Stderr, "No Account ID set. Open the menu (run without arguments) and add it under Settings,")
		fmt.Fprintln(os.Stderr, "or set the BUZZHEAVIER_ACCOUNT_ID environment variable.")
		return 1
	}
	plan := uploadPlan{mode: mode, folderID: folder, locationID: location, note: note}
	if location == "" {
		plan.locationID, plan.locationName = a.st.DefaultLocationID, a.st.DefaultLocationName
	}

	files := collectFiles(paths)
	if len(files) == 0 {
		return 1
	}
	fmt.Println()
	results := a.runUploads(files, plan, stdinIsTTY, !noCopy)
	failed := len(paths) - len(files)
	for _, r := range results {
		if r.err != nil {
			failed++
		}
	}
	if failed > 0 {
		return 1
	}
	return 0
}
