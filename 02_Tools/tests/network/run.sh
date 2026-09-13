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
# The bring-up is shared with the appliance verification stage so one apt list
# and one NetworkManager sequence stay in one place.
. /repo/02_Tools/tests/network/network-up.sh
nilo_network_up
trap 'kill "$nm_pid" 2>/dev/null || true' EXIT
# Install only inside this disposable namespace, before connectivity tests
# replace its online route. The image deliberately starts without wg.
NILO_INSTALL_SYSTEM=1 /tmp/nethelper.test -test.v -test.timeout 5m -test.run '^TestWireGuardInstallSystem$'
[ "${NILO_INSTALL_ONLY:-0}" != 1 ] || exit 0
nilo_network_configure
nmcli -f GENERAL.DEVICE,GENERAL.TYPE,GENERAL.STATE device show eth0
ip address show dev eth0
nmcli -f IP4,IP6 device show eth0
NILO_NETHELPER_BINARY=/tmp/nilo-nethelper /tmp/nethelper.test -test.v -test.timeout 12m "$@" || { echo '--- NetworkManager log (tail) ---'; tail -n 80 /tmp/networkmanager.log; exit 1; }
/tmp/appliance.test -test.v -test.timeout 6m -test.run TestApplianceSystem
