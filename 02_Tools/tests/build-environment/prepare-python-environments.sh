#!/usr/bin/env bash
set -euo pipefail

repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/../../.." && pwd)
cd "$repo_root"

projects=()
while IFS= read -r -d '' pyproject; do
    projects+=("$(dirname "$pyproject")")
done < <(find 05_Software -name pyproject.toml -print0)

environment_name=${UV_PROJECT_ENVIRONMENT:-.venv-linux-x86_64}
serial_wheelhouse=${NILO_SERIAL_WHEELHOUSE:-${NILO_OUTPUT_DIR:-$repo_root/02_Tools/cache/containers/outputs}/python-packages}
serial_wheel=$(python 02_Tools/scripts/prepare-serial-dependency.py --output "$serial_wheelhouse" --offline)
for project in "${projects[@]}"; do
    environment="$project/$environment_name"
    uv venv --clear --python /opt/nilo/python-bin/python3 "$environment"
    uv pip install --offline --no-index --python "$environment/bin/python" \
        /opt/nilo/inputs/python-packages/*.whl
    uv pip install --offline --no-index --python "$environment/bin/python" "$serial_wheel"
    for local_project in "${projects[@]}"; do
        uv pip install --offline --no-index --no-deps \
            --python "$environment/bin/python" -e "$local_project"
    done
done
