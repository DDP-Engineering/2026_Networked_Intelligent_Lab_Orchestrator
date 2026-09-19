# Offline packaging input foundation

This directory implements the first packaging boundary for NILO. It defines the
two exact delivery targets, acquires immutable external inputs during an
explicitly connected phase, verifies the transferred cache, and prepares a
deterministic input receipt only inside a network-isolated Linux build
environment.

It does not yet build a `.deb`, `.run`, or `.img.xz`. The checked-in input lock
is deliberately marked incomplete, so every production fetch, verification,
and preparation command currently fails closed and names the unresolved
dependency closures. Change `complete` only after every required input has an
approved HTTPS origin, exact size, SHA-256 digest, and licence/provenance
record.

## Exact targets

`config/targets.json` is the target authority:

- `ubuntu-24.04-amd64` — Ubuntu 24.04 LTS, `amd64`, eventual `.deb` and `.run`;
- `rpi-debian13-arm64` — Raspberry Pi OS based on Debian 13, `arm64`, eventual
  `.deb`, `.run`, and `.img.xz`.

A dependency pool belongs to that exact OS release and architecture. A newer OS
release is a different target and requires its own resolved pool and acceptance
run.

## Connected fetch

After completing `config/inputs.lock.json`, populate the ignored
`02_Tools/cache/packaging` cache:

```powershell
.\02_Tools\packaging\fetch\fetch-all.ps1 ubuntu-24.04-amd64
.\02_Tools\packaging\fetch\fetch-all.ps1 rpi-debian13-arm64
```

```sh
sh ./02_Tools/packaging/fetch/fetch-all.sh ubuntu-24.04-amd64
sh ./02_Tools/packaging/fetch/fetch-all.sh rpi-debian13-arm64
```

Fetch accepts only validated HTTPS entries selected for `shared` or the named
target. It streams through cache-local staging, bounds the declared size,
checks the final redirect URL and SHA-256, then atomically installs the
digest-addressed object. A verified existing object is reused.

## Cache verification

Verification opens no URL and writes no cache object:

```powershell
.\02_Tools\packaging\verify\verify-cache.ps1 ubuntu-24.04-amd64
```

```sh
sh ./02_Tools/packaging/verify/verify-cache.sh ubuntu-24.04-amd64
```

The command reports every missing, wrong-sized, digest-mismatched, symlinked,
or otherwise non-regular selected object it can inspect. On Linux it traverses
the digest-derived path without following links and opens the final object
non-blocking so a FIFO cannot stall verification. Extra objects may remain in
the shared content-addressed cache, but they never enter a target inventory
unless the lock declares them.

## Offline preparation

Preparation is reached through the locked build environment:

```powershell
.\02_Tools\env\with-build-environment.ps1 package ubuntu-24.04-amd64
```

```sh
./02_Tools/env/with-build-environment.sh package ubuntu-24.04-amd64
```

The wrapper keeps its existing network-none, unprivileged, read-only-checkout
boundary; it additionally mounts the packaging cache read-only at
`/packaging-cache`. The preparation process independently requires Linux,
loopback as the sole interface, and no IPv4 or IPv6 default route.

On success it writes:

```text
02_Tools/cache/containers/outputs/packaging/<target>/preparation.json
```

The receipt contains the exact target, the digest of the same captured lock
bytes used to verify the inventory, and the observed isolation state. Physical
Raspberry Pi or mini-PC acceptance is always `not_run`; cross-compilation and
static checks never promote it.

The OCI boundary itself has an executable complete-fixture check. It derives a
temporary test image only from already-present repository runtimes and the
appliance test image, performs no download, and removes its test image and
volume when finished:

```sh
sh ./02_Tools/tests/packaging/check.sh
```

## Exit behavior

- `0` — requested validation or preparation completed;
- `2` — configuration, integrity, target, or isolation policy was refused;
- another nonzero value — a repository runtime, container engine, or unexpected
  operating-system error prevented the command from running.

No command assigns a release version, creates a Git commit, or publishes an
artifact.
