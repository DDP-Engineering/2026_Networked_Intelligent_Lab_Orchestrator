#!/bin/sh
set -eu
[ "$#" -eq 5 ] || { echo "usage: build-payload.sh TARGET OFFLINE_RECEIPT INPUT_LOCK STAGED_ROOT OUTPUT.tar.xz" >&2; exit 2; }
repo=$(CDPATH= cd -- "$(dirname -- "$0")/../../.." && pwd)
version=$(tr -d '\r\n' <"$repo/VERSION")
epoch=${SOURCE_DATE_EPOCH:-$(git -C "$repo" log -1 --format=%ct)}
exec python3 "$repo/02_Tools/packaging/release_builder.py" payload \
  --offline-receipt "$2" --lock "$3" --source "$4" --output "$5" --target "$1" --version "$version" \
  --source-date-epoch "$epoch" --receipt "$5.json"
