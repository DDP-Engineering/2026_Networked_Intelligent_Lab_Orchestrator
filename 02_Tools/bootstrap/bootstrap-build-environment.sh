#!/usr/bin/env sh
set -eu

usage() { printf '%s\n' "usage: $0 [--engine docker|podman] [--install-engine --engine-archive-path DIR [--allow-engine-downgrades]] [--archive-path DIR] [--offline | --rebuild-from-pinned-inputs]" >&2; exit 2; }
engine=docker
install_engine=0
allow_engine_downgrades=0
engine_archive_path=
archive_path=
offline=0
rebuild=0
while [ "$#" -gt 0 ]; do
    case "$1" in
        --engine) engine=${2:-}; shift 2 ;;
        --install-engine) install_engine=1; shift ;;
        --allow-engine-downgrades) allow_engine_downgrades=1; shift ;;
        --engine-archive-path) engine_archive_path=${2:-}; shift 2 ;;
        --archive-path) archive_path=${2:-}; shift 2 ;;
        --offline) offline=1; shift ;;
        --rebuild-from-pinned-inputs) rebuild=1; shift ;;
        *) usage ;;
    esac
done
case "$engine" in docker|podman) ;; *) usage ;; esac
[ "$offline" -eq 0 ] || [ "$rebuild" -eq 0 ] || usage
[ "$install_engine" -eq 0 ] || [ -n "$engine_archive_path" ] || usage
[ -z "$engine_archive_path" ] || [ "$install_engine" -eq 1 ] || usage
[ "$allow_engine_downgrades" -eq 0 ] || [ "$install_engine" -eq 1 ] || usage

repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
os_release_file=${NILO_TEST_OS_RELEASE_FILE:-/etc/os-release}
architecture_probe=${NILO_TEST_ARCHITECTURE_PROBE:-}
systemd_probe=${NILO_TEST_SYSTEMD_PROBE:-}
engine_script=${NILO_TEST_ENGINE_BOOTSTRAP:-$repo_root/02_Tools/bootstrap/bootstrap-container-engine.sh}
image_script=${NILO_TEST_IMAGE_SETUP:-$repo_root/02_Tools/bootstrap/setup-build-environment-image.sh}

if [ -n "$architecture_probe" ]; then architecture=$(sh "$architecture_probe" | tr -d '\r\n'); else architecture=$(uname -m); fi
if [ -n "$systemd_probe" ]; then systemd=$(sh "$systemd_probe" 2>/dev/null | tr -d '\r\n' || true); else systemd=$(systemctl is-system-running 2>/dev/null || true); fi
identity=$(tr -d '\r' < "$os_release_file")
if ! printf '%s\n' "$identity" | grep -Fxq 'ID=ubuntu' || \
   ! printf '%s\n' "$identity" | grep -Eq '^VERSION_ID="?24\.04"?$' || \
   [ "$architecture" != x86_64 ] || \
   { [ "$systemd" != running ] && [ "$systemd" != degraded ]; }; then
    printf '%s\n' "error: native build host must be Ubuntu 24.04 x86_64 with systemd support; architecture='$architecture', systemd='$systemd'" >&2
    exit 1
fi
printf '%s\n' "Native Ubuntu build host verified: Ubuntu 24.04 x86_64; systemd=$systemd"

if [ "$install_engine" -eq 1 ]; then
    if [ "$allow_engine_downgrades" -eq 1 ]; then
        sh "$engine_script" --engine "$engine" --install \
            --archive-path "$engine_archive_path" --allow-downgrades
    else
        sh "$engine_script" --engine "$engine" --install --archive-path "$engine_archive_path"
    fi
fi

set -- --engine "$engine" --engine-verifier "$engine_script"
if [ -n "$archive_path" ]; then set -- "$@" --archive-path "$archive_path"; fi
if [ "$offline" -eq 1 ]; then set -- "$@" --offline; fi
if [ "$rebuild" -eq 1 ]; then set -- "$@" --rebuild-from-pinned-inputs; fi
exec sh "$image_script" "$@"
