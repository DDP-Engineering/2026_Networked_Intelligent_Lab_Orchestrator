#!/bin/sh
# Verify the offline structure of a Raspberry Pi delivery image. Hardware
# acceptance remains a separate, explicitly not-run gate.
set -eu
[ "$#" -eq 1 ] || { echo "usage: verify-image.sh IMAGE.img.xz" >&2; exit 2; }
image_archive=$1
[ -f "$image_archive" ] && [ ! -L "$image_archive" ] || { echo "image must be a regular non-symlink file" >&2; exit 2; }
[ "$(id -u)" -eq 0 ] || { echo "image structure verification requires root" >&2; exit 2; }
for command in xz parted losetup lsblk e2fsck mount umount chroot systemctl stat grep find; do
  command -v "$command" >/dev/null 2>&1 || { echo "required image verifier is missing: $command" >&2; exit 2; }
done
xz -t "$image_archive"

work=$(mktemp -d "${TMPDIR:-/tmp}/nilo-verify-image.XXXXXX")
image=$work/image.img
root=$work/root
loop=
mounted=0
cleanup() {
  set +e
  [ "$mounted" -eq 0 ] || umount "$root"
  [ -z "$loop" ] || losetup -d "$loop"
  rm -rf -- "$work"
}
trap cleanup EXIT HUP INT TERM
xz -dc -- "$image_archive" >"$image"
parted -s "$image" print >/dev/null
loop=$(losetup --find --show --partscan --read-only "$image")
root_partition=$(lsblk -lnpo NAME,FSTYPE "$loop" | awk '$2 ~ /^ext[234]$/ {last=$1} END {print last}')
[ -n "$root_partition" ] || { echo 'image has no Linux root partition' >&2; exit 2; }
e2fsck -fn "$root_partition" >/dev/null || [ "$?" -le 1 ]
mkdir "$root"
mount -o ro,noload "$root_partition" "$root"
mounted=1

for path in usr/bin/nilo usr/libexec/nilo/nilo-image-firstboot usr/libexec/nilo/nilo-installer-recover var/lib/nilo-installer/first-admin.password var/lib/nilo/image-firstboot.pending; do
  [ -f "$root/$path" ] && [ ! -L "$root/$path" ] || { echo "image is missing required file: $path" >&2; exit 2; }
done
chroot "$root" dpkg-query -W -f='${Status}\n' nilo | grep -qx 'install ok installed'
for unit in NetworkManager.service nilo-image-firstboot.service nilo-nethelper.socket nilo-recovery.service nilo-firstboot.service nilo.service; do
  [ "$(systemctl --root="$root" is-enabled "$unit")" = enabled ] || { echo "image unit is not enabled: $unit" >&2; exit 2; }
done
[ -f "$root/etc/NetworkManager/conf.d/10-nilo-managed.conf" ] || { echo 'image lacks NILO NetworkManager ownership configuration' >&2; exit 2; }
[ "$(stat -c '%a:%U:%G' "$root/var/lib/nilo-installer")" = '700:root:root' ] || { echo 'installer state permissions are invalid' >&2; exit 2; }
[ "$(stat -c '%a:%U:%G' "$root/var/lib/nilo-installer/first-admin.password")" = '600:root:root' ] || { echo 'image credential permissions are invalid' >&2; exit 2; }
[ ! -s "$root/etc/machine-id" ] || { echo 'image machine-id is not sanitized' >&2; exit 2; }
if find "$root/etc/ssh" -maxdepth 1 -name 'ssh_host_*' -print -quit | grep -q .; then
  echo 'image contains an SSH host key' >&2; exit 2
fi
printf '%s\n' '{"schema":1,"structure":"verified","hardware_acceptance":"not_run"}'
