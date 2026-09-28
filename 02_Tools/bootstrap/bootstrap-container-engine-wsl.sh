#!/usr/bin/env sh
set -eu

# Compatibility entry point retained for existing Windows/WSL2 instructions.
repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
exec sh "$repo_root/02_Tools/bootstrap/bootstrap-container-engine.sh" "$@"
