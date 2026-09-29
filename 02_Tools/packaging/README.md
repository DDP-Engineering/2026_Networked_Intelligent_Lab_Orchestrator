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

## Public commands

Use `nilo-tools.ps1 package TARGET` on Windows or
`sh 02_Tools/nilo-tools.sh package TARGET` on Linux for canonical offline input
preparation. This writes a preparation receipt; it does not build a release.
Use `package release ARTIFACTS ACCEPTANCE SOURCE_OFFERS UBUNTU_RECEIPT PI_RECEIPT OUTPUT`
for release assembly and `package verify RELEASE_DIRECTORY` for verification.
Windows paths are translated into the selected WSL distribution. Both actions
use local pinned tools and the existing builder/verifier contracts described below.
Input acquisition and privileged image construction remain explicit specialist actions.

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

An `os-package-pool` declaration also supplies an `apt` policy: the immutable
repository snapshot URL, suite, component, exact archive-keyring SHA-256,
admitted full signing-key fingerprints, documented base-package assumptions,
and sorted root package names. Its tar must contain `apt/InRelease`,
`apt/Packages`, `apt/archive-keyring.gpg`, and the referenced `pool/*.deb`
files. Resolution uses `gpgv`, matches the full signing fingerprint, verifies
that `InRelease` binds `Packages`, checks every `.deb` against signed size,
digest, identity and architecture metadata, and proves that the pool is exactly
the recursive dependency closure of its roots after the declared base packages.
The resulting lock and offline receipt retain this policy and per-package
filename, version, architecture, dependency, signer, size and digest
provenance; release SBOM generation expands those package records rather than
representing the pool only as one opaque archive.

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
bytes used to verify the inventory, the canonical cache root, every selected
content-addressed object, and the observed isolation state. Artifact builders
rehash those objects at use time and bind external dependency/base-image bytes
to that admitted inventory. Physical
Raspberry Pi or mini-PC acceptance is always `not_run`; cross-compilation and
static checks never promote it.

## Offline artifact build

Artifact builders accept explicit, already staged inputs. They never resolve a
URL and can run in the repository's network-none build environment:

```sh
sh ./02_Tools/packaging/build/stage-payload.sh TARGET BIN_DIR GUI_DIST BUNDLES STAGE
sh ./02_Tools/packaging/build/build-payload.sh TARGET PREPARATION_JSON INPUT_LOCK_JSON STAGE payload.tar.xz
sh ./02_Tools/packaging/build/build-deb.sh TARGET PREPARATION_JSON INPUT_LOCK_JSON payload.tar.xz nilo_VERSION_ARCH.deb
sh ./02_Tools/packaging/build/build-offline-installer.sh TARGET PREPARATION_JSON INPUT_LOCK_JSON nilo_VERSION_ARCH.deb POOL nilo-VERSION-TARGET.run
sh ./02_Tools/packaging/build/build-pi-image.sh PI_PREPARATION_JSON INPUT_LOCK_JSON BASE.img.xz nilo_VERSION_arm64.deb POOL ADMIN_PASSWORD nilo-pi4-VERSION.img.xz
sh ./02_Tools/packaging/build/build-candidate-set.sh OUTPUT SOURCE_ROOT GO_MODULE_CACHE_DIR NPM_CACHE_DIR UBUNTU_POOL UBUNTU_RECEIPT UBUNTU_SIGROK_RUNTIME_DIR PI_POOL PI_RECEIPT PI_SIGROK_RUNTIME_DIR BASE.img.xz ADMIN_PASSWORD INPUT_LOCK_JSON
```

