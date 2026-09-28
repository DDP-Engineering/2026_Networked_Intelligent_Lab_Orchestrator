#!/usr/bin/env sh
set -eu

script_root=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
repository_root=$(CDPATH= cd -- "$script_root/../.." && pwd)
software_root="$repository_root/05_Software"
with_tools="$repository_root/02_Tools/env/with-tools.sh"

"$repository_root/02_Tools/scripts/doctor.sh"

before=$(mktemp)
after=$(mktemp)
trap 'rm -f "$before" "$after"' EXIT HUP INT TERM
git -C "$repository_root" status --porcelain=v1 --untracked-files=all >"$before"

sdk_root="$software_root/integrations/sdk/python"
go_root="$software_root/core"

(cd "$sdk_root" && "$with_tools" uv sync --all-groups --locked)
(cd "$sdk_root" && "$with_tools" uv run pytest -q)
(cd "$sdk_root" && "$with_tools" uv run ruff check .)
(cd "$sdk_root" && "$with_tools" uv run ruff format --check .)
(cd "$software_root" && "$with_tools" uv run --project integrations/sdk/python --all-groups --locked pytest -q tests/contract/test_integration_contract.py)

(cd "$go_root" && "$with_tools" go test ./internal/operation ./internal/integration/supervisor ./internal/acceptance)
(cd "$go_root" && "$with_tools" go test ./...)
(cd "$go_root" && "$with_tools" go vet ./...)

cgo_enabled=$($with_tools go env CGO_ENABLED)
if [ "$cgo_enabled" = "1" ] && "$with_tools" sh -c 'command -v cc >/dev/null 2>&1'; then
    (cd "$go_root" && "$with_tools" go test -race ./...)
else
    printf '%s\n' "Go race tests: SKIPPED (CGO_ENABLED=$cgo_enabled or no C compiler)"
fi

(cd "$software_root" && "$with_tools" sh "$script_root/verify-portable-core.sh")

git -C "$repository_root" status --porcelain=v1 --untracked-files=all >"$after"
if ! diff -u "$before" "$after"; then
    printf '%s\n' 'Increment 1 verification left a repository artifact or changed a tracked file' >&2
    exit 1
fi

printf '%s\n' 'NILO Increment 1 verification: PASS'
