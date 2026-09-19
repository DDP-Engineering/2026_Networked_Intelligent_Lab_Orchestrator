#!/bin/sh
set -eu
[ "$#" -eq 4 ] || { echo "usage: build-deb.sh TARGET OFFLINE_RECEIPT PAYLOAD.tar.xz OUTPUT.deb" >&2; exit 2; }
repo=$(CDPATH= cd -- "$(dirname -- "$0")/../../.." && pwd)
version=$(tr -d '\r\n' <"$repo/VERSION")
epoch=${SOURCE_DATE_EPOCH:-$(git -C "$repo" log -1 --format=%ct)}
exec python3 "$repo/02_Tools/packaging/release_builder.py" deb \
  --target "$1" --offline-receipt "$2" --payload "$3" --output "$4" --version "$version" \
  --source-date-epoch "$epoch" --receipt "$4.json"
