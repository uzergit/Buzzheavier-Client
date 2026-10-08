#!/bin/sh
# Starts the Buzzheavier Client build that matches this machine's CPU.
here="$(cd "$(dirname "$0")" && pwd)"
case "$(uname -m)" in
	x86_64 | amd64) bin="buzzheavier-x86_64" ;;
	aarch64 | arm64 | armv8*) bin="buzzheavier-arm64" ;;
	*)
		echo "Sorry, $(uname -m) CPUs are not supported (x86_64 and arm64 are)." >&2
		exit 1
		;;
esac
exec "$here/bin/$bin" "$@"
