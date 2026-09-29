#!/bin/sh
set -eu
[ "$#" -eq 1 ] || { echo "usage: verify-deb.sh PACKAGE.deb" >&2; exit 2; }
package=$1
dpkg-deb --info "$package" >/dev/null
contents=$(dpkg-deb --contents "$package")
printf '%s\n' "$contents" | grep -q './usr/bin/nilo$'
printf '%s\n' "$contents" | grep -q './usr/lib/systemd/system/nilo.service$'
if printf '%s\n' "$contents" | grep -Eq './usr/local/|./var/(lib|log)/nilo/.+'; then
  echo "Debian package owns a forbidden mutable or local path" >&2
  exit 2
fi
