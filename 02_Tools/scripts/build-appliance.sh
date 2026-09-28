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
 if [ "${NILO_BUNDLE_CAMERA:-0}" = "1" ]; then
  camera_runtime=${NILO_CAMERA_RUNTIME_ROOT:-}
  camera_wheels=${NILO_CAMERA_WHEELHOUSE_ROOT:-}
  camera_dependencies=${NILO_CAMERA_DEPENDENCY_ROOT:-}
  [ -n "$camera_runtime" ] && [ -n "$camera_wheels" ] && [ -n "$camera_dependencies" ] || {
   echo "Camera bundle requires NILO_CAMERA_RUNTIME_ROOT, NILO_CAMERA_WHEELHOUSE_ROOT and NILO_CAMERA_DEPENDENCY_ROOT" >&2
   exit 2
  }
  ../../02_Tools/env/with-tools.sh python ../../02_Tools/scripts/build-camera-bundle.py \
   --project "$repo/05_Software/integrations/camera" \
   --dependency-root "$camera_dependencies/$arch" --wheelhouse "$camera_wheels/$arch" \
   --out "$out/linux-$arch/integration-bundles/camera" --architecture "$arch" \
   --runtime-root "$camera_runtime/$arch"
 fi
 if [ "${NILO_BUNDLE_SERIAL:-0}" = "1" ]; then
  serial_runtime=${NILO_SERIAL_RUNTIME_ROOT:-}
  serial_wheels=${NILO_SERIAL_WHEELHOUSE:-}
  serial_dependencies=${NILO_SERIAL_DEPENDENCY_ROOT:-}
  [ -n "$serial_runtime" ] && [ -n "$serial_wheels" ] && [ -n "$serial_dependencies" ] || {
   echo "Serial bundle requires NILO_SERIAL_RUNTIME_ROOT, NILO_SERIAL_WHEELHOUSE and NILO_SERIAL_DEPENDENCY_ROOT" >&2
   exit 2
  }
  ../../02_Tools/env/with-tools.sh python ../../02_Tools/scripts/build-serial-bundle.py \
   --project "$repo/05_Software/integrations/serial" \
   --dependency-root "$serial_dependencies/$arch" --wheelhouse "$serial_wheels" \
   --out "$out/linux-$arch/integration-bundles/serial" --architecture "$arch" \
   --runtime-root "$serial_runtime/$arch"
 fi
done
echo "built:"
find "$out" -type f -name 'nilo*' -exec ls -l {} \;
