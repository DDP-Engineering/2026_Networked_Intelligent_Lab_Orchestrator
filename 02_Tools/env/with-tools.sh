#!/usr/bin/env sh
set -eu

tool_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
# shellcheck disable=SC1091
. "$tool_root/toolchain.versions"

runtime_root="$tool_root/runtime"
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

UV_CACHE_DIR="$tool_root/cache/uv"
UV_PYTHON_INSTALL_DIR="$runtime_root/python"
UV_PYTHON_BIN_DIR="$python_bin"
UV_PYTHON_PREFERENCE=only-managed
export UV_CACHE_DIR UV_PYTHON_INSTALL_DIR UV_PYTHON_BIN_DIR UV_PYTHON_PREFERENCE PATH
managed_python=$("$uv_bin/uv" python find "$PYTHON_VERSION")
managed_python_bin=$(dirname -- "$managed_python")
PATH="$go_bin:$uv_bin:$managed_python_bin:$python_bin:$PATH"
export PATH
exec "$@"
