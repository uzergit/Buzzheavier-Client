Buzzheavier Client for macOS
https://github.com/uzergit/Buzzheavier-Client

Pick the right download
  Apple Silicon (M1, M2, M3, M4 ...) -> macOS-AppleSilicon
  Intel Macs                          -> macOS-Intel
  (Apple menu -> About This Mac shows "Chip: Apple ..." on Apple Silicon.)

First start
  The app is not notarised by Apple, so macOS blocks it the first time:
    1. Double-click "buzzheavier". macOS says it cannot be verified -> Done.
    2. Open System Settings -> Privacy & Security, scroll down and click
       "Open Anyway" next to "buzzheavier".
    3. Double-click it again and confirm with "Open".
  Or, in Terminal, run this once inside the unzipped folder:
       xattr -d com.apple.quarantine buzzheavier

  After that, double-clicking "buzzheavier" opens it in Terminal.

Install for the Terminal (optional)
  sudo cp buzzheavier /usr/local/bin/
  Then run "buzzheavier" from anywhere, or "buzzheavier file.zip" to upload.

Uploading to your account
  Copy your Account ID from https://buzzheavier.com/settings and paste it in
  Settings -> 1.

Tip: drag files from Finder into the Terminal window and press Enter to
upload them.

Settings and upload history are stored in
~/Library/Application Support/BuzzheavierClient

Updates
  When a new version is out, the main menu shows "u  Update". Settings has
  a switch for automatic updates.
