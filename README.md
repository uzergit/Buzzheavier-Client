# Buzzheavier Client

im using Claude code from now on when working on this project 🤑🤑🤑

Free uploader for [buzzheavier.com](https://buzzheavier.com). Upload files anonymously or into your account, manage your files and folders, and get the download link copied to your clipboard. Runs in the terminal on Windows, macOS and Linux.

**I do NOT log anything.** The only thing the client stores is your own settings and upload history, on your own computer.

## Download

Get the latest version from the [Releases page](https://github.com/uzergit/Buzzheavier-Client/releases/latest):

| System | Download |
| --- | --- |
| Windows 10 / 11 | `Buzzheavier-Client-v2.0.0-Windows-x64.zip` |
| macOS, Apple Silicon (M1/M2/M3/M4…) | `Buzzheavier-Client-v2.0.0-macOS-AppleSilicon.zip` |
| macOS, Intel | `Buzzheavier-Client-v2.0.0-macOS-Intel.zip` |
| Linux, any distro (x86_64 and arm64) | `Buzzheavier-Client-v2.0.0-Linux-Universal.tar.gz` |

Every package has a `README.txt` with the first-start steps for that system.

- **Windows:** unzip and double-click `BuzzheavierClient.exe`. If SmartScreen appears, click *More info → Run anyway*. You can also drag files onto the `.exe`.
- **macOS:** unzip and double-click `buzzheavier`. The app isn't notarised by Apple, so the first time you need to go to *System Settings → Privacy & Security → Open Anyway*. Or run `xattr -d com.apple.quarantine buzzheavier` once.
- **Linux:** `tar xzf Buzzheavier-Client-*-Linux-Universal.tar.gz` and run `./buzzheavier`. The launcher picks the right build for your CPU.

## Using it

```
  Buzzheavier Client  v2.0.0
  ────────────────────────────────────────────────────
   1  Anonymous upload
   2  Upload to my account
   3  Account & file manager
   4  Settings
   5  Upload history
   6  Quit
  ────────────────────────────────────────────────────
  Account ID     set abcd…wxyz
  Quick upload   to my account (follows last used mode)

  Drag files into this window (or paste a path) and press Enter
  to upload them right away.
```

- **Quick upload:** drag one or more files into the window and press Enter.
- **Uploading to your account:** get your Account ID from [buzzheavier.com/settings](https://buzzheavier.com/settings) and add it under *Settings → 1*. Files go into your root folder unless you pick another one (type `b` to browse your folders).
- **Optional per upload:** target folder, storage location and a note shown under the download link.
- **Account & file manager:** browse your files and folders, view account info and storage locations, create folders, rename, move, change notes, delete files and folders, and copy file links. Big folders are listed in full.
- **Settings:** Account ID (checked with Buzzheavier before saving), quick upload mode (anonymous / account / last used), default storage location, and whether quick uploads ask for options.
- **Upload history:** every link is saved to `uploads.log`, so you can find it later.
- **Updates:** when a new version is out, the main menu shows **u  Update**. It downloads the right package for your system, checks it against the release checksums, and restarts into the new version. Turn on *Settings → Automatic updates* to have new versions installed for you.

### Command line

```
buzzheavier                         interactive menu
buzzheavier FILE...                 upload with your quick upload mode
buzzheavier upload [options] FILE...
    -a, --account        upload into your account
        --anon           upload anonymously
    -f, --folder ID      account folder (default: root folder)
    -l, --location ID    storage location (see "buzzheavier locations")
    -n, --note TEXT      note shown under the download link
        --no-copy        don't copy links to the clipboard
buzzheavier locations
buzzheavier update [--yes]          install the newest version
buzzheavier --version
```

Scripts can pass the Account ID in the `BUZZHEAVIER_ACCOUNT_ID` environment variable.

### Where settings are stored

| System | Folder |
| --- | --- |
| Windows | `%APPDATA%\BuzzheavierClient` |
| macOS | `~/Library/Application Support/BuzzheavierClient` |
| Linux | `~/.config/BuzzheavierClient` |

If you used version 1 on Windows, your Account ID is imported automatically from the `buzz_settings.ini` that sits next to the `.exe`.

## Notes

- Files on Buzzheavier expire unless they keep getting downloaded. Check [buzzheavier.com](https://buzzheavier.com) for the current rules. Files uploaded to your account show up in your file manager there.
- Buzzheavier doesn't allow `# ; | \ /` in file names, so the client swaps them for `_` and tells you.
- The site has ads. Close any popups and try the download again.

## Building from source

Needs Go 1.22 or newer.

```
go test ./...
scripts/build.sh          # builds all four release packages into dist/
```

The original Windows batch version is kept in [`legacy/`](legacy/).
