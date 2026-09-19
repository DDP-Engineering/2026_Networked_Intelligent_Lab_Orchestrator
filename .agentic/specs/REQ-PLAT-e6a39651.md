# REQ-PLAT-e6a39651 — Build sanitized Raspberry Pi appliance images

## Behavior

The privileged image builder verifies and decompresses the pinned Raspberry Pi
OS Lite 64-bit Debian 13 image, grows and mounts it with trap-owned cleanup,
blocks service starts, installs the offline pool and arm64 NILO package,
configures first boot, sanitizes cloned identity, unmounts completely, checks
the filesystem and emits deterministic compressed bytes plus a checksum.

## Data / interface

`build-pi-image.sh <cache> <arm64-deb> <output> --admin-password-file <path>`
requires Linux root or an isolated capability-bearing builder. The image target
is Raspberry Pi 4 with at least 16 GB storage. SSH and recovery GPIO are
disabled by default. First boot expands the root filesystem, regenerates
machine ID and host keys only if SSH is later enabled, consumes the protected
administrator source, starts helper/core and leaves integrations disabled.
Opt-in recovery uses BCM17 active-low button and BCM27 active-high LED.

## Examples

The sealed image contains no machine ID, host key, logs, package cache,
`policy-rc.d`, build mount or running NILO process. An unavailable GPIO library
does not block normal boot because recovery stays disabled.

## Acceptance

Fixture-image tests validate mount cleanup and sanitation. Static inspection
checks partitions, files, units, architecture and checksum. Real boot,
filesystem growth, credentials, networking, UI, integrations, power cycle and
GPIO stay separately `not_run` until physical Pi execution.

## Assumptions and open decisions

The official dated base image and digest come from the adopted complete lock.
The build password file is explicit local secret material and is never stored
in source, cache provenance, logs or a general release manifest.
