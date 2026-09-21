#!/bin/sh
# Customize one pinned Raspberry Pi OS image without starting target services.
set -eu

[ "$#" -eq 5 ] || {
  echo "usage: build-pi-image.sh BASE.img.xz NILO.deb POOL ADMIN_PASSWORD OUTPUT.img.xz" >&2
  exit 2
}
base=$1
package=$2
pool=$3
admin_password=$4
output=$5
output_dir=$(dirname -- "$output")
output_name=$(basename -- "$output")

[ "$(id -u)" -eq 0 ] || { echo "Raspberry Pi image construction requires root" >&2; exit 2; }
for input in "$base" "$package" "$admin_password"; do
  [ -f "$input" ] && [ ! -L "$input" ] || { echo "unsafe image input: $input" >&2; exit 2; }
done
[ -d "$pool" ] && [ ! -L "$pool" ] || { echo "unsafe dependency pool: $pool" >&2; exit 2; }
for command in xz truncate parted losetup lsblk e2fsck resize2fs mount umount chroot systemctl sha256sum stat grep find; do
  command -v "$command" >/dev/null 2>&1 || { echo "required image tool is missing: $command" >&2; exit 2; }
done

work=$(mktemp -d "${TMPDIR:-/tmp}/nilo-pi-image.XXXXXX")
image=$work/nilo.img
root=$work/root
boot=$work/boot
loop=
root_partition=
boot_partition=
mounted_root=0
mounted_boot=0
temporary_output=

cleanup() {
  set +e
  [ "$mounted_boot" -eq 0 ] || umount "$boot"
  [ "$mounted_root" -eq 0 ] || umount "$root"
  [ -z "$loop" ] || losetup -d "$loop"
  [ -z "$temporary_output" ] || rm -f -- "$temporary_output"
  rm -rf -- "$work"
}
trap cleanup EXIT HUP INT TERM

xz -dc -- "$base" >"$image"
truncate -s 16G "$image"
loop=$(losetup --find --show --partscan "$image")
root_partition=$(lsblk -lnpo NAME,FSTYPE "$loop" | awk '$2 ~ /^ext[234]$/ {last=$1} END {print last}')
[ -n "$root_partition" ] || { echo "Raspberry Pi root filesystem was not found" >&2; exit 2; }
root_partition_number=$(lsblk -nro PARTN "$root_partition")
[ -n "$root_partition_number" ] || { echo "Raspberry Pi root partition number was not found" >&2; exit 2; }
losetup -d "$loop"
loop=
parted -s "$image" resizepart "$root_partition_number" 100%
loop=$(losetup --find --show --partscan "$image")
root_partition=$(lsblk -lnpo NAME,FSTYPE "$loop" | awk '$2 ~ /^ext[234]$/ {last=$1} END {print last}')
boot_partition=$(lsblk -lnpo NAME,FSTYPE "$loop" | awk '$2 ~ /^(vfat|fat)$/ {print $1; exit}')
[ -n "$root_partition" ] || { echo "resized Raspberry Pi root filesystem was not found" >&2; exit 2; }
e2fsck -pf "$root_partition" || [ "$?" -le 1 ]
resize2fs "$root_partition"
mkdir "$root" "$boot"
mount "$root_partition" "$root"
mounted_root=1
if [ -n "$boot_partition" ]; then
  mount "$boot_partition" "$boot"
  mounted_boot=1
fi

install -d -m 0755 "$root/tmp/nilo-pool"
install -m 0644 "$package" "$root/tmp/nilo-pool/"
find "$pool" -maxdepth 1 -type f -name '*.deb' -exec install -m 0644 '{}' "$root/tmp/nilo-pool/" ';'
cat >"$root/usr/sbin/policy-rc.d" <<'POLICY'
#!/bin/sh
exit 101
POLICY
chmod 0755 "$root/usr/sbin/policy-rc.d"

