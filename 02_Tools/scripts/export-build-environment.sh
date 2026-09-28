#!/usr/bin/env sh
set -eu

usage() { printf '%s\n' "usage: $0 --engine docker|podman --image REF --generation NAME [--part-size BYTES] [--output DIR]" >&2; exit 2; }
engine=
image=
generation=
part_size=1073741824
output=
while [ "$#" -gt 0 ]; do
    case "$1" in
        --engine) engine=${2:-}; shift 2 ;;
        --image) image=${2:-}; shift 2 ;;
        --generation) generation=${2:-}; shift 2 ;;
        --part-size) part_size=${2:-}; shift 2 ;;
        --output) output=${2:-}; shift 2 ;;
        *) usage ;;
    esac
done
[ -n "$engine" ] && [ -n "$image" ] && [ -n "$generation" ] || usage
case "$engine" in docker|podman) ;; *) usage ;; esac
repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
: "${NILO_BUILDER_ID:=local Ubuntu 24.04 linux/amd64 OCI engine (native or WSL2)}"
export NILO_BUILDER_ID
[ -n "$output" ] || output="$repo_root/02_Tools/archives/build-environment"
lock="$repo_root/02_Tools/containers/build/build-environment.lock.json"
locked_generation=$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1], encoding="utf-8"))["generation"])' "$lock")
[ "$generation" = "$locked_generation" ] || { printf '%s\n' "error: requested generation $generation does not match lock generation $locked_generation" >&2; exit 1; }
source_identity=$(python3 "$repo_root/02_Tools/scripts/build_environment.py" verify-source-state --repository-root "$repo_root")
source_base_commit=$(printf '%s' "$source_identity" | python3 -c 'import json,sys; print(json.load(sys.stdin)["base_commit"])')
source_tree_sha256=$(printf '%s' "$source_identity" | python3 -c 'import json,sys; print(json.load(sys.stdin)["tree_sha256"])')
source_status=$(printf '%s' "$source_identity" | python3 -c 'import json,sys; print(json.load(sys.stdin)["status"])')
command -v skopeo >/dev/null 2>&1 || { printf '%s\n' "error: skopeo is required for OCI archive export" >&2; exit 1; }
if [ "$engine" = docker ]; then transport="docker-daemon:$image"; else transport="containers-storage:$image"; fi
image_source_tree_sha256=$(skopeo inspect "$transport" | python3 -c 'import json,sys; print((json.load(sys.stdin).get("Labels") or {}).get("io.nilo.source-tree.sha256", ""))')
python3 "$repo_root/02_Tools/scripts/build_environment.py" verify-image-source \
    --current-tree-sha256 "$source_tree_sha256" --image-tree-sha256 "$image_source_tree_sha256"
temporary_root=$(mktemp -d "${TMPDIR:-/tmp}/nilo-build-export.XXXXXX")
trap 'rm -rf "$temporary_root"' EXIT HUP INT TERM
metadata="$output/metadata/$generation"
"$repo_root/02_Tools/scripts/collect-build-environment-metadata.sh" --engine "$engine" --image "$image" --output "$metadata"
archive="$temporary_root/nilo-build-env-$generation.oci.tar.gz"
oci_archive="$temporary_root/nilo-build-env-$generation.oci.tar"
SOURCE_DATE_EPOCH=${SOURCE_DATE_EPOCH:-1788566400}
export SOURCE_DATE_EPOCH
skopeo copy --preserve-digests "$transport" "oci-archive:$oci_archive:nilo-build-environment"
digest=$(skopeo inspect --format '{{.Digest}}' "oci-archive:$oci_archive")
case "$digest" in sha256:????????????????????????????????????????????????????????????????) ;; *) printf '%s\n' "error: OCI archive has invalid manifest digest: $digest" >&2; exit 1 ;; esac
uncompressed_size=$(python3 -c 'import os,sys; print(os.path.getsize(sys.argv[1]))' "$oci_archive")
gzip -n -9 -c "$oci_archive" > "$archive"
gzip -t "$archive"
python3 "$repo_root/02_Tools/scripts/build_environment.py" split-archive \
    --archive "$archive" --output "$output" --generation "$generation" \
    --part-size "$part_size" --image-digest "$digest" --lock "$lock" \
    --uncompressed-size "$uncompressed_size" \
    --source-base-commit "$source_base_commit" --source-tree-sha256 "$source_tree_sha256" \
    --source-status "$source_status"
python3 "$repo_root/02_Tools/scripts/build_environment.py" inspect-parts \
    --manifest "$output/current.json" --archive-path "$output"
printf '%s\n' "export complete with pending round-trip status; run verify-archive-round-trip.sh before publication"
