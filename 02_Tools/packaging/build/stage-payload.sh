#!/bin/sh
set -eu
[ "$#" -eq 5 ] || { echo "usage: stage-payload.sh TARGET APPLIANCE_BIN_DIR GUI_DIST BUNDLE_DIR OUTPUT_ROOT" >&2; exit 2; }
target=$1
binaries=$2
gui=$3
bundles=$4
output=$5
case "$target" in ubuntu-24.04-amd64|rpi-debian13-arm64) ;; *) echo "unsupported target: $target" >&2; exit 2;; esac
for path in "$binaries/nilo" "$binaries/nilo-nethelper" "$gui/index.html" "$bundles/sigrok/manifest.json"; do
  [ -f "$path" ] && [ ! -L "$path" ] || { echo "required payload input is missing or unsafe: $path" >&2; exit 2; }
done
[ ! -e "$output" ] || { echo "payload stage already exists: $output" >&2; exit 2; }
mkdir -p "$output/usr/bin" "$output/usr/libexec/nilo" "$output/usr/share/nilo/gui" \
  "$output/usr/lib/nilo/integration-bundles" "$output/etc/nilo"
install -m 0755 "$binaries/nilo" "$output/usr/bin/nilo"
install -m 0755 "$binaries/nilo-nethelper" "$output/usr/libexec/nilo/nilo-nethelper"
cp -R "$gui/." "$output/usr/share/nilo/gui/"
cp -R "$bundles/." "$output/usr/lib/nilo/integration-bundles/"
printf '%s\n' 'NILO_GPIO_RECOVERY=disabled' >"$output/etc/nilo/nilo.env"
