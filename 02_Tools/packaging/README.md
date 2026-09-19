# Offline packaging and delivery

This directory defines the two exact delivery targets, resolves and acquires
immutable external inputs during an explicit connected phase, and builds
deterministic payloads, Debian packages, self-extracting offline installers,
the Raspberry Pi appliance image, and release evidence without network access.

The checked-in input lock remains deliberately incomplete until the real base
image, dependency pools, source offers, GUI cache and production Sigrok
runtimes have approved HTTPS origins and redistribution evidence. Production
commands fail closed while that review is incomplete; tests use temporary
complete locks and never promote them into the shipped configuration.

## Exact targets

`config/targets.json` is the target authority:

- `ubuntu-24.04-amd64` — Ubuntu 24.04 LTS, `amd64`, `.deb` and `.run`;
- `rpi-debian13-arm64` — Raspberry Pi OS based on Debian 13, `arm64`,
  `.deb`, `.run`, and `.img.xz`.

A dependency pool belongs to that exact OS release and architecture. A newer OS
release is a different target and requires its own resolved pool and acceptance
run.

## Connected acquisition

Describe each reviewed input class in `config/release-inputs.json`, then resolve
the mutable source URLs into a digest-addressed cache and a proposal:

```powershell
.\02_Tools\packaging\fetch\resolve-inputs.ps1
```

```sh
sh ./02_Tools/packaging/fetch/resolve-inputs.sh
```

Review `inputs.lock.proposal.json`, its licences and source obligations before
copying it over `inputs.lock.json`. Every declaration names the exact artifact
type and architecture, an HTTPS provenance reference, and explicit
`redistribution: permitted` authority. Resolution validates the expected xz
image, Debian pool, runtime, cache, or source-offer structure before admitting
the observed size and SHA-256 to the proposal.

After adopting the reviewed proposal, populate or re-check the ignored cache:

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

## Offline artifact build

Artifact builders accept explicit, already staged inputs. They never resolve a
URL and can run in the repository's network-none build environment:

```sh
sh ./02_Tools/packaging/build/stage-payload.sh TARGET BIN_DIR GUI_DIST BUNDLES STAGE
sh ./02_Tools/packaging/build/build-payload.sh TARGET PREPARATION_JSON STAGE payload.tar.xz
sh ./02_Tools/packaging/build/build-deb.sh TARGET PREPARATION_JSON payload.tar.xz nilo_VERSION_ARCH.deb
sh ./02_Tools/packaging/build/build-offline-installer.sh TARGET PREPARATION_JSON nilo_VERSION_ARCH.deb POOL nilo-VERSION-TARGET.run
sh ./02_Tools/packaging/build/build-pi-image.sh PI_PREPARATION_JSON BASE.img.xz nilo_VERSION_arm64.deb POOL ADMIN_PASSWORD nilo-pi4-VERSION.img.xz
```

Every artifact builder requires the matching offline preparation receipt; a
direct invocation cannot bypass the verified cache and network-isolation gate.
The Pi image wrapper additionally requires the verified base image and an
explicit root-only administrator password file. The password is installed with
mode `0600`, consumed by `nilo seed-admin` on first boot, and is never a
repository default or command-line value. `build-release.sh` takes both target
preparation receipts, executed software evidence, and a corresponding-source
directory containing `SOURCE_OFFER.txt` plus licence texts. It closes the exact
five-artifact set using the plan names (`nilo-<version>-<target>.run` and
`nilo-pi4-<version>.img.xz`) into checksums, provenance, SBOM, notices, lock and
target copies, an operator guide, and digest-bound acceptance evidence. Public
distribution stays blocked until signing and physical acceptance are supplied.

Verification entrypoints live under `verify/`. Static and emulated checks can
pass without hardware; Raspberry Pi boot/GPIO and Ubuntu mini-PC USB checks
remain `not_run` until executed on the named physical targets.

The OCI boundary itself has an executable complete-fixture check. It derives a
temporary test image only from already-present repository runtimes and the
appliance test image, performs no download, and removes its test image and
volume when finished:

```sh
sh ./02_Tools/tests/packaging/check.sh
```

## Exit behavior

- `0` — requested validation, build or verification completed;
- `2` — configuration, integrity, target, or isolation policy was refused;
- another nonzero value — a repository runtime, container engine, or unexpected
  operating-system error prevented the command from running.

No command assigns a release version, creates a Git commit, or publishes an
artifact.
