#!/usr/bin/env sh
set -eu

usage() { printf '%s\n' "usage: $0 --first-digest sha256:... --second-digest sha256:... --first-log FILE --second-log FILE" >&2; exit 2; }
first=
second=
first_log=
second_log=
while [ "$#" -gt 0 ]; do
    case "$1" in
        --first-digest) first=${2:-}; shift 2 ;;
        --second-digest) second=${2:-}; shift 2 ;;
        --first-log) first_log=${2:-}; shift 2 ;;
        --second-log) second_log=${2:-}; shift 2 ;;
        *) usage ;;
    esac
done
[ -n "$first" ] && [ -n "$second" ] && [ -s "$first_log" ] && [ -s "$second_log" ] || usage
if [ "$first" != "$second" ]; then
    printf '%s\n' "locked repeatable environment only: independent OCI digests differ ($first vs $second)" >&2
    exit 1
fi
printf '%s\n' "bit-reproducible image evidence: independent OCI digests match $first"
