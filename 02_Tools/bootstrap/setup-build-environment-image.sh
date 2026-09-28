#!/usr/bin/env sh
set -eu

usage() { printf '%s\n' "usage: $0 [--engine docker|podman] [--engine-verifier SCRIPT] [--archive-path DIR] [--offline | --rebuild-from-pinned-inputs]" >&2; exit 2; }
engine=docker
engine_verifier=
archive_path=
offline=0
rebuild=0
while [ "$#" -gt 0 ]; do
    case "$1" in
        --engine) engine=${2:-}; shift 2 ;;
        --engine-verifier) engine_verifier=${2:-}; shift 2 ;;
        --archive-path) archive_path=${2:-}; shift 2 ;;
        --offline) offline=1; shift ;;
        --rebuild-from-pinned-inputs) rebuild=1; shift ;;
        *) usage ;;
    esac
done
[ "$offline" -eq 0 ] || [ "$rebuild" -eq 0 ] || usage
case "$engine" in docker|podman) ;; *) usage ;; esac

require_image_tools() {
    if [ -n "$engine_verifier" ]; then
        sh "$engine_verifier" --engine "$engine"
        return
    fi
    command -v "$engine" >/dev/null 2>&1 || { printf '%s\n' "error: OCI engine '$engine' is unavailable on the supported Ubuntu host; use the explicit engine-install action" >&2; exit 1; }
    command -v skopeo >/dev/null 2>&1 || { printf '%s\n' "error: skopeo is unavailable; obtain the verified host-bootstrap content and use the explicit engine-install action" >&2; exit 1; }
}

repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
manifest=${NILO_TEST_BUILD_ENVIRONMENT_MANIFEST:-$repo_root/02_Tools/archives/build-environment/current.json}
[ -n "$archive_path" ] || archive_path=$(dirname -- "$manifest")
python3 "$repo_root/02_Tools/scripts/build_environment.py" validate-lock --lock "$repo_root/02_Tools/containers/build/build-environment.lock.json"
python3 "$repo_root/02_Tools/scripts/build_environment.py" validate-manifest --manifest "$manifest"
reference=$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1], encoding="utf-8"))["image"]["reference"])' "$manifest")
expected=$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1], encoding="utf-8"))["image"]["digest"])' "$manifest")
part_count=$(python3 -c 'import json,sys; print(len(json.load(open(sys.argv[1], encoding="utf-8"))["parts"]))' "$manifest")
if command -v "$engine" >/dev/null 2>&1 && command -v skopeo >/dev/null 2>&1; then
    if [ "$engine" = docker ]; then transport="docker-daemon:$reference"; else transport="containers-storage:$reference"; fi
    actual=$(skopeo inspect --format '{{.Digest}}' "$transport" 2>/dev/null || true)
    if [ "$actual" = "$expected" ]; then
        require_image_tools
        "$repo_root/02_Tools/scripts/verify-build-environment.sh" --engine "$engine" --image "$reference" --inventory-only
        printf '%s\n' "build environment already present and verified: image digest=$actual"
        exit 0
    fi
fi

if [ "$part_count" -gt 0 ]; then
    if python3 "$repo_root/02_Tools/scripts/build_environment.py" inspect-parts --manifest "$manifest" --archive-path "$archive_path"; then
        require_image_tools
        exec "$repo_root/02_Tools/scripts/restore-build-environment.sh" --engine "$engine" --manifest "$manifest" --archive-path "$archive_path"
    fi
    if [ "$rebuild" -eq 0 ]; then
        exit 1
    fi
    printf '%s\n' "Local archive defects were reported; continuing with explicitly selected pinned reconstruction." >&2
fi

if [ "$rebuild" -eq 1 ]; then
    printf '%s\n' "Explicit networked reconstruction selected. Only the reviewed base image, Ubuntu snapshot, and lock-listed upstream inputs may be contacted."
    require_image_tools
    exec "$repo_root/02_Tools/scripts/build-build-environment.sh" --engine "$engine"
fi

printf '%s\n' "error: local multipart build-environment content is absent; current generation is pending export" >&2
printf '%s\n' "Recovery path 1: fetch the named LFS objects for the current clone yourself, then rerun setup." >&2
printf '%s\n' "Recovery path 2: obtain the same parts from another repository origin and pass --archive-path for content already obtained." >&2
printf '%s\n' "Recovery path 3: explicitly rebuild from pinned upstream inputs with --rebuild-from-pinned-inputs (network required)." >&2
[ "$offline" -eq 0 ] || printf '%s\n' "Offline mode forbids reconstruction." >&2
exit 1
