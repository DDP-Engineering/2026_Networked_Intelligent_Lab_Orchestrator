#!/bin/sh
set -eu
[ "$#" -eq 1 ] || { echo "usage: verify-release.sh RELEASE_DIRECTORY" >&2; exit 2; }
repo=$(CDPATH= cd -- "$(dirname -- "$0")/../../.." && pwd)
exec python3 "$repo/02_Tools/packaging/release_builder.py" verify --root "$1" --receipt "$1.verification.json"
