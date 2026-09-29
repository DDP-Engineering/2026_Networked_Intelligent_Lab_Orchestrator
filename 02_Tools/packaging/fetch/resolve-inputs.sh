#!/bin/sh
set -eu
repo=$(CDPATH= cd -- "$(dirname -- "$0")/../../.." && pwd)
cache=${NILO_PACKAGING_CACHE:-$repo/02_Tools/cache/packaging}
proposal=${1:-$repo/02_Tools/packaging/config/inputs.lock.proposal.json}
exec python3 "$repo/02_Tools/packaging/package_inputs.py" resolve \
  --targets "$repo/02_Tools/packaging/config/targets.json" \
  --declarations "$repo/02_Tools/packaging/config/release-inputs.json" \
  --cache "$cache" \
  --proposal "$proposal"
