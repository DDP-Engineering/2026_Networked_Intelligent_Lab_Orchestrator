#!/bin/sh
# Bring up D-Bus, NetworkManager and this container's own eth0.
#
# Sourced, never executed: the caller keeps the NetworkManager process so it can
# stop it in its own trap, and keeps the shell's exit status handling. It only
# ever names eth0, the adapter the disposable engine network gave this
# container, so nothing here can reach an interface on the developer's host.
#
# Callers: 02_Tools/tests/network/run.sh (helper system tests) and
# 02_Tools/tests/appliance/run.sh (appliance verification stage).

nilo_network_up() {
    dbus-daemon --system --fork
    NetworkManager --no-daemon >/tmp/networkmanager.log 2>&1 &
    nm_pid=$!
    attempt=0
    until nmcli general status >/dev/null 2>&1; do
        attempt=$((attempt + 1))
        [ "$attempt" -lt 50 ] || { cat /tmp/networkmanager.log; return 1; }
        sleep 0.2
    done
}

# Configure this container's eth0 with the fixed addressing the tests assert
# against. Every wait is bounded; a link that never settles fails the run rather
# than hanging it.
nilo_network_configure() {
    nmcli device set eth0 managed yes
    nmcli connection add type ethernet ifname eth0 con-name nilo-test \
        ipv4.method manual ipv4.addresses 192.0.2.2/24 ipv4.gateway 192.0.2.1 \
        ipv4.dns 192.0.2.53 ipv4.route-metric 100 \
        ipv6.method manual ipv6.addresses fd00:10::2/64 ipv6.gateway fd00:10::1 \
        ipv6.dns fd00:10::53 ipv6.route-metric 100 connection.autoconnect yes
    nmcli connection up nilo-test
    attempt=0
    until nmcli --escape no -g IP4.ADDRESS,IP6.ADDRESS device show eth0 | grep -q "fd00:10::2/64"; do
        attempt=$((attempt + 1))
        [ "$attempt" -lt 50 ] || { cat /tmp/networkmanager.log; return 1; }
        sleep 0.2
    done
    attempt=0
    while ip -6 address show dev eth0 tentative | grep -q inet6; do
        attempt=$((attempt + 1))
        [ "$attempt" -lt 50 ] || { echo 'IPv6 DAD did not complete' >&2; return 1; }
        sleep 0.2
    done
}
