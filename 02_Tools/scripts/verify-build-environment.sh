#!/usr/bin/env sh
set -eu

usage() { printf '%s\n' "usage: $0 --engine docker|podman --image REF [--inventory-only]" >&2; exit 2; }
engine=
image=
inventory_only=0
while [ "$#" -gt 0 ]; do
    case "$1" in
        --engine) [ "$#" -ge 2 ] || usage; engine=$2; shift 2 ;;
        --image) [ "$#" -ge 2 ] || usage; image=$2; shift 2 ;;
        --inventory-only) inventory_only=1; shift ;;
        *) usage ;;
    esac
done
[ -n "$engine" ] && [ -n "$image" ] || usage
case "$engine" in docker|podman) ;; *) usage ;; esac

repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
inventory="$repo_root/02_Tools/containers/build/inventory.json"
"$engine" image inspect "$image" >/dev/null
python3 - "$engine" "$image" "$inventory" <<'PY'
import json, subprocess, sys
engine, image, path = sys.argv[1:]
for item in json.load(open(path, encoding="utf-8"))["commands"]:
    result = subprocess.run([engine, "run", "--rm", "--network", "none", image, "sh", "-lc", item["command"]], text=True, capture_output=True)
    output = result.stdout + result.stderr
    if result.returncode or item["contains"] not in output:
        raise SystemExit(f"inventory failure for {item['name']}: {output.strip()}")
    print(f"{item['name']}: {output.strip().splitlines()[0]}")
PY
[ "$inventory_only" -eq 0 ] || exit 0
command -v skopeo >/dev/null 2>&1 || { printf '%s\n' "error: skopeo is required for canonical OCI manifest digests" >&2; exit 1; }

"$engine" run --rm --network none --cap-drop all --security-opt no-new-privileges \
    --entrypoint sh "$image" -lc '
set -eu
tmp=$(mktemp -d)
trap "rm -rf $tmp" EXIT HUP INT TERM
printf "int main(void){return 0;}\n" > "$tmp/smoke.c"
gcc "$tmp/smoke.c" -o "$tmp/native"
file "$tmp/native" | grep -E "x86-64|x86_64"
aarch64-linux-gnu-gcc "$tmp/smoke.c" -o "$tmp/arm64"
file "$tmp/arm64" | grep -E "ARM aarch64|ARM64"
'

# The offline suite runs against a read-only checkout copied into an isolated
# container source root, so verification cannot mutate host dependency trees.
set -- run --rm --network none --cap-drop all --security-opt no-new-privileges \
    --user "$(id -u):$(id -g)" -v "$repo_root:/workspace:ro" \
    -v "$repo_root/.git:/work/git-dir:ro" -e GIT_OPTIONAL_LOCKS=0 \
    -e NILO_ISOLATED_SOURCE=/work/source -w /work
# A directory on the observed WSL backing volume supplies statvfs evidence.
if [ -n "${NILO_WSL_HOST_VOLUME:-}" ]; then
    [ -d "$NILO_WSL_HOST_VOLUME" ] || { printf '%s\n' "error: explicit WSL backing-volume evidence directory is missing: $NILO_WSL_HOST_VOLUME" >&2; exit 1; }
    set -- "$@" -v "$NILO_WSL_HOST_VOLUME:/work/host-volume:ro" -e NILO_WSL_HOST_VOLUME=/work/host-volume
fi
if [ -n "${NILO_KERNEL_MODULES_PATH:-}" ]; then
    [ -d "$NILO_KERNEL_MODULES_PATH" ] || { printf '%s\n' "error: explicit kernel module evidence directory is missing: $NILO_KERNEL_MODULES_PATH" >&2; exit 1; }
    set -- "$@" -v "$NILO_KERNEL_MODULES_PATH:/lib/modules:ro"
fi
"$engine" "$@" \
    --entrypoint bash "$image" -lc '
set -eu
cp -R /opt/nilo/cache-seed/. /work/runtime-cache/
mkdir -p "$NILO_ISOLATED_SOURCE"
tar -C /workspace -cf - \
    --exclude=./.git --exclude=./02_Tools/cache --exclude=./02_Tools/tests/build-environment/evidence.local.json \
    --exclude='*.part[0-9][0-9][0-9]' --exclude='*.oci.tar' --exclude='*.oci.tar.gz' --exclude='nilo-wsl-host-bootstrap-*.tar.gz' \
    --exclude=./02_Tools/runtime --exclude=.playwright-cli \
    --exclude=.pytest_cache --exclude=.ruff_cache \
    --exclude=node_modules --exclude=.venv --exclude=.venv-\* \
    --exclude=dist --exclude=coverage --exclude=test-results \
    --exclude=playwright-report --exclude=__pycache__ \
    . | tar -C "$NILO_ISOLATED_SOURCE" -xf -
# Git does not carry empty directories left by excluded dependency trees.
find "$NILO_ISOLATED_SOURCE" -depth -type d -empty -delete
# Git metadata stays read-only and outside the writable copy; optional locks
# are disabled so read-only history queries never try to write an index.
ln -sfn /work/git-dir "$NILO_ISOLATED_SOURCE/.git"
cd "$NILO_ISOLATED_SOURCE"
./02_Tools/tests/build-environment/verify-offline.sh
'

# Prove from the host side that the container run left the developer's own
# dependency trees untouched.
"$repo_root/02_Tools/tests/build-environment/verify-host-isolation.sh" --engine "$engine" --image "$image"
if [ "$engine" = docker ]; then transport="docker-daemon:$image"; else transport="containers-storage:$image"; fi
digest=$(skopeo inspect --format '{{.Digest}}' "$transport")
printf '%s\n' "build environment verification passed: image digest=$digest"
