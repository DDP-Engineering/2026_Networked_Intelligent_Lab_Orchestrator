#!/usr/bin/env sh
set -eu

usage() { printf '%s\n' "usage: $0 --host USER@HOST --bundle PATH --expected-libsigrok VERSION" >&2; exit 2; }
host=
bundle=
expected=
while [ "$#" -gt 0 ]; do
    case "$1" in
        --host) host=${2:-}; shift 2 ;;
        --bundle) bundle=${2:-}; shift 2 ;;
        --expected-libsigrok) expected=${2:-}; shift 2 ;;
        *) usage ;;
    esac
done
[ -n "$host" ] && [ -f "$bundle" ] && [ -n "$expected" ] || usage
case "$host" in *@*) ;; *) usage ;; esac
remote=$(ssh "$host" 'mktemp -d /tmp/nilo-arm64-abi.XXXXXX')
trap 'ssh "$host" "rm -rf -- $remote" >/dev/null 2>&1 || true' EXIT HUP INT TERM
scp "$bundle" "$host:$remote/bundle.tar.gz"
ssh "$host" "set -eu; test \"\$(uname -m)\" = aarch64; tar -xzf '$remote/bundle.tar.gz' -C '$remote'; file '$remote'/nilo_sigrok_worker | grep -E 'ARM aarch64|ARM64'; pkg-config --modversion libsigrok | grep -Fx '$expected'; '$remote'/nilo_sigrok_worker --abi-smoke"
printf '%s\n' "authorized ARM64 Sigrok ABI smoke passed on $host for libsigrok $expected"
