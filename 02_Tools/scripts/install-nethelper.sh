#!/bin/sh
# Explicit Linux installation; build the helper unprivileged before invoking.
set -eu
if [ "$(id -u)" -ne 0 ] || [ "$#" -ne 2 ]; then
 echo 'Usage (as root): install-nethelper.sh HELPER_BINARY EXISTING_SERVICE_USER' >&2
 exit 2
fi
binary=$(realpath -- "$1")
service_user=$2
case "$service_user" in ''|*[!a-zA-Z0-9_-]*) echo 'Invalid service user' >&2; exit 2;; esac
service_uid=$(id -u "$service_user")
service_gid=$(id -g "$service_user")
[ "$service_uid" -gt 0 ] || { echo 'Bench Core must use a non-root service account' >&2; exit 1; }
[ -f "$binary" ] && [ -x "$binary" ] || { echo 'Built helper executable required' >&2; exit 1; }
# Device-network forwarding renders into nftables; the helper refuses to
# forward without it rather than falling back to anything else.
command -v nft >/dev/null 2>&1 || echo 'warning: nft is not installed; device networks stay saved but unsupported until nftables is installed' >&2
install -d -o root -g root -m 0755 /usr/local/libexec
install -o root -g root -m 0755 "$binary" /usr/local/libexec/nilo-nethelper
install -d -o root -g root -m 0700 /var/lib/nilo-nethelper
cat > /etc/systemd/system/nilo-nethelper.service <<EOF
[Unit]
Description=NILO restricted NetworkManager helper
After=NetworkManager.service
Requires=NetworkManager.service

[Service]
Type=simple
User=root
Group=$service_gid
RuntimeDirectory=nilo
RuntimeDirectoryMode=0750
ExecStart=/usr/local/libexec/nilo-nethelper --allow-uid $service_uid --socket-gid $service_gid
Restart=on-failure
RestartSec=3
UMask=0077
NoNewPrivileges=yes
PrivateTmp=yes
ProtectHome=yes
ProtectSystem=strict
# Acknowledged wireguard-tools installation needs package payload/database and
# signed repository access. /boot and home directories remain protected.
ReadWritePaths=/var/lib/nilo-nethelper /run/nilo /usr /etc /var/lib/dpkg /var/lib/apt /var/lib/systemd /var/cache/apt /var/log
RestrictAddressFamilies=AF_UNIX AF_INET AF_INET6 AF_NETLINK

[Install]
WantedBy=multi-user.target
EOF
# Independent recovery: a root-owned timer that runs even when both Bench Core
# and the helper are gone. It consumes only the saved records under
# /var/lib/nilo-nethelper, takes the helper's lock without waiting, and does
# nothing while the helper is alive.
cat > /etc/systemd/system/nilo-nethelper-recovery.service <<EOF
[Unit]
Description=NILO independent network recovery pass
After=NetworkManager.service
Requires=NetworkManager.service

[Service]
Type=oneshot
User=root
ExecStart=/usr/local/libexec/nilo-nethelper recover --state-dir /var/lib/nilo-nethelper
UMask=0077
NoNewPrivileges=yes
PrivateTmp=yes
ProtectHome=yes
ProtectSystem=strict
ReadWritePaths=/var/lib/nilo-nethelper
RestrictAddressFamilies=AF_UNIX AF_NETLINK
EOF
cat > /etc/systemd/system/nilo-nethelper-recovery.timer <<EOF
[Unit]
Description=Run the NILO independent network recovery pass

[Timer]
OnBootSec=20s
OnUnitActiveSec=30s
AccuracySec=5s
Unit=nilo-nethelper-recovery.service

[Install]
WantedBy=timers.target
EOF
systemctl daemon-reload
systemctl enable --now nilo-nethelper.service
systemctl enable --now nilo-nethelper-recovery.timer
