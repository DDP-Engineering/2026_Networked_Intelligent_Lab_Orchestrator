#!/usr/bin/env sh
set -eu

offline=0
if [ "${1:-}" = "--offline" ]; then
    offline=1
elif [ "$#" -ne 0 ]; then
    printf '%s\n' "usage: $0 [--offline]" >&2
    exit 2
fi

tool_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)

# The tracked file contains only validated NAME=VALUE records.
# shellcheck disable=SC1091
. "$tool_root/toolchain.versions"

case "$(uname -m)" in
    x86_64|amd64)
        architecture=amd64
        platform_key=linux-x86_64
        go_digest=$GO_LINUX_AMD64_SHA256
        uv_target=x86_64-unknown-linux-gnu
        uv_digest=$UV_LINUX_AMD64_SHA256
        ;;
    aarch64|arm64)
        architecture=arm64
        platform_key=linux-arm64
        go_digest=$GO_LINUX_ARM64_SHA256
        uv_target=aarch64-unknown-linux-gnu
        uv_digest=$UV_LINUX_ARM64_SHA256
        ;;
    *)
        printf '%s\n' "bootstrap.sh supports Linux x86_64 and ARM64; detected $(uname -m)" >&2
        exit 1
        ;;
esac

runtime_root="$tool_root/runtime/$platform_key"
cache_root="$tool_root/cache/$platform_key"
download_root="$tool_root/cache/downloads"

mkdir -p "$runtime_root" "$download_root"

verified_archive() {
    url=$1
    destination=$2
    expected=$3
    if [ ! -f "$destination" ]; then
        if [ "$offline" -eq 1 ]; then
            printf '%s\n' "offline bootstrap requires cached archive: $destination" >&2
            exit 1
        fi
        printf '%s\n' "downloading $url"
        curl --fail --location --retry 3 --output "$destination" "$url"
    fi
    printf '%s  %s\n' "$expected" "$destination" | sha256sum -c -
}

go_archive_name="go${GO_VERSION}.linux-${architecture}.tar.gz"
go_archive="$download_root/$go_archive_name"
go_version_root="$runtime_root/go/$GO_VERSION"
go_executable="$go_version_root/go/bin/go"
if [ ! -x "$go_executable" ]; then
    verified_archive "https://go.dev/dl/$go_archive_name" "$go_archive" "$go_digest"
    mkdir -p "$go_version_root"
    staging_root=$(mktemp -d "$runtime_root/bootstrap-go.XXXXXX")
    trap 'rm -rf "$staging_root"' EXIT HUP INT TERM
    tar -xzf "$go_archive" -C "$staging_root"
    mv "$staging_root/go" "$go_version_root/go"
    rm -rf "$staging_root"
    trap - EXIT HUP INT TERM
fi

uv_archive_name="uv-${uv_target}.tar.gz"
uv_archive="$download_root/uv-${UV_VERSION}-${architecture}.tar.gz"
uv_root="$runtime_root/uv/$UV_VERSION"
uv_executable="$uv_root/uv"
if [ ! -x "$uv_executable" ]; then
    verified_archive "https://github.com/astral-sh/uv/releases/download/$UV_VERSION/$uv_archive_name" "$uv_archive" "$uv_digest"
    mkdir -p "$uv_root"
    tar -xzf "$uv_archive" -C "$uv_root" --strip-components=1
fi

UV_CACHE_DIR="$cache_root/uv"
UV_PYTHON_INSTALL_DIR="$runtime_root/python"
UV_PYTHON_BIN_DIR="$runtime_root/python-bin"
UV_PYTHON_PREFERENCE=only-managed
UV_PROJECT_ENVIRONMENT=".venv-$platform_key"
export UV_CACHE_DIR UV_PYTHON_INSTALL_DIR UV_PYTHON_BIN_DIR UV_PYTHON_PREFERENCE UV_PROJECT_ENVIRONMENT
mkdir -p "$UV_CACHE_DIR" "$UV_PYTHON_INSTALL_DIR" "$UV_PYTHON_BIN_DIR"

if [ "$offline" -eq 1 ]; then
    "$uv_executable" --offline python install "$PYTHON_VERSION"
else
    "$uv_executable" python install "$PYTHON_VERSION"
fi

"$tool_root/scripts/doctor.sh"
