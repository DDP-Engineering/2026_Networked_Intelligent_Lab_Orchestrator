#!/usr/bin/env bash
set -euo pipefail

repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/../../.." && pwd)
cd "$repo_root"

# The helper validates every project's locked runtime and build inputs before
# clearing any environment. All installations are offline and hash-verified.
exec python 02_Tools/scripts/locked_build_inputs.py prepare-python \
    --repository-root "$repo_root" \
    --lock "$repo_root/02_Tools/containers/build/build-environment.lock.json"
