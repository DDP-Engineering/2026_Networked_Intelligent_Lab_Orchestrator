#!/usr/bin/env sh
set -eu

tool_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
go_bin="$tool_root/runtime/go/1.27.0/go/bin"

if [ ! -x "$go_bin/go" ]; then
    printf '%s\n' "Repository-local Go 1.27.0 is missing at $go_bin/go. Restore it with the documented bootstrap process." >&2
    exit 1
fi

PATH="$go_bin:$PATH"
export PATH
exec "$@"
