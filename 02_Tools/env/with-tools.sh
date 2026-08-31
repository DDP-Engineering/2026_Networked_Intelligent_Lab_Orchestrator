#!/usr/bin/env sh
set -eu

tool_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
# shellcheck disable=SC1091
. "$tool_root/toolchain.versions"

case "$(uname -m)" in
    x86_64|amd64) platform_key=linux-x86_64 ;;
    aarch64|arm64) platform_key=linux-arm64 ;;
    *) printf '%s\n' "with-tools.sh supports Linux x86_64 and ARM64; detected $(uname -m)" >&2; exit 1 ;;
esac
runtime_root="$tool_root/runtime/$platform_key"
go_bin="$runtime_root/go/$GO_VERSION/go/bin"
uv_bin="$runtime_root/uv/$UV_VERSION"
python_bin="$runtime_root/python-bin"

if [ "$#" -lt 1 ]; then
    printf '%s\n' "pass a command, for example: ./02_Tools/env/with-tools.sh go version" >&2
    exit 2
fi
if [ ! -x "$go_bin/go" ] || [ ! -x "$uv_bin/uv" ]; then
    printf '%s\n' "repository-local tools are missing; run ./02_Tools/bootstrap/bootstrap.sh" >&2
    exit 1
fi

UV_CACHE_DIR="$tool_root/cache/$platform_key/uv"
UV_PYTHON_INSTALL_DIR="$runtime_root/python"
UV_PYTHON_BIN_DIR="$python_bin"
UV_PYTHON_PREFERENCE=only-managed
UV_PROJECT_ENVIRONMENT=".venv-$platform_key"
export UV_CACHE_DIR UV_PYTHON_INSTALL_DIR UV_PYTHON_BIN_DIR UV_PYTHON_PREFERENCE UV_PROJECT_ENVIRONMENT PATH
managed_python=$("$uv_bin/uv" python find "$PYTHON_VERSION")
managed_python_bin=$(dirname -- "$managed_python")
PATH="$go_bin:$uv_bin:$managed_python_bin:$python_bin:$PATH"
export PATH
exec "$@"
