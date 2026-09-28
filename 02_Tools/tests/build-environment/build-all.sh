#!/usr/bin/env sh
set -eu

repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/../../.." && pwd)
cd "$repo_root"
python 02_Tools/scripts/locked_build_inputs.py verify --repository-root "$repo_root" \
    --lock "$repo_root/02_Tools/containers/build/build-environment.lock.json"
./02_Tools/tests/build-environment/prepare-python-environments.sh
find 05_Software -name go.mod -print0 | xargs -0 -n1 sh -c 'root=$(dirname "$0"); (cd "$root" && GOPROXY=file:///opt/nilo/inputs/go-proxy GOSUMDB=off go mod download && go mod verify && CGO_ENABLED=0 GOPROXY=off go build ./...)'
npm --prefix 05_Software/frontend ci --offline --ignore-scripts
npm --prefix 05_Software/frontend run build
if [ -n "${NILO_OUTPUT_DIR:-}" ] && [ -d 05_Software/frontend/dist ]; then
    mkdir -p "$NILO_OUTPUT_DIR/nilo_gui"
    cp -R 05_Software/frontend/dist/. "$NILO_OUTPUT_DIR/nilo_gui/"
    printf '%s\n' "published GUI production build to $NILO_OUTPUT_DIR/nilo_gui"
fi
