#!/usr/bin/env sh
set -eu

tool_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
# The tracked file contains validated NAME=VALUE records.
# shellcheck disable=SC1091
. "$tool_root/toolchain.versions"
case "$(uname -m)" in
    x86_64|amd64) platform_key=linux-x86_64 ;;
    aarch64|arm64) platform_key=linux-arm64 ;;
    *) printf '%s\n' "with-go.sh supports Linux x86_64 and ARM64; detected $(uname -m)" >&2; exit 1 ;;
esac
go_bin="$tool_root/runtime/$platform_key/go/$GO_VERSION/go/bin"

if [ ! -x "$go_bin/go" ]; then
    printf '%s\n' "Repository-local Go $GO_VERSION is missing at $go_bin/go. Run ./02_Tools/bootstrap/bootstrap.sh." >&2
    exit 1
fi

PATH="$go_bin:$PATH"
export PATH
exec "$@"
