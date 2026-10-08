#!/bin/sh
# Builds the four release packages into dist/:
#   Windows (x64), macOS Intel, macOS Apple Silicon, Linux universal (x86_64 + arm64)
# Usage: scripts/build.sh [version]
set -eu

cd "$(dirname "$0")/.."
VERSION="${1:-$(sed -n 's/^var version = "\(.*\)"/\1/p' main.go)}"
DIST="dist"
NAME="Buzzheavier-Client-v$VERSION"
LDFLAGS="-s -w -X main.version=$VERSION"

rm -rf "$DIST"
mkdir -p "$DIST/stage"

build() { # goos goarch output
	echo "building $1/$2"
	CGO_ENABLED=0 GOOS="$1" GOARCH="$2" go build -trimpath -ldflags "$LDFLAGS" -o "$3" .
}

# Windows
w="$DIST/stage/$NAME-Windows"
mkdir -p "$w"
build windows amd64 "$w/BuzzheavierClient.exe"
cp packaging/README-Windows.txt "$w/README.txt"
(cd "$DIST/stage" && zip -qr9X "../$NAME-Windows-x64.zip" "$NAME-Windows")

# macOS (one package per CPU type)
for arch in amd64:Intel arm64:AppleSilicon; do
	goarch="${arch%%:*}" label="${arch#*:}"
	m="$DIST/stage/$NAME-macOS-$label"
	mkdir -p "$m"
	build darwin "$goarch" "$m/buzzheavier"
	if command -v codesign >/dev/null 2>&1; then
		codesign --force --sign - "$m/buzzheavier"
	fi
	cp packaging/README-macOS.txt "$m/README.txt"
	(cd "$DIST/stage" && zip -qr9X "../$NAME-macOS-$label.zip" "$NAME-macOS-$label")
done

# Linux: static binaries for both CPU types plus a launcher that picks one
l="$DIST/stage/$NAME-Linux-Universal"
mkdir -p "$l/bin"
build linux amd64 "$l/bin/buzzheavier-x86_64"
build linux arm64 "$l/bin/buzzheavier-arm64"
cp packaging/buzzheavier-linux.sh "$l/buzzheavier"
cp packaging/README-Linux.txt "$l/README.txt"
chmod 755 "$l/buzzheavier" "$l/bin/"*
tar -C "$DIST/stage" -czf "$DIST/$NAME-Linux-Universal.tar.gz" "$NAME-Linux-Universal"

rm -rf "$DIST/stage"
(cd "$DIST" && shasum -a 256 *.zip *.tar.gz > SHA256SUMS.txt)
echo
ls -lh "$DIST"
