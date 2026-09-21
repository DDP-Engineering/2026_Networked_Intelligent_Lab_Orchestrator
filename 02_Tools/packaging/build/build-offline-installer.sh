#!/bin/sh
set -eu
[ "$#" -eq 6 ] || { echo "usage: build-offline-installer.sh TARGET OFFLINE_RECEIPT INPUT_LOCK NILO.deb POOL OUTPUT.run" >&2; exit 2; }
repo=$(CDPATH= cd -- "$(dirname -- "$0")/../../.." && pwd)
version=$(tr -d '\r\n' <"$repo/VERSION")
epoch=${SOURCE_DATE_EPOCH:-$(git -C "$repo" log -1 --format=%ct)}
revision=${NILO_SOURCE_REVISION:-$(git -C "$repo" rev-parse HEAD)}
invocation=${NILO_BUILD_INVOCATION_ID:-build-$(date -u +%Y%m%dT%H%M%SZ)-$$}
exec python3 "$repo/02_Tools/packaging/release_builder.py" installer \
  --target "$1" --offline-receipt "$2" --lock "$3" --package "$4" --pool "$5" --output "$6" \
  --version "$version" --source-date-epoch "$epoch" --receipt "$6.json" \
  --build-invocation "$invocation" --source-revision "$revision"
