#!/bin/sh
set -eu
[ -f /.dockerenv ] || { echo 'Run only in the disposable test container' >&2; exit 1; }
[ "${NILO_NETWORK_SYSTEM:-}" = 1 ] || { echo 'Explicit system-test opt-in required' >&2; exit 1; }
cd /repo/05_Software/nilo_assistant
race_flag=
if [ "${NILO_NETWORK_RACE:-0}" = 1 ]; then race_flag=-race; fi
# Finish compilation/downloads while the container still has its original link.
../../02_Tools/env/with-go.sh go test $race_flag -mod=readonly -c -o /tmp/nethelper.test ./internal/nethelper
../../02_Tools/env/with-go.sh go build -mod=readonly -o /tmp/nilo ./cmd/nilo
../../02_Tools/env/with-go.sh go build -mod=readonly -o /tmp/nilo-nethelper ./cmd/nilo-nethelper
../../02_Tools/env/with-go.sh go test $race_flag -mod=readonly -c -o /tmp/appliance.test ./cmd/nilo
dbus-daemon --system --fork
NetworkManager --no-daemon >/tmp/networkmanager.log 2>&1 &
nm_pid=$!
trap 'kill "$nm_pid" 2>/dev/null || true' EXIT
attempt=0
until nmcli general status >/dev/null 2>&1; do
 attempt=$((attempt+1)); [ "$attempt" -lt 50 ] || { cat /tmp/networkmanager.log; exit 1; }
 sleep 0.2
done
# Install only inside this disposable namespace, before connectivity tests
# replace its online route. The image deliberately starts without wg.
NILO_INSTALL_SYSTEM=1 /tmp/nethelper.test -test.v -test.timeout 5m -test.run '^TestWireGuardInstallSystem$'
[ "${NILO_INSTALL_ONLY:-0}" != 1 ] || exit 0
nmcli device set eth0 managed yes
nmcli connection add type ethernet ifname eth0 con-name nilo-test ipv4.method manual ipv4.addresses 192.0.2.2/24 ipv4.gateway 192.0.2.1 ipv4.dns 192.0.2.53 ipv6.method manual ipv6.addresses fd00:10::2/64 ipv6.gateway fd00:10::1 ipv6.dns fd00:10::53 connection.autoconnect yes
nmcli connection up nilo-test
attempt=0
until nmcli --escape no -g IP4.ADDRESS,IP6.ADDRESS device show eth0 | grep -q "fd00:10::2/64"; do
 attempt=$((attempt+1)); [ "$attempt" -lt 50 ] || { cat /tmp/networkmanager.log; exit 1; }
 sleep 0.2
done
attempt=0
while ip -6 address show dev eth0 tentative | grep -q inet6; do
 attempt=$((attempt+1)); [ "$attempt" -lt 50 ] || { echo 'IPv6 DAD did not complete' >&2; exit 1; }
 sleep 0.2
done
nmcli -f GENERAL.DEVICE,GENERAL.TYPE,GENERAL.STATE device show eth0
ip address show dev eth0
nmcli -f IP4,IP6 device show eth0
NILO_NETHELPER_BINARY=/tmp/nilo-nethelper /tmp/nethelper.test -test.v -test.timeout 12m "$@" || { echo '--- NetworkManager log (tail) ---'; tail -n 80 /tmp/networkmanager.log; exit 1; }
/tmp/appliance.test -test.v -test.timeout 6m -test.run TestApplianceSystem
