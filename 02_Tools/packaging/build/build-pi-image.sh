#!/bin/sh
set -eu
[ "$#" -eq 7 ] || { echo "usage: build-pi-image.sh OFFLINE_RECEIPT INPUT_LOCK BASE.img.xz NILO.deb POOL ADMIN_PASSWORD OUTPUT.img.xz" >&2; exit 2; }
repo=$(CDPATH= cd -- "$(dirname -- "$0")/../../.." && pwd)
version=$(tr -d '\r\n' <"$repo/VERSION")
epoch=${SOURCE_DATE_EPOCH:-$(git -C "$repo" log -1 --format=%ct)}
revision=${NILO_SOURCE_REVISION:-$(git -C "$repo" rev-parse HEAD)}
invocation=${NILO_BUILD_INVOCATION_ID:-build-$(date -u +%Y%m%dT%H%M%SZ)-$$}
exec python3 "$repo/02_Tools/packaging/release_builder.py" image \
  --target rpi-debian13-arm64 --offline-receipt "$1" --lock "$2" --base-image "$3" --package "$4" --pool "$5" \
  --admin-password-file "$6" --output "$7" --version "$version" \
  --source-date-epoch "$epoch" --receipt "$7.json" \
  --build-invocation "$invocation" --source-revision "$revision"
