#!/bin/sh
# Create one fresh exact-target system container for a destructive lifecycle check.
set -eu

target=
check=
artifacts=
prior_installer=
while [ "$#" -gt 0 ]; do
  case "$1" in
    --target) target=$2; shift 2 ;;
    --check) check=$2; shift 2 ;;
    --artifacts) artifacts=$2; shift 2 ;;
    --prior-installer) prior_installer=$2; shift 2 ;;
    *) echo "unsupported target acceptance argument: $1" >&2; exit 2 ;;
  esac
done
[ -n "$target" ] && [ -n "$check" ] && [ -d "$artifacts" ] || {
  echo 'target acceptance harness requires target, check and artifacts' >&2; exit 2;
}
case "$target" in
  ubuntu-24.04-amd64)
    image=${NILO_ACCEPTANCE_IMAGE_UBUNTU_24_04_AMD64:-}
    platform=linux/amd64
    ;;
  rpi-debian13-arm64)
    image=${NILO_ACCEPTANCE_IMAGE_RPI_DEBIAN13_ARM64:-}
    platform=linux/arm64
    ;;
  *) echo "unsupported target: $target" >&2; exit 2 ;;
esac
case "$image" in *@sha256:????????????????????????????????????????????????????????????????) ;;
  *) echo "acceptance image for $target must be digest-pinned and already local" >&2; exit 2 ;;
esac
[ "${NILO_ACCEPTANCE_EXPECTED_IMAGE:-}" = "$image" ] || {
  echo 'target acceptance image does not match the reviewed command plan' >&2; exit 2;
}
engine=${NILO_ACCEPTANCE_CONTAINER_ENGINE:-}
if [ -z "$engine" ]; then
  command -v podman >/dev/null 2>&1 && engine=podman || true
  [ -n "$engine" ] || { command -v docker >/dev/null 2>&1 && engine=docker || true; }
fi
[ -n "$engine" ] || { echo 'Podman or Docker is required for target acceptance' >&2; exit 2; }
image_id=$("$engine" image inspect --format '{{.Id}}' "$image")
case "$image_id" in sha256:????????????????????????????????????????????????????????????????) ;;
  *) echo 'container engine returned an invalid immutable image identity' >&2; exit 2 ;;
esac

repo=$(CDPATH= cd -- "$(dirname -- "$0")/../../.." && pwd)
name="nilo-acceptance-$(printf '%s-%s-%s' "$target" "$check" "$$" | tr -c 'a-zA-Z0-9_.-' '-')"
cleanup() { "$engine" rm -f "$name" >/dev/null 2>&1 || true; }
trap cleanup EXIT HUP INT TERM
set -- "$engine" run -d --rm --name "$name" --network none --platform "$platform" --privileged \
  --tmpfs /run --tmpfs /tmp -v "$repo:$repo:ro" -v "$artifacts:/nilo/artifacts:ro"
if [ -n "$prior_installer" ]; then
  [ -f "$prior_installer" ] && [ ! -L "$prior_installer" ] || { echo 'unsafe prior installer' >&2; exit 2; }
  set -- "$@" -v "$prior_installer:/nilo/prior-installer.run:ro"
fi
set -- "$@" "$image" /sbin/init
"$@" >/dev/null

attempt=0
until "$engine" exec "$name" systemctl is-system-running --wait >/dev/null 2>&1; do
  attempt=$((attempt + 1))
  [ "$attempt" -lt 30 ] || { echo 'target container did not reach systemd' >&2; exit 1; }
  sleep 1
done
kernel=$("$engine" exec "$name" uname -r)
os_identity=$("$engine" exec "$name" sh -ec '. /etc/os-release; printf "%s:%s\n" "$ID" "$VERSION_ID"')
os_id=${os_identity%%:*}
os_version=${os_identity#*:}
architecture=$("$engine" exec "$name" dpkg --print-architecture)
printf 'NILO_ACCEPTANCE_IMAGE_REFERENCE=%s\n' "$image"
printf 'NILO_ACCEPTANCE_IMAGE_ID=%s\n' "$image_id"
printf 'NILO_ACCEPTANCE_KERNEL=%s\n' "$kernel"
printf 'NILO_ACCEPTANCE_OS_ID=%s\n' "$os_id"
printf 'NILO_ACCEPTANCE_OS_VERSION=%s\n' "$os_version"
printf 'NILO_ACCEPTANCE_ARCHITECTURE=%s\n' "$architecture"
set -- "$engine" exec -e "NILO_ACCEPTANCE_DISPOSABLE_TARGET=$target" "$name" sh \
  "$repo/02_Tools/tests/packaging/release-acceptance.sh" \
  --target "$target" --check "$check" --artifacts /nilo/artifacts
[ -z "$prior_installer" ] || set -- "$@" --prior-installer /nilo/prior-installer.run
"$@"
