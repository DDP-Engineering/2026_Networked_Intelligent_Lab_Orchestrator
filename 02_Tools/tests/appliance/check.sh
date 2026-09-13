#!/bin/sh
# The appliance verification stage.
#
# Starts Bench Core in the configuration an appliance ships - `--profile
# appliance`, TLS material, a built interface directory and the real privileged
# helper - against a real NetworkManager, inside a disposable engine network
# this script creates and removes. Nothing here is simulated: every adapter the
# stage reports is one NetworkManager actually detected in that namespace.
#
# It runs on native Linux and inside WSL2. A host it does not support, a missing
# container engine, or an engine whose daemon does not answer is named and
# refused; none of those is ever reported as a pass or a skip.
#
# Exit codes: 0 the stage passed, 1 the stage failed, 2 a prerequisite refusal.
set -eu

EXIT_PREREQUISITE=2

fail_prerequisite() {
    printf '%s\n' "$1" >&2
    exit "$EXIT_PREREQUISITE"
}

# --- Prerequisites, before anything is built, created or started. ------------

kernel=$(uname -s 2>/dev/null || echo unknown)
case "$kernel" in
    Linux) ;;
    *)
        fail_prerequisite "The appliance verification stage runs on Linux, natively or inside a WSL2 distribution; this host reports $kernel. From a Windows checkout run it inside the WSL2 distribution prepared by 02_Tools/bootstrap/bootstrap-development.ps1."
        ;;
esac

engine=${NILO_APPLIANCE_ENGINE:-}
if [ -z "$engine" ]; then
    for candidate in docker podman; do
        if command -v "$candidate" >/dev/null 2>&1; then
            engine=$candidate
            break
        fi
    done
fi
[ -n "$engine" ] || fail_prerequisite 'No container engine was found on the path. The appliance verification stage needs Docker or Podman; install one in the WSL2 distribution, or set NILO_APPLIANCE_ENGINE to its executable.'

"$engine" info >/dev/null 2>&1 || fail_prerequisite "The container engine '$engine' is installed but its daemon did not answer. Start the engine daemon - for Docker inside WSL2, 'sudo service docker start' - and run the stage again."

# The host-isolation comparison is only meaningful if the host state can be read
# at all. Without this, a host with no `ip` would compare empty against empty and
# report isolation nobody observed.
command -v ip >/dev/null 2>&1 || fail_prerequisite "The 'ip' command is required to record this host's interfaces and routes before and after the stage. Install iproute2."
[ -d /lib/modules ] || fail_prerequisite "The running kernel's module inventory is unavailable at /lib/modules. The appliance stage needs read-only kernel evidence before it can exercise WireGuard."

# A prerequisite-only run exists so the refusals above can be exercised without
# building an image or creating a network.
[ "${NILO_APPLIANCE_PREREQUISITE_ONLY:-0}" != 1 ] || { printf '%s\n' "prerequisites satisfied: $engine"; exit 0; }

repo=$(CDPATH= cd -- "$(dirname -- "$0")/../../.." && pwd)

case "${NILO_APPLIANCE_DISTRO:-debian13}" in
    debian13) base=debian@sha256:f324c7ff54321e8d9c588493a20244965938ce0aa50bbd1022d38010e9ffc4b1; image=nilo-appliance-stage:local ;;
    ubuntu24) base=ubuntu@sha256:33ceb71981b602c1a7443a53469e4dba065f7503eab3078a2d7a57a2ab987517; image=nilo-appliance-stage:ubuntu24 ;;
    *) fail_prerequisite 'Supported fixtures: debian13, ubuntu24' ;;
esac

# --- Host state, recorded before the run and compared after it. --------------
#
# The stage must leave the developer's own machine exactly as it found it. The
# comparison runs on every exit path, including a failed stage, so a run that
# changed the host is reported as such rather than hidden behind the earlier
# failure.

host_state() {
    ip -o link show 2>/dev/null | awk '{print $2}' | sort
    printf -- '--\n'
    ip route show default 2>/dev/null | sort
}

host_state_before=$(host_state)

name=nilo-appliance-stage-$$
subnet=$(printf 'fd00:9435:%x::/64' "$(( $$ % 65535 ))")
# A bench appliance has an uplink and a device-facing adapter, and those roles
# are exclusive. Two engine networks give the container two real adapters, so
# that distinction is exercised against adapters the kernel actually has.
device_subnet=$(printf 'fd00:9436:%x::/64' "$(( $$ % 65535 ))")
network_created=0
device_network_created=0
container_created=0

compare_host_state() {
    host_state_after=$(host_state)
    if [ "$host_state_before" != "$host_state_after" ]; then
        printf '%s\n' 'The appliance verification stage changed this host network. Before:' >&2
        printf '%s\n' "$host_state_before" >&2
        printf '%s\n' 'After:' >&2
        printf '%s\n' "$host_state_after" >&2
        return 1
    fi
    return 0
}

cleanup() {
    status=$?
    if [ "$container_created" = 1 ]; then
        "$engine" rm -f "$name" >/dev/null 2>&1 || true
    fi
    if [ "$network_created" = 1 ]; then
        "$engine" network rm "$name" >/dev/null 2>&1 || true
    fi
    if [ "$device_network_created" = 1 ]; then
        "$engine" network rm "$name-device" >/dev/null 2>&1 || true
    fi
    if ! compare_host_state; then
        exit 1
    fi
    exit "$status"
}
trap cleanup EXIT HUP INT TERM

# --- The stage itself. -------------------------------------------------------

# The build context is the repository root so the interface project's locked
# manifests can be copied in; Dockerfile.dockerignore narrows it to those.
"$engine" build --target appliance --build-arg "NILO_BASE=$base" -t "$image" -f "$repo/02_Tools/tests/network/Dockerfile" "$repo"
"$engine" network create --ipv6 --subnet "$subnet" "$name" >/dev/null
network_created=1
"$engine" network create --ipv6 --subnet "$device_subnet" "$name-device" >/dev/null
device_network_created=1

# The container is created, given its second adapter, and only then started: an
# engine attaches a further network to a container that exists, not to one being
# run. The Go cache is shared with the helper system run - one image, one module
# graph, one cache - because a second volume would re-download the whole graph
# on every developer's machine for no benefit.
"$engine" create --name "$name" --network "$name" \
    --cap-add NET_ADMIN --cap-add SYS_ADMIN --security-opt systempaths=unconfined \
    --security-opt seccomp=unconfined \
    -e NILO_APPLIANCE_STAGE=1 \
    -e GOCACHE=/cache/build -e GOMODCACHE=/cache/mod \
    -v nilo-network-test-go-cache:/cache -v "$repo:/repo:ro" \
    -v /lib/modules:/lib/modules:ro \
    "$image" sh /repo/02_Tools/tests/appliance/run.sh "$@" >/dev/null
container_created=1
"$engine" network connect "$name-device" "$name"
"$engine" start -a "$name"
