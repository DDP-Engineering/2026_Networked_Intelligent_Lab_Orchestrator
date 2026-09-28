#!/bin/sh
# Execute one fixed release-acceptance check. Lifecycle checks are destructive
# by design and therefore require a disposable exact-target namespace prepared
# by the packaging acceptance harness.
set -eu

target=
check=
artifacts=
candidate_build_inputs=
source_revision=
source_date_epoch=
expected_lock_sha256=
prior_installer=
while [ "$#" -gt 0 ]; do
  case "$1" in
    --target) target=$2; shift 2 ;;
    --check) check=$2; shift 2 ;;
    --artifacts) artifacts=$2; shift 2 ;;
    --candidate-build-inputs) candidate_build_inputs=$2; shift 2 ;;
    --source-revision) source_revision=$2; shift 2 ;;
    --source-date-epoch) source_date_epoch=$2; shift 2 ;;
    --expected-lock-sha256) expected_lock_sha256=$2; shift 2 ;;
    --prior-installer) prior_installer=$2; shift 2 ;;
    *) echo "unsupported release-acceptance argument: $1" >&2; exit 2 ;;
  esac
done
[ -n "$target" ] && [ -n "$check" ] && [ -d "$artifacts" ] || {
  echo 'usage: release-acceptance.sh --target TARGET --check CHECK --artifacts DIRECTORY' >&2
  exit 2
}
repo=$(CDPATH= cd -- "$(dirname -- "$0")/../../.." && pwd)
case "$target" in
  ubuntu-24.04-amd64) arch=amd64; deb="$artifacts/nilo_"*"_amd64.deb" ;;
  rpi-debian13-arm64) arch=arm64; deb="$artifacts/nilo_"*"_arm64.deb" ;;
  *) echo "unsupported release target: $target" >&2; exit 2 ;;
esac
installer="$artifacts/nilo-"*"-$target.run"
set -- $deb
[ "$#" -eq 1 ] && [ -f "$1" ] && [ ! -L "$1" ] || { echo 'exact Debian artifact is missing' >&2; exit 2; }
deb=$1
set -- $installer
[ "$#" -eq 1 ] && [ -f "$1" ] && [ ! -L "$1" ] || { echo 'exact installer artifact is missing' >&2; exit 2; }
installer=$1

# The assembler checks this before and after every invocation. Recheck inside
# the executed verifier so direct use cannot silently weaken the boundary.
[ "$(uname -s)" = Linux ] || { echo 'acceptance requires Linux' >&2; exit 2; }
interfaces=$(find /sys/class/net -mindepth 1 -maxdepth 1 -printf '%f\n' | sort)
[ "$interfaces" = lo ] || { echo 'acceptance namespace is not loopback-only' >&2; exit 2; }
ip -4 route show default | grep -q . && { echo 'acceptance namespace has an IPv4 default route' >&2; exit 2; }
ip -6 route show default | grep -q . && { echo 'acceptance namespace has an IPv6 default route' >&2; exit 2; }

case "$check" in
  package-policy)
    exec sh "$repo/02_Tools/packaging/verify/verify-deb.sh" "$deb"
    ;;
  image-structure)
    [ "$target" = rpi-debian13-arm64 ] || { echo 'image check is Raspberry-Pi-only' >&2; exit 2; }
    set -- "$artifacts"/*.img.xz
    [ "$#" -eq 1 ] || { echo 'exact image artifact is missing' >&2; exit 2; }
    exec sh "$repo/02_Tools/packaging/verify/verify-image.sh" "$1"
    ;;
  checksums)
    find "$artifacts" -maxdepth 1 -type f -print0 | sort -z | xargs -0 sha256sum
    ;;
  offline-boundary)
    printf '%s\n' 'loopback-only namespace with no IPv4 or IPv6 default route'
    ;;
  build-reproducibility)
    [ -f "$candidate_build_inputs" ] && [ ! -L "$candidate_build_inputs" ] && \
      [ -n "$source_revision" ] && [ -n "$source_date_epoch" ] && \
      [ -n "$expected_lock_sha256" ] || {
        echo 'reproducibility check requires fixed candidate build inputs and source identity' >&2; exit 2;
      }
    exec python3 "$repo/02_Tools/packaging/release_builder.py" reproduce \
      --artifacts "$artifacts" --build-inputs "$candidate_build_inputs" \
      --source-revision "$source_revision" --source-date-epoch "$source_date_epoch" \
      --expected-lock-sha256 "$expected_lock_sha256"
    ;;
  package-install|package-reinstall|installer-clean-install|installer-upgrade|installer-clean-rollback|installer-rollback-dependencies|installer-rollback-package|installer-rollback-migrated|installer-rollback-started|installer-rollback-verified|installer-interruption-preflight|installer-interruption-quiesced|installer-interruption-snapshotted|installer-interruption-dependencies|installer-interruption-package|installer-interruption-migrated|installer-interruption-started|installer-interruption-verified|installer-interruption-committed|installer-interruption-rolling_back|installer-interruption-rolled_back|installer-interruption-recovery-required|removal-purge|core-status-gui-helper-catalogue|integration-install-upgrade-health)
    [ "${NILO_ACCEPTANCE_DISPOSABLE_TARGET:-}" = "$target" ] || {
      echo 'lifecycle acceptance requires an exact disposable target namespace' >&2; exit 2;
    }
    [ "$(dpkg --print-architecture)" = "$arch" ] || { echo 'acceptance architecture mismatch' >&2; exit 2; }
    # shellcheck disable=SC1091
    . /etc/os-release
    case "$target" in
      ubuntu-24.04-amd64)
        [ "${ID:-}" = ubuntu ] && [ "${VERSION_ID:-}" = 24.04 ] || {
          echo 'acceptance operating-system release mismatch' >&2; exit 2;
        }
        ;;
      rpi-debian13-arm64)
        { [ "${ID:-}" = debian ] || [ "${ID:-}" = raspbian ]; } && [ "${VERSION_ID:-}" = 13 ] || {
          echo 'acceptance operating-system release mismatch' >&2; exit 2;
        }
        ;;
    esac
    printf 'acceptance target: %s (%s %s, %s)\n' "$target" "$ID" "$VERSION_ID" "$arch"
    # The target harness owns isolation and fault injection; this fixed command
    # cannot be replaced by an arbitrary caller-selected executable.
    exec sh "$repo/02_Tools/tests/packaging/target-lifecycle-check.sh" \
      "$target" "$check" "$deb" "$installer" "$prior_installer"
    ;;
  *) echo "unknown release acceptance check: $check" >&2; exit 2 ;;
esac