Every artifact builder requires both the matching offline preparation receipt
and the trusted input lock whose digest the receipt records; a direct
invocation cannot bypass the admitted inventory or network-isolation gate.
The Pi image wrapper additionally requires the verified base image and an
explicit root-only administrator password file. The image retains that source
as `/var/lib/nilo-installer/first-admin.password`, beneath a root-owned
mode-`0700` directory and itself root-owned mode `0600`; a narrow root one-shot
copies it to a mode-`0600` `nilo`-owned file under `/run`, invokes
`nilo seed-admin` as the service user, and removes the ephemeral handoff on
every exit while deleting the root source only after successful consumption.
Neither credential location is service-writable, a repository default, or a
command-line value. The image also carries a root-owned
one-shot preparation marker: before NILO starts, it expands the detected root
partition/filesystem, establishes the sanitized machine ID, and verifies that
NetworkManager is active. `build-release.sh` takes both target
preparation receipts, a schema-2 acceptance plan naming checks and timeouts, and a corresponding-source
directory that is an exact extraction of the admitted `source-offers` archive,
including `SOURCE_OFFER.txt`, `source-index.json` and licence texts. It closes the exact
five-artifact set using the plan names (`nilo-<version>-<target>.run` and
`nilo-pi4-<version>.img.xz`) into checksums, provenance, SBOM, notices, lock and
target copies, an operator guide, and digest-bound acceptance evidence. The
assembler first reparses the lock with the canonical signed-APT contract,
revalidates each signed pool from the admitted cache, and parses every `.deb`,
`.run` and `.img.xz` structure. Each installer must embed the exact package
set and bytes from its target's revalidated signed pool plus the matching NILO
package. It then executes every named software check
through the fixed repository-owned dispatcher. Destructive lifecycle rows go
through `run-target-acceptance.sh`, which requires already-local digest-pinned
Ubuntu amd64 and Debian 13 arm64 system images, starts a fresh privileged
container for each row with `--network none` and the exact platform, and alone
establishes the disposable-target marker. The assembler refuses caller-selected commands and a namespace
with any non-loopback interface or default route, derives pass only from a zero
exit status, and requires the reproducibility check to compare a distinct
rebuilt artifact directory byte for byte. The acceptance plan supplies only a
schema-1 candidate-build-input document containing absolute paths to the
identified Git source checkout; lock-bound Go-module, npm-cache and per-target
Sigrok runtime archives; signed package pools and preparation receipts;
verified Pi base image; protected administrator-password file; and release
input lock. The verifier safely extracts the lock-bound cache archives, then
runs `build-candidate-set.sh` itself twice into fresh temporary roots. Each run
rebuilds the exact Git revision's Go binaries, production GUI, target Sigrok
bundles, staged payloads and all five final artifacts. Its schema-3 build-set
receipts embed every artifact-builder receipt and
bind distinct pre-established invocation identities, source revision,
input-lock digest, builder program digest, command, inputs and artifact path.
Caller-supplied artifact copies or build receipts are not accepted as
reproducibility proof.
Rollback acceptance includes a successful representative-state upgrade, clean
install rollback to absent state, real dependency/package transaction faults,
migration/start/health faults, and kill-interrupted recovery at every durable
transaction marker from a separately built prior version. The
lifecycle runner also performs direct `.deb` install/reinstall and authenticated
Sigrok install/upgrade API operations on disposable exact-target systems.
Retained evidence includes the verifier-closure digest; exact configured image
reference, observed image ID, kernel, OS release and architecture for every
lifecycle row; exact primary, reproduced and prior artifact bindings; result;
and hashed stdout and stderr. Caller-authored
`status: passed` records are not accepted. Public
distribution stays blocked until signing and physical acceptance are supplied.

Reinstall and upgrade preflight use APT's simulated transaction to identify the
packages that can actually change. The installer extracts the authenticated
embedded `dpkg-repack` package into its private transaction directory before
mutation, so a clean host does not need that command preinstalled. Each installed affected package is repacked
into the durable transaction directory, hashed and fsynced with the package
lists and journal before mutation; rollback installs the retained set together
and does not depend on `/var/cache/apt/archives`. Privileged transaction state
lives under root-owned mode-`0700` `/var/lib/nilo-installer/transactions`,
outside the service-writable application data root. Recovery revalidates the
sealed prior-package digest, package identity and version before reinstalling.

Each generated `.run` launcher authenticates its unique member inventory and
the embedded manifest before extracting the full archive, then checks every
regular file's size and digest before executing the installer driver. Release
SPDX evidence models each deliverable as containing its artifact and depending
on the admitted shared and target input packages; verification rejects missing,
extra or duplicate relationship edges.

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
