#!/bin/sh
set -eu
[ "$#" -eq 6 ] || { echo "usage: build-release.sh ARTIFACT_DIRECTORY ACCEPTANCE_JSON SOURCE_OFFERS_DIRECTORY UBUNTU_RECEIPT PI_RECEIPT OUTPUT_DIRECTORY" >&2; exit 2; }
repo=$(CDPATH= cd -- "$(dirname -- "$0")/../../.." && pwd)
version=$(tr -d '\r\n' <"$repo/VERSION")
epoch=${SOURCE_DATE_EPOCH:-$(git -C "$repo" log -1 --format=%ct)}
revision=${NILO_SOURCE_REVISION:-$(git -C "$repo" rev-parse HEAD)}
exec python3 "$repo/02_Tools/packaging/release_builder.py" release \
  --artifacts "$1" --acceptance "$2" --source-offers "$3" --offline-receipt "$4" --offline-receipt "$5" --output "$6" --version "$version" --source-date-epoch "$epoch" \
  --lock "$repo/02_Tools/packaging/config/inputs.lock.json" \
  --targets "$repo/02_Tools/packaging/config/targets.json" \
  --source-revision "$revision" --receipt "$6.receipt.json"
