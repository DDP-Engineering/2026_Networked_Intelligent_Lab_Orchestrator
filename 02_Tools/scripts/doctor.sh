#!/usr/bin/env sh
set -eu

tool_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
# shellcheck disable=SC1091
. "$tool_root/toolchain.versions"
case "$(uname -m)" in
    x86_64|amd64) platform_key=linux-x86_64 ;;
    aarch64|arm64) platform_key=linux-arm64 ;;
    *) printf '%s\n' "doctor.sh supports Linux x86_64 and ARM64; detected $(uname -m)" >&2; exit 1 ;;
esac
runtime_root="$tool_root/runtime/$platform_key"
go_executable="$runtime_root/go/$GO_VERSION/go/bin/go"
uv_executable="$runtime_root/uv/$UV_VERSION/uv"

if [ ! -x "$go_executable" ] || [ ! -x "$uv_executable" ]; then
    printf '%s\n' "repository-local Go or uv is missing" >&2
    exit 1
fi

go_version=$("$go_executable" version)
uv_version=$("$uv_executable" --version)
case "$go_version" in *"go$GO_VERSION"*) ;; *) printf '%s\n' "unexpected Go version: $go_version" >&2; exit 1;; esac
case "$uv_version" in "uv $UV_VERSION"*) ;; *) printf '%s\n' "unexpected uv version: $uv_version" >&2; exit 1;; esac

UV_CACHE_DIR="$tool_root/cache/$platform_key/uv"
UV_PYTHON_INSTALL_DIR="$runtime_root/python"
UV_PYTHON_BIN_DIR="$runtime_root/python-bin"
UV_PYTHON_PREFERENCE=only-managed
UV_PROJECT_ENVIRONMENT=".venv-$platform_key"
export UV_CACHE_DIR UV_PYTHON_INSTALL_DIR UV_PYTHON_BIN_DIR UV_PYTHON_PREFERENCE UV_PROJECT_ENVIRONMENT
python_executable=$("$uv_executable" python find "$PYTHON_VERSION")
python_version=$("$python_executable" --version)
if [ "$python_version" != "Python $PYTHON_VERSION" ]; then
    printf '%s\n' "unexpected Python version: $python_version" >&2
    exit 1
fi

printf '%s\n' "Go: $go_version" "uv: $uv_version" "Python: $python_version" "NILO repository-local toolchain: OK"
