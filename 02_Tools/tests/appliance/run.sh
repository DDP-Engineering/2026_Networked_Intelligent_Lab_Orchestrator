#!/bin/sh
# The container side of the appliance verification stage.
#
# Brings up D-Bus, NetworkManager and this container's own eth0, then builds and
# runs the appliance stage proof. Every adapter named here is this container's;
# nothing reaches the host that started it.
set -eu
[ -f /.dockerenv ] || { echo 'Run only in the disposable appliance stage container' >&2; exit 1; }
[ "${NILO_APPLIANCE_STAGE:-}" = 1 ] || { echo 'Explicit appliance stage opt-in required' >&2; exit 1; }

# The image inventory, proved rather than declared. Each probe below must stay
# equal to 02_Tools/tests/appliance/inventory.json, which
# 02_Tools/tests/software/test_appliance_stage.py asserts, so the list cannot
# drift from what the stage actually depends on.
. /repo/02_Tools/tests/appliance/probe.sh
chromium=$(find "$PLAYWRIGHT_BROWSERS_PATH" -type f -path '*/chrome-linux*/chrome' -print -quit)
[ -x "$chromium" ] || { echo 'The pinned Chromium executable was not found' >&2; exit 1; }
ln -sf "$chromium" /tmp/nilo-appliance-chromium
echo '--- appliance verification image inventory ---'
probe NetworkManager NetworkManager --version
probe dbus dbus-daemon --version
probe Avahi avahi-daemon --version
probe nftables nft --version
probe WireGuard wg --version
probe node node --version
probe chromium /tmp/nilo-appliance-chromium --version
echo '--- end inventory ---'

cd /repo/05_Software/core

# Finish compilation and any module reads while the container still has its
# original link, exactly as the helper system run does.
../../02_Tools/env/with-go.sh go build -mod=readonly -o /tmp/nilo ./cmd/nilo
../../02_Tools/env/with-go.sh go build -mod=readonly -o /tmp/nilo-nethelper ./cmd/nilo-nethelper
../../02_Tools/env/with-go.sh go test -mod=readonly -c -o /tmp/appliance-stage.test ./cmd/nilo

. /repo/02_Tools/tests/network/network-up.sh
nilo_network_up
browser_namespace=nilo-browser
browser_ingress=nilo-browser0
browser_host=198.18.0.1

cleanup_runtime() {
    ip netns delete "$browser_namespace" 2>/dev/null || true
    kill "$nm_pid" 2>/dev/null || true
    kill "$avahi_pid" 2>/dev/null || true
}
trap cleanup_runtime EXIT
nilo_network_configure

# Chromium runs in its own network namespace and reaches the appliance through
# this real veth ingress. Without that separation, Linux routes a connection to
# its own address over loopback and the production management firewall quite
# correctly rejects it for not arriving on the configured interface.
ip netns add "$browser_namespace"
ip link add "$browser_ingress" type veth peer name eth0 netns "$browser_namespace"
ip netns exec "$browser_namespace" ip link set lo up
ip netns exec "$browser_namespace" ip address add 198.18.0.2/30 dev eth0
ip netns exec "$browser_namespace" ip link set eth0 up
ip netns exec "$browser_namespace" ip route add default via "$browser_host"
nmcli device set "$browser_ingress" managed yes
nmcli connection add type ethernet ifname "$browser_ingress" con-name nilo-browser \
    ipv4.method manual ipv4.addresses "$browser_host/30" \
    ipv6.method disabled connection.autoconnect no
nmcli connection up nilo-browser
ip netns exec "$browser_namespace" ping -c 1 -W 2 "$browser_host" >/dev/null

# What the proof is asserted against, in the log, so a failure is diagnosable
# without re-running the stage.
nmcli -f GENERAL.DEVICE,GENERAL.TYPE,GENERAL.STATE device show eth0
nmcli -f IP4,IP6 device show eth0

NILO_NETHELPER_BINARY=/tmp/nilo-nethelper \
    /tmp/appliance-stage.test -test.v -test.timeout 6m \
    -test.run '^TestApplianceStage' \
    || { echo '--- NetworkManager log (tail) ---'; tail -n 80 /tmp/networkmanager.log; exit 1; }

# --- The browser suite, against the interface this appliance serves. ---------
#
# The interface project is built into a writable copy: the checkout is mounted
# read-only and Bench Core needs a real --gui-dir to serve. Its dependencies
# come from the image, installed there from the committed lockfile.
gui_root=/tmp/nilo-gui
rm -rf "$gui_root"
mkdir -p "$gui_root"
cp -r /repo/05_Software/frontend/. "$gui_root"/
rm -rf "$gui_root/node_modules"
ln -s /opt/nilo/gui-modules/node_modules "$gui_root/node_modules"
cd "$gui_root"
npm run build

browser_wrapper=/tmp/nilo-browser-chromium
printf '#!/bin/sh\nexec ip netns exec %s %s "$@"\n' "$browser_namespace" "$chromium" >"$browser_wrapper"
chmod 0755 "$browser_wrapper"

# The runner is root inside this disposable container, so Chromium's own sandbox
# is disabled in playwright.config.ts; the appliance it drives still runs
# unprivileged, spawned under the service account by the harness.
NILO_NILO_BINARY=/tmp/nilo \
NILO_NETHELPER_BINARY=/tmp/nilo-nethelper \
NILO_GUI_DIR="$gui_root/dist" \
NILO_APPLIANCE_BROWSER_NAMESPACE="$browser_namespace" \
NILO_APPLIANCE_MANAGEMENT_INTERFACE="$browser_ingress" \
NILO_APPLIANCE_MANAGEMENT_HOST="$browser_host" \
NILO_BROWSER_EXECUTABLE="$browser_wrapper" \
    npm run test:e2e -- "$@" \
    || { echo '--- NetworkManager log (tail) ---'; tail -n 80 /tmp/networkmanager.log; exit 1; }
