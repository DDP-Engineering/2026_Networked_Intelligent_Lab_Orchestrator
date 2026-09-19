#!/usr/bin/env sh
set -eu

usage() {
    printf '%s\n' "usage: $0 ubuntu-24.04-amd64|rpi-debian13-arm64 [CACHE]" >&2
    exit 2
}

[ "$#" -ge 1 ] && [ "$#" -le 2 ] || usage
target=$1
case "$target" in
    ubuntu-24.04-amd64|rpi-debian13-arm64) ;;
    *) usage ;;
esac
packaging_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
tools_root=$(CDPATH= cd -- "$packaging_root/.." && pwd)
cache=${2:-$tools_root/cache/packaging}

exec "$tools_root/env/with-tools.sh" python "$packaging_root/package_inputs.py" \
    fetch \
    --targets "$packaging_root/config/targets.json" \
    --lock "$packaging_root/config/inputs.lock.json" \
    --cache "$cache" \
    --target "$target"
