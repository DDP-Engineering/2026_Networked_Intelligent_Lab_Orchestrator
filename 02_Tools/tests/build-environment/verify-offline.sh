#!/usr/bin/env sh
set -eu

repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/../../.." && pwd)
cd "$repo_root"
./02_Tools/tests/build-environment/build-all.sh
(cd 05_Software && python scripts/aegis_test.py lint)
python -m unittest discover -s 05_Software/scripts/tests -p 'test_*.py' -v
find 05_Software -name go.mod -print0 | xargs -0 -n1 sh -c 'root=$(dirname "$0"); (cd "$root" && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go test ./... && CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go test -exec=/bin/true ./...)'
find 05_Software -name uv.lock -print0 | xargs -0 -n1 sh -c 'root=$(dirname "$0"); "$root/${UV_PROJECT_ENVIRONMENT:-.venv-linux-x86_64}/bin/python" -m pytest -q "$root"'
npm --prefix 05_Software/nilo_gui run test:ci
npm --prefix 05_Software/nilo_gui run test:e2e
