#!/bin/sh
set -eu
[ "$#" -eq 5 ] || { echo "usage: build-offline-installer.sh TARGET OFFLINE_RECEIPT NILO.deb POOL OUTPUT.run" >&2; exit 2; }
repo=$(CDPATH= cd -- "$(dirname -- "$0")/../../.." && pwd)
version=$(tr -d '\r\n' <"$repo/VERSION")
epoch=${SOURCE_DATE_EPOCH:-$(git -C "$repo" log -1 --format=%ct)}
exec python3 "$repo/02_Tools/packaging/release_builder.py" installer \
  --target "$1" --offline-receipt "$2" --package "$3" --pool "$4" --output "$5" \
  --version "$version" --source-date-epoch "$epoch" --receipt "$5.json"
