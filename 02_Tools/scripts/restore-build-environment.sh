#!/usr/bin/env sh
set -eu

usage() { printf '%s\n' "usage: $0 --engine docker|podman --manifest FILE [--archive-path DIR] [--temporary-dir DIR]" >&2; exit 2; }
engine=
manifest=
archive_path=
temporary_dir=${NILO_TMPDIR:-}
while [ "$#" -gt 0 ]; do
    case "$1" in
        --engine) engine=${2:-}; shift 2 ;;
        --manifest) manifest=${2:-}; shift 2 ;;
        --archive-path) archive_path=${2:-}; shift 2 ;;
        --temporary-dir) temporary_dir=${2:-}; shift 2 ;;
        *) usage ;;
    esac
done
[ -n "$engine" ] && [ -n "$manifest" ] || usage
case "$engine" in docker|podman) ;; *) usage ;; esac
[ -n "$archive_path" ] || archive_path=$(dirname -- "$manifest")
[ -n "$temporary_dir" ] || temporary_dir=${TMPDIR:-/tmp}
repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
core="$repo_root/02_Tools/scripts/build_environment.py"
command -v skopeo >/dev/null 2>&1 || { printf '%s\n' "error: skopeo is required for OCI archive import" >&2; exit 1; }

expected=$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1], encoding="utf-8"))["image"]["digest"])' "$manifest")
reference=$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1], encoding="utf-8"))["image"]["reference"])' "$manifest")
# The uncompressed OCI archive is the only full-size temporary this restore
# creates. Manifests generated before the size was recorded fall back to a
# deliberately conservative multiple of the compressed archive.
required=$(python3 -c '
import json, sys
archive = json.load(open(sys.argv[1], encoding="utf-8"))["archive"]
recorded = archive.get("uncompressed_size")
print(int(recorded) if isinstance(recorded, int) and recorded > 0 else int(archive["size"]) * 4)
' "$manifest")

# Fail before any long operation, naming what is required and what is available.
python3 "$core" check-space --path "$temporary_dir" --required-bytes "$required" --purpose "joined OCI archive staging"
if [ "$engine" = docker ]; then
    engine_root=$(docker info --format '{{.DockerRootDir}}' 2>/dev/null || true)
else
    engine_root=$(podman info --format '{{.Store.GraphRoot}}' 2>/dev/null || true)
fi
if [ -n "$engine_root" ] && [ -d "$engine_root" ]; then
    # The daemon owns this directory; the invoking developer only needs to
    # inspect its filesystem capacity, not write into the store directly.
    python3 "$core" check-space --path "$engine_root" --required-bytes "$required" \
        --purpose "$engine image storage" --host-volume advisory --allow-read-only-path
else
    printf '%s\n' "warning: $engine storage root could not be determined; its free space was not checked" >&2
fi

oci_archive=$(mktemp "$temporary_dir/nilo-build-env.XXXXXX.oci.tar")
join_status=$(mktemp "$temporary_dir/nilo-build-env.XXXXXX.status")
# Only this restore's own temporaries are removed. Archive parts and Git LFS
# objects are never touched, including on failure.
trap 'rm -f "$oci_archive" "$join_status"' EXIT HUP INT TERM
# One streamed pass: parts are hashed, the complete archive digest is verified,
# and the bytes are decompressed straight into the single staging file. POSIX sh
# has no pipefail, so the producer's exit status is captured explicitly and a
# failed or corrupt join can never reach the import.
{ python3 "$core" join-archive --manifest "$manifest" --archive-path "$archive_path" --output - \
    || printf '%s' "$?" > "$join_status"; } | gzip -dc > "$oci_archive"
if [ -s "$join_status" ]; then
    printf '%s\n' "error: archive join or digest verification failed (exit $(cat "$join_status")); no image was imported and every archive part is unchanged" >&2
    exit 1
fi
archive_digest=$(skopeo inspect --format '{{.Digest}}' "oci-archive:$oci_archive")
[ "$archive_digest" = "$expected" ] || { printf '%s\n' "error: OCI archive manifest digest mismatch: expected $expected, got $archive_digest" >&2; exit 1; }
if [ "$engine" = docker ]; then transport="docker-daemon:$reference"; else transport="containers-storage:$reference"; fi
skopeo copy --preserve-digests "oci-archive:$oci_archive" "$transport"
actual=$(skopeo inspect --format '{{.Digest}}' "$transport")
[ "$actual" = "$expected" ] || { printf '%s\n' "error: imported image digest mismatch: expected $expected, got $actual" >&2; exit 1; }
"$repo_root/02_Tools/scripts/verify-build-environment.sh" --engine "$engine" --image "$reference" --inventory-only
printf '%s\n' "restored and verified image digest=$actual"
