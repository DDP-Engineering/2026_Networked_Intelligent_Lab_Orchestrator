#!/bin/sh
# Build the appliance binaries for the Raspberry Pi (linux/arm64) and for a
# disposable x86-64 machine, with the pinned Go toolchain. Nothing is installed
# or copied anywhere: the result is a directory to hand to the installer.
set -eu
repo=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
out=${1:-$repo/05_Software/core/dist}
cd "$repo/05_Software/core"
for arch in arm64 amd64; do
 mkdir -p "$out/linux-$arch"
 GOOS=linux GOARCH=$arch CGO_ENABLED=0 ../../02_Tools/env/with-go.sh go build -trimpath -mod=readonly -o "$out/linux-$arch/nilo" ./cmd/nilo
 GOOS=linux GOARCH=$arch CGO_ENABLED=0 ../../02_Tools/env/with-go.sh go build -trimpath -mod=readonly -o "$out/linux-$arch/nilo-nethelper" ./cmd/nilo-nethelper
 # Opt-in edition payload. The same bundle can be delivered separately later.
 if [ "${NILO_BUNDLE_SIGROK:-0}" = "1" ]; then
  runtime_root=${NILO_SIGROK_RUNTIME_ROOT:-}
  [ -n "$runtime_root" ] || { echo "NILO_SIGROK_RUNTIME_ROOT is required when bundling Sigrok" >&2; exit 2; }
  ../../02_Tools/env/with-tools.sh python ../../02_Tools/scripts/build-sigrok-bundle.py --out "$out/linux-$arch/integration-bundles/sigrok" --architecture "$arch" --runtime-root "$runtime_root/$arch"
 fi
done
echo "built:"
find "$out" -type f -name 'nilo*' -exec ls -l {} \;
