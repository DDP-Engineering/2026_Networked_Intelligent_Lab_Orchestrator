#!/bin/sh
set -eu
[ "$#" -eq 5 ] || { echo "usage: build-deb.sh TARGET OFFLINE_RECEIPT INPUT_LOCK PAYLOAD.tar.xz OUTPUT.deb" >&2; exit 2; }
repo=$(CDPATH= cd -- "$(dirname -- "$0")/../../.." && pwd)
version=$(tr -d '\r\n' <"$repo/VERSION")
epoch=${SOURCE_DATE_EPOCH:-$(git -C "$repo" log -1 --format=%ct)}
revision=${NILO_SOURCE_REVISION:-$(git -C "$repo" rev-parse HEAD)}
invocation=${NILO_BUILD_INVOCATION_ID:-build-$(date -u +%Y%m%dT%H%M%SZ)-$$}
exec python3 "$repo/02_Tools/packaging/release_builder.py" deb \
  --target "$1" --offline-receipt "$2" --lock "$3" --payload "$4" --output "$5" --version "$version" \
  --source-date-epoch "$epoch" --receipt "$5.json" \
  --build-invocation "$invocation" --source-revision "$revision"
