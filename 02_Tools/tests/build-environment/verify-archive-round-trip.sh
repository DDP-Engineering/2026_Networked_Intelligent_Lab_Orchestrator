#!/usr/bin/env sh
set -eu

usage() { printf '%s\n' "usage: $0 --engine docker|podman --manifest FILE [--archive-path DIR]" >&2; exit 2; }
engine=
manifest=
archive_path=
while [ "$#" -gt 0 ]; do
    case "$1" in
        --engine) engine=${2:-}; shift 2 ;;
        --manifest) manifest=${2:-}; shift 2 ;;
        --archive-path) archive_path=${2:-}; shift 2 ;;
        *) usage ;;
    esac
done
[ -n "$engine" ] && [ -n "$manifest" ] || usage
case "$engine" in docker|podman) ;; *) usage ;; esac
[ -n "$archive_path" ] || archive_path=$(dirname -- "$manifest")
repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/../../.." && pwd)
reference=$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1], encoding="utf-8"))["image"]["reference"])' "$manifest")
generation=$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1], encoding="utf-8"))["generation"])' "$manifest")
temporary_log=$(mktemp "${TMPDIR:-/tmp}/nilo-round-trip.XXXXXX.log")
trap 'rm -f "$temporary_log"' EXIT HUP INT TERM

if {
    "$repo_root/02_Tools/scripts/restore-build-environment.sh" --engine "$engine" --manifest "$manifest" --archive-path "$archive_path"
    "$repo_root/02_Tools/scripts/verify-build-environment.sh" --engine "$engine" --image "$reference"
} > "$temporary_log" 2>&1; then
    evidence_dir="$(dirname -- "$manifest")/metadata/$generation"
    mkdir -p "$evidence_dir"
    evidence="$evidence_dir/round-trip.log"
    cp "$temporary_log" "$evidence"
    python3 "$repo_root/02_Tools/scripts/build_environment.py" record-round-trip \
        --manifest "$manifest" --evidence "$evidence" \
        --command "restore-build-environment.sh --engine $engine" \
        --command "verify-build-environment.sh --engine $engine"
    printf '%s\n' "archive import and complete offline verification passed; manifest round-trip status promoted"
else
    cat "$temporary_log" >&2
    printf '%s\n' "error: archive round trip failed; manifest remains pending" >&2
    exit 1
fi
