#!/bin/sh
# Bind one complete artifact build to its builder receipts and trusted inputs.
set -eu
[ "$#" -eq 9 ] || {
  echo 'usage: record-build-set.sh ARTIFACTS OUTPUT LOCK SOURCE_REVISION RECEIPT1 RECEIPT2 RECEIPT3 RECEIPT4 RECEIPT5' >&2
  exit 2
}
artifacts=$1
output=$2
lock=$3
source_revision=$4
repo=$(CDPATH= cd -- "$(dirname -- "$0")/../../.." && pwd)
exec python3 "$repo/02_Tools/packaging/release_builder.py" build-set-receipt \
  --artifacts "$artifacts" --output "$output" --lock "$lock" \
  --source-revision "$source_revision" \
  --artifact-receipt "$5" --artifact-receipt "$6" --artifact-receipt "$7" \
  --artifact-receipt "$8" --artifact-receipt "$9"
