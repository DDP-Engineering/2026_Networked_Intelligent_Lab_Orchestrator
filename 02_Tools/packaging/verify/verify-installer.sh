#!/bin/sh
set -eu
[ "$#" -eq 1 ] || { echo "usage: verify-installer.sh INSTALLER.run" >&2; exit 2; }
[ -f "$1" ] && [ ! -L "$1" ] || { echo "installer must be a regular non-symlink file" >&2; exit 2; }
exec sh "$1" --inspect