# binfmt_misc/QEMU must already be prepared by the pinned build environment.
chroot "$root" /bin/true || {
  echo "arm64 chroot is unavailable; configure pinned binfmt/QEMU support" >&2
  exit 2
}
chroot "$root" /bin/sh -ec '
  apt-get --no-download --no-install-recommends --no-remove \
    -o Dir::Etc::sourcelist=/dev/null -o Dir::Etc::sourceparts=- \
    -o Acquire::http::Proxy=false -o Acquire::https::Proxy=false \
    --yes install /tmp/nilo-pool/*.deb
  rm -rf /tmp/nilo-pool
'
rm -f "$root/usr/sbin/policy-rc.d"

install -d -o 0 -g 0 -m 0755 "$root/etc/nilo"
printf '%s\n' 'NILO_FIRST_ADMIN_USERNAME=admin' >"$root/etc/nilo/firstboot.env"
printf '%s\n' 'NILO_GPIO_RECOVERY=disabled' >"$root/etc/nilo/nilo.env"
chmod 0640 "$root/etc/nilo/firstboot.env" "$root/etc/nilo/nilo.env"
install -d -o 0 -g 0 -m 0750 "$root/var/lib/nilo"
chroot "$root" chown -R nilo:nilo /var/lib/nilo
install -d -o 0 -g 0 -m 0700 "$root/var/lib/nilo-installer"
install -o 0 -g 0 -m 0600 "$admin_password" "$root/var/lib/nilo-installer/first-admin.password"
install -o 0 -g 0 -m 0600 /dev/null "$root/var/lib/nilo/image-firstboot.pending"

install -d -o 0 -g 0 -m 0755 "$root/etc/NetworkManager/conf.d"
cat >"$root/etc/NetworkManager/conf.d/10-nilo-managed.conf" <<'NETWORK_MANAGER'
[main]
plugins=keyfile
NETWORK_MANAGER
chmod 0644 "$root/etc/NetworkManager/conf.d/10-nilo-managed.conf"

systemctl --root="$root" enable NetworkManager.service nilo-image-firstboot.service nilo-nethelper.socket nilo-recovery.service nilo-firstboot.service nilo.service >/dev/null
systemctl --root="$root" disable dhcpcd.service systemd-networkd.service >/dev/null 2>&1 || true
systemctl --root="$root" disable ssh.service sshd.service >/dev/null 2>&1 || true
rm -f "$root/etc/ssh"/ssh_host_* "$root/etc/machine-id" "$root/var/lib/dbus/machine-id"
: >"$root/etc/machine-id"
find "$root/var/log" -type f -exec sh -c ': >"$1"' _ '{}' ';'
rm -f "$root/var/lib/dhcp"/* "$root/var/lib/NetworkManager"/*lease* 2>/dev/null || true
chroot "$root" dpkg-query -W -f='${Status}\n' nilo | grep -qx 'install ok installed'
chroot "$root" /usr/bin/nilo version >/dev/null
for unit in NetworkManager.service nilo-image-firstboot.service nilo-nethelper.socket nilo-recovery.service nilo-firstboot.service nilo.service; do
  [ "$(systemctl --root="$root" is-enabled "$unit")" = enabled ] || { echo "required unit is not enabled: $unit" >&2; exit 2; }
done
[ -f "$root/var/lib/nilo/image-firstboot.pending" ] && [ ! -L "$root/var/lib/nilo/image-firstboot.pending" ] || {
  echo 'image first-boot marker is invalid' >&2; exit 2;
}
[ "$(stat -c '%a:%U:%G' "$root/var/lib/nilo-installer")" = '700:root:root' ] || {
  echo 'installer state directory permissions are invalid' >&2; exit 2;
}
[ "$(stat -c '%a:%U:%G' "$root/var/lib/nilo-installer/first-admin.password")" = '600:root:root' ] || {
  echo 'first administrator credential permissions are invalid' >&2; exit 2;
}
[ ! -s "$root/etc/machine-id" ] || { echo 'machine-id was not sanitized' >&2; exit 2; }
if find "$root/etc/ssh" -maxdepth 1 -name 'ssh_host_*' -print -quit | grep -q .; then
  echo 'SSH host key material remains in the image' >&2; exit 2
fi
find "$root" -xdev -type f -exec touch -h -d "@$SOURCE_DATE_EPOCH" '{}' +
sync

if [ "$mounted_boot" -eq 1 ]; then umount "$boot"; mounted_boot=0; fi
umount "$root"
mounted_root=0
e2fsck -pf "$root_partition" || [ "$?" -le 1 ]
parted -s "$image" print >/dev/null
losetup -d "$loop"
loop=
mkdir -p "$output_dir"
temporary_output=$(mktemp "$output_dir/.${output_name}.XXXXXX")
xz --threads=1 --check=crc64 -9 -c "$image" >"$temporary_output"
sync "$temporary_output"
mv -f "$temporary_output" "$output"
temporary_output=
sync "$output" "$output_dir"
