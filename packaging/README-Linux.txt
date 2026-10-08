Buzzheavier Client for Linux (universal)
https://github.com/uzergit/Buzzheavier-Client

Works on any Linux distribution, on x86_64 (most PCs and servers) and arm64
(Raspberry Pi 4/5 with a 64-bit OS, ARM servers). The binaries are static, so
nothing else needs to be installed.

Start
  ./buzzheavier            interactive menu
  ./buzzheavier file.zip   upload straight away
  ./buzzheavier --help     all command line options

The "buzzheavier" script picks the right binary from bin/ for your CPU. You
can also run bin/buzzheavier-x86_64 or bin/buzzheavier-arm64 directly, or
copy one of them to /usr/local/bin/buzzheavier.

Copying links to the clipboard needs wl-clipboard (Wayland), xclip or xsel.

Settings and upload history are stored in ~/.config/BuzzheavierClient

Updates
  When a new version is out, the main menu shows "u  Update". Settings has
  a switch for automatic updates.
