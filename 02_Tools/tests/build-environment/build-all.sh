#!/usr/bin/env sh
set -eu

repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/../../.." && pwd)
cd "$repo_root"
find 05_Software -name go.mod -print0 | xargs -0 -n1 sh -c 'root=$(dirname "$0"); (cd "$root" && CGO_ENABLED=0 go build ./...)'
./02_Tools/tests/build-environment/prepare-python-environments.sh
npm --prefix 05_Software/nilo_gui ci --offline --ignore-scripts
npm --prefix 05_Software/nilo_gui run build
if [ -n "${NILO_OUTPUT_DIR:-}" ] && [ -d 05_Software/nilo_gui/dist ]; then
    mkdir -p "$NILO_OUTPUT_DIR/nilo_gui"
    cp -R 05_Software/nilo_gui/dist/. "$NILO_OUTPUT_DIR/nilo_gui/"
    printf '%s\n' "published GUI production build to $NILO_OUTPUT_DIR/nilo_gui"
fi
