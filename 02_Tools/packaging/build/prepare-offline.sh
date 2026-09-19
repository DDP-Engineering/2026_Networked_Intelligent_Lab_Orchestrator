#!/usr/bin/env sh
set -eu

usage() {
    printf '%s\n' "usage: $0 TARGET OUTPUT [CACHE]" >&2
    exit 2
}

[ "$#" -ge 2 ] && [ "$#" -le 3 ] || usage
target=$1
output=$2
cache=${3:-/packaging-cache}
case "$target" in
    ubuntu-24.04-amd64|rpi-debian13-arm64) ;;
    *) usage ;;
esac
packaging_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)

exec python "$packaging_root/package_inputs.py" \
    prepare \
    --targets "$packaging_root/config/targets.json" \
    --lock "$packaging_root/config/inputs.lock.json" \
    --cache "$cache" \
    --target "$target" \
    --output "$output"
