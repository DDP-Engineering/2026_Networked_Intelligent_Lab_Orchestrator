#!/usr/bin/env sh
set -eu

usage() { printf '%s\n' "usage: $0 [--engine docker|podman] [--image REFERENCE] test|build|shell|run [COMMAND ...]" >&2; exit 2; }
engine=${NILO_OCI_ENGINE:-docker}
image_override=
while :; do
    case "${1:-}" in
        --engine) [ "$#" -ge 3 ] || usage; engine=$2; shift 2 ;;
        # An explicit candidate image, for verifying a generation that is not
        # yet accepted into the archive manifest. The caller is then responsible
        # for having verified that image's digest itself.
        --image) [ "$#" -ge 3 ] || usage; image_override=$2; shift 2 ;;
        *) break ;;
    esac
done
[ "$#" -ge 1 ] || usage
action=$1
shift
case "$engine" in docker|podman) ;; *) usage ;; esac
repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
manifest="$repo_root/02_Tools/archives/build-environment/current.json"
reference=$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1], encoding="utf-8"))["image"]["reference"])' "$manifest")
expected=$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1], encoding="utf-8"))["image"]["digest"])' "$manifest")
generation=$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1], encoding="utf-8"))["generation"])' "$manifest")
if [ -n "$image_override" ]; then
    reference=$image_override
    if [ "$engine" = docker ]; then transport="docker-daemon:$reference"; else transport="containers-storage:$reference"; fi
    actual=$(skopeo inspect --format '{{.Digest}}' "$transport" 2>/dev/null || true)
    [ -n "$actual" ] || { printf '%s\n' "error: candidate image $reference is not present in the $engine store" >&2; exit 1; }
    labelled=$("$engine" image inspect --format '{{index .Config.Labels "io.nilo.build-environment.generation"}}' "$reference" 2>/dev/null || true)
    if [ -n "$labelled" ] && [ "$labelled" != "<no value>" ]; then generation=$labelled; fi
    printf '%s\n' "verifying an explicit candidate image, not the accepted current generation"
else
    if [ "$engine" = docker ]; then transport="docker-daemon:$reference"; else transport="containers-storage:$reference"; fi
    actual=$(skopeo inspect --format '{{.Digest}}' "$transport" 2>/dev/null || true)
    [ "$actual" = "$expected" ] || { printf '%s\n' "error: image digest is not accepted: expected $expected, got ${actual:-missing}; run bootstrap-build-environment first" >&2; exit 1; }
fi

uid=$(id -u)
gid=$(id -g)
cache_volume="nilo-build-cache-$generation"
output_root="$repo_root/02_Tools/cache/containers/outputs"
mkdir -p "$output_root"
printf '%s\n' "NILO build environment generation=$generation image digest=$actual"
case "$action" in
    test) command="./02_Tools/tests/build-environment/verify-offline.sh" ;;
    build) command="./02_Tools/tests/build-environment/build-all.sh" ;;
    shell) command="exec bash" ;;
    run) [ "$#" -gt 0 ] || usage; command="$*" ;;
    *) usage ;;
esac

# The checkout is mounted read-only and copied into an image-scoped isolated
# source root. The container therefore tests the developer's current source,
# including uncommitted changes, without ever creating or replacing
# node_modules, platform virtual environments, or build output in the host
# checkout. Declared artifacts are written to /outputs instead.
set -- run --rm --network none --cap-drop all --security-opt no-new-privileges \
    --user "$uid:$gid" -v "$repo_root:/workspace:ro" \
    -v "$repo_root/.git:/work/git-dir:ro" -e GIT_OPTIONAL_LOCKS=0 \
    --mount "type=volume,source=$cache_volume,target=/work/runtime-cache" \
    -v "$output_root:/outputs:rw" -e NILO_OUTPUT_DIR=/outputs \
    -e NILO_ISOLATED_SOURCE=/work/source -w /work \
    --entrypoint sh "$reference" -c '
set -eu
marker="/work/runtime-cache/.nilo-seed-$2"
if [ ! -f "$marker" ]; then
    cp -R /opt/nilo/cache-seed/. /work/runtime-cache/
    : > "$marker"
fi
mkdir -p "$NILO_ISOLATED_SOURCE"
tar -C /workspace -cf - \
    --exclude=./.git \
    --exclude=./02_Tools/cache \
    --exclude=./02_Tools/archives \
    --exclude=./02_Tools/runtime \
    --exclude=./.playwright-cli \
    --exclude=./.pytest_cache \
    --exclude=./.ruff_cache \
    --exclude=node_modules \
    --exclude=.venv \
    --exclude=.venv-\* \
    --exclude=dist \
    --exclude=coverage \
    --exclude=test-results \
    --exclude=playwright-report \
    --exclude=__pycache__ \
    . | tar -C "$NILO_ISOLATED_SOURCE" -xf -
# Git metadata stays read-only and outside the writable copy; optional locks
# are disabled so read-only history queries never try to write an index.
ln -sfn /work/git-dir "$NILO_ISOLATED_SOURCE/.git"
cd "$NILO_ISOLATED_SOURCE"
exec sh -lc "$1"
' sh "$command" "$generation"
"$engine" "$@"
