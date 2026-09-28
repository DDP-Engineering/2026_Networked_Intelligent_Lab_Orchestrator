#!/usr/bin/env sh
set -eu

repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/../../.." && pwd)
cd "$repo_root"
./02_Tools/tests/build-environment/build-all.sh
(cd 05_Software && python ../02_Tools/scripts/aegis_test.py lint)
python -m unittest discover -s 02_Tools/tests/software -p 'test_*.py' -v
find 05_Software -name go.mod -print0 | xargs -0 -n1 sh -c 'root=$(dirname "$0"); (cd "$root" && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go test ./... && CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go test -exec=/bin/true ./...)'
find 05_Software -name uv.lock -print0 | xargs -0 -n1 sh -c 'root=$(dirname "$0"); "$root/${UV_PROJECT_ENVIRONMENT:-.venv-linux-x86_64}/bin/python" -m pytest -q "$root"'
"05_Software/integrations/sdk/python/${UV_PROJECT_ENVIRONMENT:-.venv-linux-x86_64}/bin/python" -m pytest -q 05_Software/tests/contract/test_integration_contract.py
# Keep component-test deadlines reliable on the supported WSL2 host.
npm --prefix 05_Software/frontend run test:ci -- --maxWorkers=2
npm --prefix 05_Software/frontend run build
# The browser suite drives Bench Core on the appliance profile, which needs the
# privileged helper, a real NetworkManager and NET_ADMIN. This container runs
# with no network and dropped capabilities by design, so that suite is the
# appliance verification stage's, run from the host:
#   ./02_Tools/tests/appliance/check.sh
