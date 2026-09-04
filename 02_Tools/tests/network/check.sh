#!/bin/sh
# Real NetworkManager tests in an owned, disposable network namespace.
set -eu
repo=$(CDPATH= cd -- "$(dirname -- "$0")/../../.." && pwd)
case "${NILO_NETWORK_DISTRO:-debian13}" in
 debian13) base=debian@sha256:f324c7ff54321e8d9c588493a20244965938ce0aa50bbd1022d38010e9ffc4b1; image=nilo-network-test:local ;;
 ubuntu24) base=ubuntu@sha256:33ceb71981b602c1a7443a53469e4dba065f7503eab3078a2d7a57a2ab987517; image=nilo-network-test:ubuntu24 ;;
 *) echo 'Supported fixtures: debian13, ubuntu24' >&2; exit 2 ;;
esac
name=nilo-network-test-$$
subnet=$(printf 'fd00:9434:%x::/64' "$(( $$ % 65535 ))")
cleanup() {
 docker rm -f "$name" >/dev/null 2>&1 || true
 docker network rm "$name" >/dev/null 2>&1 || true
}
docker build --build-arg "NILO_BASE=$base" -t "$image" "$repo/02_Tools/tests/network"
docker network create --ipv6 --subnet "$subnet" "$name" >/dev/null
trap cleanup EXIT HUP INT TERM
docker run --rm --name "$name" --network "$name" --cap-add NET_ADMIN --cap-add SYS_ADMIN --security-opt systempaths=unconfined \
 -e NILO_NETWORK_SYSTEM=1 -e NILO_NETWORK_RACE="${NILO_NETWORK_RACE:-0}" \
 -e NILO_INSTALL_ONLY="${NILO_INSTALL_ONLY:-0}" \
 -e GOCACHE=/cache/build -e GOMODCACHE=/cache/mod \
 -v nilo-network-test-go-cache:/cache -v "$repo:/repo:ro" "$image" sh /repo/02_Tools/tests/network/run.sh "$@"
