#!/bin/sh
# Build one complete five-artifact candidate and emit its invocation receipt.
set -eu
[ "$#" -eq 13 ] || {
  echo 'usage: build-candidate-set.sh OUTPUT SOURCE_ROOT GO_MODULE_CACHE NPM_CACHE UBUNTU_POOL UBUNTU_RECEIPT UBUNTU_SIGROK_RUNTIME PI_POOL PI_RECEIPT PI_SIGROK_RUNTIME BASE_IMAGE ADMIN_PASSWORD INPUT_LOCK' >&2
  exit 2
}
output=$1
source_root=$2
go_module_cache=$3
npm_cache=$4
ubuntu_pool=$5
ubuntu_receipt=$6
ubuntu_sigrok_runtime=$7
pi_pool=$8
pi_receipt=$9
pi_sigrok_runtime=${10}
base_image=${11}
admin_password=${12}
input_lock=${13}
case "$output" in /*) ;; *) echo 'candidate output must be an absolute path' >&2; exit 2 ;; esac
[ ! -e "$output" ] || { echo "candidate output already exists: $output" >&2; exit 2; }

repo=$(CDPATH= cd -- "$(dirname -- "$0")/../../.." && pwd)
builder="$repo/02_Tools/packaging/release_builder.py"
case "$source_root" in /*) ;; *) echo 'candidate source root must be absolute' >&2; exit 2 ;; esac
[ -e "$source_root/.git" ] && [ ! -L "$source_root" ] || { echo 'candidate source root is not a Git checkout' >&2; exit 2; }
epoch=${SOURCE_DATE_EPOCH:-$(git -C "$source_root" log -1 --format=%ct)}
revision=${NILO_SOURCE_REVISION:-$(git -C "$source_root" rev-parse HEAD)}
git -C "$source_root" cat-file -e "$revision^{commit}"
invocation="build-$(date -u +%Y%m%dT%H%M%SZ)-$$-$(basename -- "$output")"
receipts="${output}.artifact-receipts"
work="${output}.source-build"
[ ! -e "$receipts" ] || { echo "candidate receipt directory already exists: $receipts" >&2; exit 2; }
[ ! -e "$work" ] || { echo "candidate source-build root already exists: $work" >&2; exit 2; }
mkdir -p "$output" "$receipts" "$work/source" "$work/bin/amd64" "$work/bin/arm64" \
  "$work/bundles/amd64" "$work/bundles/arm64"

# Build every source-derived layer from the exact identified revision. The
# checkout supplies pinned tools; source bytes come only from the Git archive.
git -C "$source_root" archive --format=tar "$revision" | tar -xf - -C "$work/source"
version=$(tr -d '\r\n' <"$work/source/VERSION")
# The tool identities come from the archived revision; executable lookup is
# owned by the already-verified offline build environment, never by a mutable
# wrapper in the caller-supplied checkout.
# shellcheck disable=SC1091
. "$work/source/02_Tools/toolchain.versions"
go_command=$(command -v go)
node_command=$(command -v node)
npm_command=$(command -v npm)
case "$($go_command version)" in *" go$GO_VERSION "*) ;; *) echo 'offline Go version mismatch' >&2; exit 2 ;; esac
[ "$($node_command --version)" = "v$NODE_VERSION" ] || { echo 'offline Node.js version mismatch' >&2; exit 2; }
[ "$($npm_command --version)" = "$NPM_VERSION" ] || { echo 'offline npm version mismatch' >&2; exit 2; }
for arch in amd64 arm64; do
  (
    cd "$work/source/05_Software/nilo_assistant"
    GOOS=linux GOARCH=$arch CGO_ENABLED=0 GOPROXY=off GOMODCACHE="$go_module_cache" \
      GOCACHE="$work/go-cache-$arch" "$go_command" build \
      -trimpath -mod=readonly -o "$work/bin/$arch/nilo" ./cmd/nilo
    GOOS=linux GOARCH=$arch CGO_ENABLED=0 GOPROXY=off GOMODCACHE="$go_module_cache" \
      GOCACHE="$work/go-cache-$arch" "$go_command" build \
      -trimpath -mod=readonly -o "$work/bin/$arch/nilo-nethelper" ./cmd/nilo-nethelper
  )
done
gui_source="$work/source/05_Software/nilo_gui"
(
  cd "$gui_source"
  npm_config_cache="$npm_cache" "$npm_command" ci --offline --ignore-scripts
  ./node_modules/.bin/tsc -b
  ./node_modules/.bin/vite build --outDir "$work/gui" --emptyOutDir
)
python3 "$work/source/02_Tools/scripts/build-sigrok-bundle.py" \
  --out "$work/bundles/amd64/sigrok" --architecture amd64 --runtime-root "$ubuntu_sigrok_runtime"
python3 "$work/source/02_Tools/scripts/build-sigrok-bundle.py" \
  --out "$work/bundles/arm64/sigrok" --architecture arm64 --runtime-root "$pi_sigrok_runtime"
sh "$work/source/02_Tools/packaging/build/stage-payload.sh" ubuntu-24.04-amd64 \
  "$work/bin/amd64" "$work/gui" "$work/bundles/amd64" "$work/stage-amd64"
sh "$work/source/02_Tools/packaging/build/stage-payload.sh" rpi-debian13-arm64 \
  "$work/bin/arm64" "$work/gui" "$work/bundles/arm64" "$work/stage-arm64"
ubuntu_payload="$work/ubuntu-payload.tar.xz"
pi_payload="$work/pi-payload.tar.xz"
python3 "$builder" payload --target ubuntu-24.04-amd64 --offline-receipt "$ubuntu_receipt" \
  --lock "$input_lock" --source "$work/stage-amd64" --output "$ubuntu_payload" \
  --version "$version" --source-date-epoch "$epoch"
python3 "$builder" payload --target rpi-debian13-arm64 --offline-receipt "$pi_receipt" \
  --lock "$input_lock" --source "$work/stage-arm64" --output "$pi_payload" \
  --version "$version" --source-date-epoch "$epoch"

ubuntu_deb="$output/nilo_${version}_amd64.deb"
ubuntu_run="$output/nilo-${version}-ubuntu-24.04-amd64.run"
pi_deb="$output/nilo_${version}_arm64.deb"
pi_run="$output/nilo-${version}-rpi-debian13-arm64.run"
pi_image="$output/nilo-pi4-${version}.img.xz"
python3 "$builder" deb --target ubuntu-24.04-amd64 --offline-receipt "$ubuntu_receipt" \
  --lock "$input_lock" --payload "$ubuntu_payload" --output "$ubuntu_deb" --version "$version" \
  --source-date-epoch "$epoch" --receipt "$receipts/ubuntu-deb.json" \
  --build-invocation "$invocation" --source-revision "$revision"
python3 "$builder" installer --target ubuntu-24.04-amd64 --offline-receipt "$ubuntu_receipt" \
  --lock "$input_lock" --package "$ubuntu_deb" --pool "$ubuntu_pool" --output "$ubuntu_run" \
  --version "$version" --source-date-epoch "$epoch" --receipt "$receipts/ubuntu-installer.json" \
  --build-invocation "$invocation" --source-revision "$revision"
python3 "$builder" deb --target rpi-debian13-arm64 --offline-receipt "$pi_receipt" \
  --lock "$input_lock" --payload "$pi_payload" --output "$pi_deb" --version "$version" \
  --source-date-epoch "$epoch" --receipt "$receipts/pi-deb.json" \
  --build-invocation "$invocation" --source-revision "$revision"
python3 "$builder" installer --target rpi-debian13-arm64 --offline-receipt "$pi_receipt" \
  --lock "$input_lock" --package "$pi_deb" --pool "$pi_pool" --output "$pi_run" \
  --version "$version" --source-date-epoch "$epoch" --receipt "$receipts/pi-installer.json" \
  --build-invocation "$invocation" --source-revision "$revision"
python3 "$builder" image --target rpi-debian13-arm64 --offline-receipt "$pi_receipt" \
  --lock "$input_lock" --base-image "$base_image" --package "$pi_deb" --pool "$pi_pool" \
  --admin-password-file "$admin_password" --output "$pi_image" --version "$version" \
  --source-date-epoch "$epoch" --receipt "$receipts/pi-image.json" \
  --build-invocation "$invocation" --source-revision "$revision"
python3 "$builder" build-set-receipt --artifacts "$output" --output "${output}.build.json" \
  --lock "$input_lock" --source-revision "$revision" \
  --artifact-receipt "$receipts/ubuntu-deb.json" \
  --artifact-receipt "$receipts/ubuntu-installer.json" \
  --artifact-receipt "$receipts/pi-deb.json" \
  --artifact-receipt "$receipts/pi-installer.json" \
  --artifact-receipt "$receipts/pi-image.json"
