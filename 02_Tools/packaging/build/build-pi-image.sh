#!/bin/sh
set -eu
[ "$#" -eq 6 ] || { echo "usage: build-pi-image.sh OFFLINE_RECEIPT BASE.img.xz NILO.deb POOL ADMIN_PASSWORD OUTPUT.img.xz" >&2; exit 2; }
repo=$(CDPATH= cd -- "$(dirname -- "$0")/../../.." && pwd)
version=$(tr -d '\r\n' <"$repo/VERSION")
epoch=${SOURCE_DATE_EPOCH:-$(git -C "$repo" log -1 --format=%ct)}
exec python3 "$repo/02_Tools/packaging/release_builder.py" image \
  --target rpi-debian13-arm64 --offline-receipt "$1" --base-image "$2" --package "$3" --pool "$4" \
  --admin-password-file "$5" --output "$6" --version "$version" \
  --source-date-epoch "$epoch" --receipt "$6.json"
