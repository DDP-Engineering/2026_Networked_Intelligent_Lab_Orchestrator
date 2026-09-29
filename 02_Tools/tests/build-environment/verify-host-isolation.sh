#!/usr/bin/env sh
set -eu

# Prove that an ordinary container build/test run cannot create or replace the
# developer's own dependency trees in the host checkout. The container mounts
# the checkout read-only and works from an isolated copy, so a run that would
# otherwise execute `npm ci` and create Linux virtual environments must leave
# the host `node_modules` and `.venv-linux-x86_64` directories exactly as it
# found them - including leaving absent ones absent.

usage() { printf '%s\n' "usage: $0 [--engine docker|podman] [--image REFERENCE]" >&2; exit 2; }
engine=docker
image=
while [ "$#" -gt 0 ]; do
    case "$1" in
        --engine) [ "$#" -ge 2 ] || usage; engine=$2; shift 2 ;;
        --image) [ "$#" -ge 2 ] || usage; image=$2; shift 2 ;;
        *) usage ;;
    esac
done
case "$engine" in docker|podman) ;; *) usage ;; esac
repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/../../.." && pwd)
cd "$repo_root"

work=$(mktemp -d "${TMPDIR:-/tmp}/nilo-host-isolation.XXXXXX")
cleanup() {
    if [ -f "$work/sentinels" ]; then
        while IFS= read -r sentinel; do
            [ -n "$sentinel" ] && rm -f "$sentinel"
        done < "$work/sentinels"
    fi
    rm -rf "$work"
}
trap cleanup EXIT HUP INT TERM
sentinel_name=.nilo-host-isolation-sentinel-$$
sentinel_token="host-owned-$$"

{
    printf '%s\n' "05_Software/frontend/node_modules"
    find 05_Software -name pyproject.toml -print | while IFS= read -r project; do
        printf '%s\n' "$(dirname "$project")/.venv-linux-x86_64"
    done
} > "$work/guarded"

fingerprint() {
    while IFS= read -r path; do
        if [ -e "$path" ]; then
            printf '%s present %s\n' "$path" "$(find "$path" -mindepth 1 -maxdepth 1 | wc -l)"
        else
            printf '%s absent\n' "$path"
        fi
    done < "$work/guarded"
}

: > "$work/sentinels"
while IFS= read -r path; do
    if [ -d "$path" ]; then
        [ ! -e "$path/$sentinel_name" ] || {
            printf '%s\n' "error: isolation sentinel already exists: $path/$sentinel_name" >&2
            exit 1
        }
        # Record ownership before creating the file so signal cleanup cannot
        # miss a partially written sentinel.
        printf '%s\n' "$path/$sentinel_name" >> "$work/sentinels"
        printf '%s\n' "$sentinel_token" > "$path/$sentinel_name"
    fi
done < "$work/guarded"

fingerprint > "$work/before"

# Exercise exactly the operations that previously mutated the host checkout.
if [ -n "$image" ]; then
    set -- --engine "$engine" --image "$image"
else
    set -- --engine "$engine"
fi
"$repo_root/02_Tools/env/with-build-environment.sh" "$@" run \
    sh -c 'npm --prefix 05_Software/frontend ci --offline --ignore-scripts && ./02_Tools/tests/build-environment/prepare-python-environments.sh'

fingerprint > "$work/after"

status=0
if ! diff -u "$work/before" "$work/after" >&2; then
    printf '%s\n' "error: the container run changed host dependency trees" >&2
    status=1
fi
while IFS= read -r sentinel; do
    [ -n "$sentinel" ] || continue
    if [ ! -f "$sentinel" ] || [ "$(cat "$sentinel")" != "$sentinel_token" ]; then
        printf '%s\n' "error: host-owned dependency content was replaced: $sentinel" >&2
        status=1
    else
        rm -f "$sentinel"
    fi
done < "$work/sentinels"
[ "$status" -eq 0 ] || exit 1
printf '%s\n' "host isolation verified: node_modules and platform virtual environments are unchanged"
