#!/usr/bin/env sh
set -eu

# Canonical host-neutral entry point; the legacy name retains compatibility.
repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
exec sh "$repo_root/02_Tools/scripts/build-wsl-host-bootstrap.sh" "$@"
