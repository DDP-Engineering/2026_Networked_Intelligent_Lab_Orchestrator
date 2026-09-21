<!--
Copyright (c) 2026 DDP Engineering
This software is licensed under the PolyForm Noncommercial License 1.0.0.
Commercial use requires a separate commercial license.
See the LICENSE.txt in the .agentic folder for full license details.
SPDX-License-Identifier: LicenseRef-PolyForm-Noncommercial-1.0.0
-->


# TASK-edf96bec - Complete offline release packaging and artifact builders

stage: validate
type: feature
path: full
interaction: autopilot
requirements: REQ-PLAT-da89bf3b, REQ-SEC-628657e1, REQ-REL-f4a90899, REQ-SEC-dd110b06, REQ-TEST-c442102f, REQ-PLAT-4af865a9, REQ-NFR-2dc6dc61, REQ-PLAT-532864ed, REQ-REL-6fd2a7b2, REQ-SEC-de8b193c, REQ-FUNC-ce587dc2, REQ-REL-6040e6f3, REQ-PLAT-c98ee5fa, REQ-SEC-936214c0, REQ-REL-e0e85969, REQ-PLAT-e6a39651, REQ-DATA-0b4ecb84, REQ-TEST-6274da69, REQ-SEC-a14ea155
parent:
branch: master
base: 47862fcde11d5af29275ba37b8afedc7a055d888
opened: 2026-09-19

## Done checklist (current stage)

- [ ] diff reviewed against scope, requirements, design, error paths, portability, security, and unrequested work
- [ ] knowledge pages updated for every materially changed component
- [ ] requirements re-read: status, prose, verification, paths, and links true
- [ ] manual or inspection verification completed and recorded where applicable
- [ ] final evolution report refreshed after the last candidate edit
- [ ] final `check --strict --run` result recorded truthfully after all edits
- [ ] remaining warnings and pre-existing failures explained
- [ ] `stage: deliver`

## Frame

Problem: The delivered packaging foundation validates exact targets, immutable
inputs and a network-isolated preparation receipt, but deliberately has an
empty incomplete lock and no `.deb`, `.run`, Raspberry Pi image or release
builder. Running it therefore cannot create the packages the user needs.
Desired outcome: Complete the remaining product prerequisites and packaging
implementation so one connected, verified acquisition can feed a fully offline
build of local candidate artifacts for Ubuntu 24.04 amd64 and Raspberry Pi OS
Debian 13 arm64, with deterministic metadata, lifecycle safety and honest
acceptance evidence.
Actors: Release engineer fetching and building; administrator installing,
upgrading, recovering or removing NILO; appliance first-boot service; release
reviewer tracing inputs, licences, checksums and tests.
Primary scenario: release engineer resolves and fetches every pinned input ->
offline release action builds target binaries, GUI, production integrations,
`.deb`, `.run` and Pi image -> verifiers exercise safe install/lifecycle and
emit a release manifest -> hardware evidence remains `not_run` where equipment
is unavailable.
Examples: Ubuntu produces `nilo_0.8.0_amd64.deb` and a matching target-specific
offline `.run`; Raspberry Pi produces the arm64 pair plus a compressed image;
an existing installed integration upgrades atomically while preserving enabled
state; first boot consumes a root-only administrator password file.
Counterexamples: A preparation receipt is not called a package; an artifact
must not fetch at install time; the test-only `fake_python` runtime, development
credentials, host paths or unverified cache bytes must not enter a release; a
cross-build must not be reported as physical boot/GPIO/USB acceptance.
In scope: Finish the unresolved PKG01-PKG03 inventories, locks and reproducible
inputs needed by real builders; implement PKG04-PKG05 product prerequisites;
implement PKG06 Debian packages, PKG07 target-specific offline installers,
PKG08 Raspberry Pi image construction, and PKG09 hardware-independent release
gates/documentation; create local ignored candidate artifacts when prerequisites
can be acquired; test failure, rollback, traversal, isolation and deterministic
metadata boundaries.
Out of scope: Public/customer distribution, signing-key generation or custody,
version assignment, Git staging/commit/push/PR, online updater, added OS
targets, automatic hardware enablement, and claims of physical Raspberry Pi or
mini-PC acceptance. A public release remains forbidden without signing and
physical acceptance.
Constraints: Use AEGIS for software changes; stay on `master`; do not commit;
preserve all pre-existing dirty/untracked files; use repository-local tools and
ignored caches; connected acquisition is the only networked phase; package and
install phases fail closed offline; never put a credential on a command line;
no physical target is available; no host-wide installation or persistent host
change is authorized. The earlier foundation candidate remains uncommitted in
the shared working tree and is a prerequisite, not unrelated work.
Success signals: Exact complete locks and provenance; reproducible target
payloads; safe production integration runtime and upgrade; idempotent package
lifecycle; self-verifying offline installers; structurally verified Pi image;
release manifest/SBOM/notices/checksums; no undeclared network access; focused
and repository-wide tests green; generated artifacts named from the existing
canonical version; unavailable physical gates marked `not_run`.
Evidence: STATED - the user wants the remaining implementation continued after
observing that current scripts create no packages; previous instructions require
AEGIS, `master`, no commit, uninterrupted work to delivery and no physical
hardware. OBSERVED - `02_Tools/packaging/README.md` explicitly says no final
artifact builder exists; `config/inputs.lock.json` is empty and incomplete;
running fetch/verify exits 2 and package actions produce no receipt or artifact;
`00_Planning/Packaging_and_Delivery_Plan.md` defines PKG04-PKG09 and twelve
decisions; existing product seams include identity bootstrap, loopback status,
managed integration lifecycle and Sigrok bundle tooling. OBSERVED - the pinned
build-environment task remains blocked by Windows system-volume capacity, and
the current host registers WSL as `Ubuntu`, not `Ubuntu-24.04`. INFERRED - the
remaining work must include product primitives before artifact assembly, and
local container/emulation evidence can cover structural behavior but not
physical acceptance. UNKNOWN - the exact reviewed Pi base-image digest,
redistribution decision for the complete Sigrok closure, final target package
closures and whether current host storage can hold both dependency pools and a
Pi image.
Assumptions: Initial internal delivery uses independently published SHA-256
files but no public unsigned distribution; no historical packaged NILO release
exists, so the first release declares no supported prior package-version matrix
while still testing generic snapshot/rollback machinery; administrator secrets
are explicit root-only file inputs and never repository defaults; SSH and Pi
GPIO recovery ship disabled; BCM17 active-low button and BCM27 active-high LED
remain the documented opt-in recovery wiring; virtualization does not prove USB
hardware acceptance.
Unknowns: Exact external versions/digests and licence/source-offer obligations
must be resolved during connected acquisition; measured integration closure
sizes decide bounded verifier limits; build capacity and emulation availability
may limit local generation; hardware results remain unknown by definition.
Decision-blocking questions: None for starting. The plan and established
security contracts select conservative defaults above. Any external input that
cannot be redistributed, verified or safely pinned becomes an explicit blocker
rather than being silently substituted or omitted.
Proposed behaviors: Build a reviewed inventory and complete input lock; create
self-contained production integration catalogues and atomic upgrades; add
file-based administrator seeding and application-owned maintenance/removal
primitives; build and verify deterministic Debian packages; create safe
self-extracting offline installers with preflight/snapshot/rollback; customise
and structurally verify a pinned Pi image without starting services; emit
checksums, SBOM, notices, provenance and a target acceptance index.
Path and reason: Full path. This introduces security-sensitive CLI and lifecycle
behavior, package/install formats, OS dependency resolution, image mutation,
rollback semantics, external dependencies and release evidence.

| Quick-path criterion | yes | no | evidence |
|---|---:|---:|---|
| No new externally observable behavior |  | x | New CLI, install, upgrade, removal and first-boot behavior |
| No new interface or data format |  | x | Package, installer, snapshot, image and release manifests |
| No new dependency |  | x | Target OS pools, embedded runtimes and Pi base image |
| Existing requirement already covers the behavior |  | x | Existing requirements cover only the foundation and adjacent product seams |
| No security or data-handling change |  | x | Root install, credential consumption, state backup and network cleanup |

## Requirement coverage

### Behavior inventory

| Frame outcome | Actor/system | Trigger or state | Response | Positive case | Boundary or failure | Observable result |
|---|---|---|---|---|---|---|
| Exact target closure | Release engineer/tooling | Connected resolve for one exact target | Pin every required input and provenance | Complete reviewed lock/cache | Missing class, bad signature/digest, unsafe licence | Complete proposal or named incomplete reason |
| Reproducible payload | Offline builder | Same source/cache/target | Normalize and rebuild deterministically | Equal payload digests | Host time/path/order leaks | Equal bytes or refusal |
| Production integrations | Catalogue builder | Build either architecture | Bundle real self-contained runtimes | Sigrok health without system Python/network | Missing/unsafe/unlicensed closure | Verified catalogue or blocked target |
| Installed integration upgrade | Bench Core | New catalogue bundle and idle integration | Atomic stage/migrate/switch/health | State-preserving upgrade | Busy, invalid, migration/health failure | New version or fully restored old version plus audit |
| First administrator | Local first-boot service | Empty store and protected password file | Consume once through identity service | One administrator, source invalidated | Unsafe file, weak secret, populated store, race | Idempotent state or secret-free error |
| Removal ownership | Local administrator | Remove or confirmed purge | Inventory exact owned resources and selected state | Preserve or purge as requested | Unknown ownership/sole path/helper failure | Deterministic report, foreign resources retained |
| Upgrade snapshot | Maintenance tooling | Before package/state mutation | Quiesce, checkpoint, digest and fsync snapshot | Restorable matching state | Busy, space, corruption, version mismatch | Atomic snapshot/restore or fail closed |
| Debian package | Package builder | Exact target payload | Build policy-valid package and idempotent scripts | Install/reinstall/remove lifecycle | Chroot, repeated scripts, mutable payload | Valid target `.deb` or named failure |
| Safe offline installer | Root installer | Matching host and verified `.run` | Safe extract/preflight/local APT install | Offline clean install | Wrong target, traversal, special file, network/removal proposal | Installed candidate or zero preflight mutation |
| Failed installer rollback | Installer/recovery | Failure or interruption after mutation | Restore prior package and matching snapshot | Verified old state resumes | Damaged prior material/reverse failure | Rolled back or stopped recovery-required state |
| Raspberry Pi image | Privileged image builder | Pinned Pi base plus arm64 package | Offline install, first boot config, sanitize, unmount/check/compress | Structurally valid image | Start, stale identity, mount/space/check failure | Checked `.img.xz` or no accepted output |
| Release evidence | Release assembler/reviewer | Candidate artifact tree | Close manifest/SBOM/licence/provenance links | Every byte traceable | Missing/mismatched relationship | Canonical evidence tree or failure |
| Offline acceptance matrix | Verification system | Release gate | Exercise target lifecycles without route | Hardware-independent checks pass | Route/attempt/fault/false hardware claim | Per-check result with physical `not_run` |
| Production payload hygiene | Release verifier | Before acceptance | Scan paths/content/executables | No forbidden material | Credential, fixture, host path, cache or undeclared executable | Safe rule-named refusal |

| Frame outcome | Atomic behavior | Requirement | Positive case | Boundary or failure | Verification | Design decision or no impact |
|---|---|---|---|---|---|---|
| Exact release matrix | Resolve only canonical target metadata | REQ-PLAT-da89bf3b | Both targets resolve | Metadata/target drift | Existing registry tests | Reuse unchanged contract |
| Connected acquisition | Cache only declared immutable bytes | REQ-SEC-628657e1 | Locked object cached | URL/size/digest/interruption | Existing fetch tests | Extend input kinds without weakening trust |
| Cache closure | Reject incomplete/corrupt target cache | REQ-REL-f4a90899 | Sorted verified inventory | Config/object defects | Existing verifier tests | Reuse as builder gate |
| Network isolation | Require observed offline namespace | REQ-SEC-dd110b06 | Preparation in network-none | Interface/route/cache defect | Existing OCI check | Extend receipt into build entry |
| Honest hardware evidence | Keep physical checks `not_run` | REQ-TEST-c442102f | Exact target checklist | Caller/emulation promotion | Existing receipt tests | Reuse across final evidence |
| Exact target closure | Resolve every target release input | REQ-PLAT-4af865a9 | Complete reviewed proposal | Missing/unverified class | Resolver fixture tests | Resolver schema and adoption flow |
| Reproducible payload | Build deterministic target payloads | REQ-NFR-2dc6dc61 | Equal independent builds | Host nondeterminism | Digest comparison | Canonical timestamps/order/owners |
| Production integrations | Bundle self-contained runtimes | REQ-PLAT-532864ed | Both Sigrok bundles run | Unsafe/missing/unlicensed runtime | Networkless health tests | Embedded closure and measured limits |
| Integration lifecycle | Upgrade installed package atomically | REQ-REL-6fd2a7b2 | State preserved | Busy/migration/health failure | Manager/API/CLI tests | Same-filesystem switch plus rollback |
| First boot | Seed administrator from protected file | REQ-SEC-de8b193c | One durable administrator | Unsafe secret/populated store/race | CLI/service tests | File-only secret and post-commit unlink |
| Removal | Inventory and remove only owned state | REQ-FUNC-ce587dc2 | Preserve and purge modes | Unknown owner/management path/failure | Maintenance/helper fixtures | Application-owned cleanup command |
| Upgrade safety | Create/restore matching snapshot | REQ-REL-6040e6f3 | Integrity-checked restore | Busy/space/damage/version mismatch | Snapshot fault tests | Versioned manifest and retention two |
| Debian payload | Build lifecycle-safe `.deb` | REQ-PLAT-c98ee5fa | Both target packages valid | Chroot/idempotence/policy failure | Package policy/lifecycle tests | Debian filesystem and four units |
| Existing-host delivery | Install safe offline `.run` | REQ-SEC-936214c0 | Matching networkless install | Wrong target/archive/APT preflight | Parser/container tests | Shell plus deterministic tar.xz |
| Failure recovery | Roll back failed installer | REQ-REL-e0e85969 | Prior package/state restored | Any transaction phase/reverse failure | Fault/interruption matrix | Durable phase record, rollback on resume |
| Pi delivery | Build sanitized image | REQ-PLAT-e6a39651 | Checked compressed image | Start/identity/mount/space/check failure | Fixture image/static inspection | Pi4 16GB, SSH/GPIO disabled |
| Traceability | Emit release evidence closure | REQ-DATA-0b4ecb84 | Manifest/SBOM/notices/provenance close | Missing/mismatched link | Schema/semantic verifier | SHA-256 internal delivery, public blocked |
| Acceptance | Verify artifacts offline | REQ-TEST-6274da69 | Lifecycle matrix passes | Route/fault/false physical evidence | Network-none target/emulation gate | Physical rows remain `not_run` |
| Payload boundary | Exclude secrets/dev fixtures | REQ-SEC-a14ea155 | Clean scan | One forbidden rule at a time | Streaming scan fixtures | Reviewed policy; no secret echo |

## Design

### Requirements coverage

- The existing target registry, immutable cache, closure and network-isolation
  gates remain the only admission path for external bytes
  (REQ-PLAT-da89bf3b, REQ-SEC-628657e1, REQ-REL-f4a90899,
  REQ-SEC-dd110b06). A connected resolver writes a proposal; only an explicitly
  complete adopted lock can enter an offline build (REQ-PLAT-4af865a9).
- A standard-library Python release builder owns canonical JSON, normalized
  tar metadata, path policy, SHA-256 closure and orchestration. Thin POSIX
  wrappers expose stable operator commands (REQ-NFR-2dc6dc61,
  REQ-DATA-0b4ecb84, REQ-SEC-a14ea155).
- Existing appliance cross-build and integration catalogue builders provide
  immutable payload inputs. Production catalogues must contain their executable
  runtime closure and pass networkless health checks; fixture runtimes cannot
  enter a release (REQ-PLAT-532864ed).
- Bench Core owns integration upgrade state. It admits `upgrade` only for an
  installed, idle package and preserves the installed/enabled state around a
  same-filesystem staged replacement (REQ-REL-6fd2a7b2).
- The `nilo` binary owns identity seeding, inventory, snapshot, restore, remove
  and purge semantics. Packaging scripts call those product commands instead
  of editing SQLite or guessing application state (REQ-SEC-de8b193c,
  REQ-FUNC-ce587dc2, REQ-REL-6040e6f3).
- The Debian builder owns immutable filesystem layout and maintainer scripts;
  the offline installer owns host preflight and transaction state; the image
  builder owns base-image mutation and sanitization (REQ-PLAT-c98ee5fa,
  REQ-SEC-936214c0, REQ-REL-e0e85969, REQ-PLAT-e6a39651).
- Artifact verifiers emit machine-readable pass/fail/not-run observations.
  Physical Pi, GPIO and USB rows cannot become `pass` without physical evidence
  (REQ-TEST-c442102f, REQ-TEST-6274da69).

### Components, interfaces and state

- `release_builder.py` exposes `payload`, `deb`, `installer`, `image`,
  `release`, `verify` and `scan` subcommands. Every command takes explicit
  repository/cache/output/target inputs, writes through a sibling temporary
  path, fsyncs where state is durable, and atomically replaces only its named
  output. JSON schemas use `schema: 1`, sorted keys and newline termination.
- Target payloads are canonical tar archives rooted at `payload/` with a
  manifest of path, type, mode, size and digest. Their timestamps derive only
  from `SOURCE_DATE_EPOCH`; uid/gid and owner names are zero/empty
  (REQ-NFR-2dc6dc61).
- Debian staging maps the payload to `/usr/bin`, `/usr/libexec/nilo`,
  `/usr/share/nilo`, `/usr/lib/nilo`, `/usr/lib/systemd/system` and `/etc/nilo`.
  Mutable directories are created by idempotent scripts, not stored in the
  archive; `policy-rc.d` and chroot detection prevent image-build starts
  (REQ-PLAT-c98ee5fa).
- A `.run` is a reviewed POSIX launcher followed by one deterministic tar.xz
  payload. The launcher validates its embedded offset/digest and delegates to
  the installer driver. Preflight is read-only; mutation starts only after
  target, disk, archive, package and snapshot checks pass
  (REQ-SEC-936214c0).
- Installer transaction state is a fsynced phase record under
  `/var/lib/nilo/transactions`. Phases are preflight, snapshotted, package
  staged, switched, verified, committed. Retry resumes or reverses the last
  durable phase; reverse failure stops in `recovery-required`
  (REQ-REL-e0e85969).
- Application snapshots live below `/var/lib/nilo/snapshots/<transaction>` and
  contain a versioned manifest plus hashes. Creation quiesces writers before a
  SQLite checkpoint and file copy; restore verifies every hash before replacing
  state. The two newest committed snapshots are retained
  (REQ-REL-6040e6f3).
- Administrator seeding accepts an absolute non-symlink password file with no
  group/other permission bits. The existing identity service performs the
  atomic bootstrap; successful durable creation is followed by unlink, while
  every refusal avoids secret output (REQ-SEC-de8b193c).
- Pi image mutation uses loop devices only after capacity and ownership checks,
  installs through local APT with service start blocked, writes disabled SSH
  and GPIO defaults, removes machine identity/leases/logs, unmounts in reverse
  order, checks the filesystem, then compresses deterministically
  (REQ-PLAT-e6a39651).

### Execution, recovery and trust boundaries

Connected input resolution/fetching is separate from an offline build. The
offline build first verifies lock/cache closure, then builds one target payload,
its `.deb`, its `.run`, and for arm64 its image. Release assembly scans all
bytes before closing checksums, SBOM, notices, provenance and acceptance JSON.
No installer or verifier is allowed to resolve a URL. External archives,
package maintainer scripts, embedded paths, credentials, device nodes and
symlinks crossing an extraction root are untrusted. Builds use per-target
locks; product maintenance uses a data-directory lock; package upgrade refuses
busy integrations after the specified timeout. Temporary paths are removed on
failure, while prior committed outputs remain untouched.

Compatibility is additive: existing interactive `admin bootstrap`, package
install/enable/disable/restart/uninstall actions and on-disk state remain valid.
The first package declares no historical version upgrade matrix, but snapshot
and rollback behavior is tested with fixture prior packages. Internal candidate
delivery publishes SHA-256 evidence; public distribution remains blocked
without signing and physical acceptance.

### Test seams and observability

Python unit tests build minimal amd64/arm64 payload fixtures twice, inspect
Debian/control and `.run` structure, inject traversal/special-file/digest/phase
faults, inspect an image root fixture, close evidence graphs and exercise every
forbidden-content rule. Go command tests cover protected-file seeding and
maintenance snapshot/inventory behavior. Bench manager tests cover idle upgrade,
busy refusal and rollback. OCI/network-namespace tests prove builders and
installers do not need a route. Every operation emits concise stderr plus a
canonical receipt; secrets are never fields.

### Alternatives, files, risks and open decisions

A collection of independent shell builders was rejected because canonical
archive parsing, traversal defense and evidence closure need one testable
implementation. Editing SQLite from maintainer scripts was rejected because it
would duplicate application invariants. Shipping an OS-level Python/Sigrok
dependency was rejected because it breaks the offline/self-contained contract.

Expected additions are under `02_Tools/packaging/{build,debian,installer,image,
verify,schemas,config}`, `02_Tools/tests/packaging`, and
`05_Software/scripts/tests/test_release_packaging.py`, plus focused Go files in
`cmd/nilo`, `internal/maintenance` and `internal/bench`. The exact Pi base
digest, target APT closure and Sigrok redistribution/source-offer evidence stay
open connected-acquisition decisions; builders fail closed while they are
unresolved. Host disk capacity may prevent generation of full local target
artifacts but does not relax implementation or fixture verification. Physical
hardware results remain `not_run`.

### Proposed architecture - TASK-edf96bec

```text
connected resolver -> reviewed lock -> immutable cache
                                      |
                               offline verifier
                                      |
 source + cache -> payload builder -> target payload
                                      |      |
                                  deb builder |-> installer builder
                                      |
                           arm64 image builder
                                      |
 artifact scan -> evidence closure -> offline acceptance index

 installer/package scripts -> nilo maintenance + nilo seed-admin
 Bench package manager -> staged integration upgrade -> health/rollback
```

## Log

- 2026-09-19 frame: User explicitly continued the remaining packaging implementation after observed package creation failed. Full-path scope covers completion of PKG01-PKG09 software and local candidate artifact generation, with autopilot to delivery on `master`, no commit/publication, and physical acceptance kept `not_run`.
- 2026-09-19 specify: Reused the five active target/cache/offline/hardware-evidence requirements and created fourteen draft requirements with detailed specs for complete resolution, reproducibility, production integrations and upgrades, first-boot/removal/snapshot primitives, Debian/installer/Pi artifacts, rollback, release evidence, offline acceptance and payload hygiene; no requirement was retired.
- 2026-09-19 design: Constrained the remaining work to a canonical offline release builder, product-owned maintenance primitives, lifecycle-specific artifact adapters and fail-closed evidence gates; detailed formats and failure semantics remain in the linked specs.
- 2026-09-19 build red: REQ-SEC-de8b193c command=`02_Tools/runtime/go/1.27.0/go/bin/go.exe test ./cmd/nilo -run TestSeedAdministrator -count=1`; exit=1; expected missing behavior=the `seed-admin` command is absent and prints top-level usage.
- 2026-09-19 build green: REQ-SEC-de8b193c command=`02_Tools/runtime/go/1.27.0/go/bin/go.exe test ./cmd/nilo -run TestSeedAdministrator -count=1`; exit=0; protected-file seeding, refusal preservation and symlink rejection pass.
- 2026-09-19 build red: REQ-NFR-2dc6dc61, REQ-SEC-a14ea155 command=`python -m unittest scripts.tests.test_release_packaging.DeterministicPayloadTests -v`; exit=1; expected missing behavior=the canonical payload builder and production-content scanner do not exist.
- 2026-09-19 build green: REQ-NFR-2dc6dc61, REQ-SEC-a14ea155 command=`python -m unittest scripts.tests.test_release_packaging.DeterministicPayloadTests -v`; exit=0; deterministic manifests, normalized metadata, mutable-state refusal and forbidden-content rules pass (Windows symlink case skipped for unavailable privilege; Linux proof remains in validation).
- 2026-09-19 build red: REQ-PLAT-c98ee5fa command=`python -m unittest scripts.tests.test_release_packaging.DebianPackageTests -v`; exit=1; expected missing behavior=the release builder has no Debian package construction interface.
- 2026-09-19 build green: REQ-PLAT-c98ee5fa command=`python -m unittest scripts.tests.test_release_packaging.DebianPackageTests -v`; exit=0; reproducible Debian archives, target matching, policy-aware idempotent scripts, conffiles and hardened units pass structural inspection.
- 2026-09-19 build red: REQ-SEC-936214c0, REQ-REL-e0e85969 command=`python -m unittest scripts.tests.test_release_packaging.OfflineInstallerTests -v`; exit=1; expected missing behavior=the release builder has no self-extracting offline installer or rollback transaction driver.
- 2026-09-19 build green: REQ-SEC-936214c0, REQ-REL-e0e85969 command=`python -m unittest scripts.tests.test_release_packaging.OfflineInstallerTests -v`; exit=0; deterministic embedded archives, digest/path/type validation, target preflight, no-download APT and durable rollback phases pass structural tests (Windows symlink case skipped for unavailable privilege; Linux proof remains in validation).
- 2026-09-19 build red: REQ-PLAT-e6a39651 command=`python -m unittest scripts.tests.test_release_packaging.RaspberryPiImageTests -v`; exit=1; expected missing behavior=the release builder has no validated Raspberry Pi image-builder adapter or sanitizing implementation.
- 2026-09-19 build green: REQ-PLAT-e6a39651 command=`python -m unittest scripts.tests.test_release_packaging.RaspberryPiImageTests -v`; exit=0; exact arm64 input validation, protected credential handoff, offline chroot policy, identity/SSH cleanup, filesystem checking and honest `not_run` hardware evidence pass fixture-backed tests.
- 2026-09-19 build red: REQ-DATA-0b4ecb84, REQ-TEST-6274da69, REQ-TEST-c442102f command=`python -m unittest scripts.tests.test_release_packaging.ReleaseEvidenceTests -v`; exit=1; expected missing behavior=release evidence assembly and semantic closure verification interfaces do not exist.
- 2026-09-19 build red: REQ-FUNC-ce587dc2, REQ-REL-6040e6f3 command=`02_Tools/runtime/go/1.27.0/go/bin/go.exe test ./cmd/nilo -run TestMaintenance -count=1`; exit=1; expected missing behavior=the `maintenance` command group and owned-state snapshot/removal implementation are absent.
- 2026-09-19 build green: REQ-DATA-0b4ecb84, REQ-TEST-6274da69, REQ-TEST-c442102f command=`python -m unittest scripts.tests.test_release_packaging.ReleaseEvidenceTests -v`; exit=0; five-artifact closure, SPDX evidence, notices, provenance, checksums, tamper refusal and physical `not_run` semantics pass.
- 2026-09-19 build green: REQ-FUNC-ce587dc2, REQ-REL-6040e6f3 command=`02_Tools/runtime/go/1.27.0/go/bin/go.exe test ./cmd/nilo -run TestMaintenance -count=1`; exit=0; owned-file inventory, protected snapshot/restore, tamper refusal, preserve-state removal and confirmed local purge pass.
- 2026-09-19 build red: REQ-REL-6fd2a7b2 command=`02_Tools/runtime/go/1.27.0/go/bin/go.exe test ./internal/bench -run TestIntegrationUpgrade -count=1`; exit=1; expected missing behavior=the managed integration lifecycle rejects the new `upgrade` action as invalid.
- 2026-09-19 build green: REQ-REL-6fd2a7b2 command=`02_Tools/runtime/go/1.27.0/go/bin/go.exe test ./internal/bench -run TestIntegrationUpgrade -count=1`; exit=0; same-filesystem stage/switch, state preservation, bounded busy refusal, health checking and filesystem rollback pass.
- 2026-09-19 build red: REQ-PLAT-532864ed command=`python -m unittest scripts.tests.test_release_packaging.ProductionIntegrationTests -v`; exit=1; expected missing behavior=the Sigrok bundle builder still interprets its third input as a Python version and invokes package resolution instead of embedding a declared production runtime.
- 2026-09-19 build red: REQ-PLAT-4af865a9 command=`python -m unittest scripts.tests.test_packaging_inputs.PackagingInputTests.test_connected_resolver_creates_a_complete_reviewable_lock_and_cache scripts.tests.test_packaging_inputs.PackagingInputTests.test_connected_resolver_names_missing_classes_and_rejects_unsafe_metadata -v`; exit=1; expected missing behavior=required release-input classes and connected resolver/proposal interfaces are absent.
- 2026-09-19 build green: REQ-PLAT-532864ed command=`python -m unittest scripts.tests.test_release_packaging.ProductionIntegrationTests -v`; exit=0; explicit target runtime closure, embedded Python/Sigrok backend, notices, measured limit and fixture refusal pass without package resolution.
- 2026-09-19 build green: REQ-PLAT-4af865a9 command=`python -m unittest scripts.tests.test_packaging_inputs.PackagingInputTests.test_connected_resolver_creates_a_complete_reviewable_lock_and_cache scripts.tests.test_packaging_inputs.PackagingInputTests.test_connected_resolver_names_missing_classes_and_rejects_unsafe_metadata -v`; exit=0; required-class closure, connected digest observation, cache population, incomplete proposals and strict metadata pass.
- 2026-09-19 build red: REQ-PLAT-c98ee5fa, REQ-SEC-936214c0, REQ-PLAT-e6a39651, REQ-DATA-0b4ecb84, REQ-TEST-6274da69 command=`python -m unittest scripts.tests.test_release_packaging.OperatorScriptTests -v`; exit=1; expected missing behavior=operator-facing artifact build/verification entrypoints are absent and the README still says no final builder exists.
- 2026-09-19 build red: REQ-PLAT-c98ee5fa, REQ-REL-e0e85969 command=`python -m unittest scripts.tests.test_release_packaging.DebianPackageTests.test_debian_package_is_deterministic_and_policy_safe -v`; exit=1; expected missing behavior=the package does not yet contain the boot-time interrupted-installer recovery helper referenced by its recovery unit.
- 2026-09-19 build green: REQ-PLAT-c98ee5fa, REQ-SEC-936214c0, REQ-PLAT-e6a39651, REQ-DATA-0b4ecb84, REQ-TEST-6274da69 command=`python -m unittest scripts.tests.test_release_packaging.OperatorScriptTests -v`; exit=0; every artifact and verifier has an offline shell entrypoint and the connected/offline operator flow is documented.
- 2026-09-19 build green: REQ-PLAT-c98ee5fa, REQ-REL-e0e85969 command=`python -m unittest scripts.tests.test_release_packaging.DebianPackageTests scripts.tests.test_release_packaging.OfflineInstallerTests -v`; exit=0; package-owned boot recovery validates fixed-path journals, reinstalls retained prior packages, restores matching snapshots, and leaves a fail-closed recovery-required state when reversal fails.
- 2026-09-19 build red: REQ-REL-6040e6f3 command=`02_Tools/runtime/go/1.27.0/go/bin/go.exe test ./cmd/nilo -run TestMaintenanceSnapshotAndRestore -count=1`; exit=1; expected missing behavior=snapshot manifests do not yet bind state to source and target application versions.
- 2026-09-19 build green: REQ-REL-6040e6f3 command=`02_Tools/runtime/go/1.27.0/go/bin/go.exe test ./cmd/nilo -run TestMaintenanceSnapshotAndRestore -count=1`; exit=0; manifests bind source/target application versions and restore verifies SQLite checkpoint, integrity and schema identity before replacement.
- 2026-09-19 build proof: `python -m unittest 05_Software/scripts/tests/test_packaging_inputs.py 05_Software/scripts/tests/test_release_packaging.py -v` passed 33 tests on Windows with four platform-permission skips; the identical Ubuntu/WSL command passed all 33, including FIFO and symlink defenses. Focused Go packages `./cmd/nilo ./internal/bench ./internal/maintenance ./internal/store/sqlite` passed, and `sh -n` accepted every packaging shell entrypoint.
- 2026-09-19 build evolution: refreshed the saved report after the final behavior increment; its seven paths describe changes since the latest required anchor. Reconciled that incremental view with `git status` and the task-base diff so all earlier anchored packaging, command, test, requirement and spec files remain accounted for; no unexpected candidate file was accepted.
- 2026-09-19 build check: `python .agentic/aegis.py check --full` returned exit 0 with no errors. Remaining candidate-related warnings are stage-zone attribution caused by the successive per-increment anchors and two lexical `secret-in-diff` matches on protected password-file handling/tests; inspection found no credential value. All stale paths for this task are corrected. Other stale-path, sidecar, old-task and scope-drift warnings pre-exist this task and remain out of scope.
- 2026-09-19 validate review round 1: specification compliance=`changes_required`; implementation quality=`changes_required`. Both reviewers independently found unearned software acceptance passes, a non-starting packaged nethelper, unsafe/incomplete installer rollback and missing offline/cache admission. Specification review additionally recorded incomplete resolver provenance, dummy Sigrok runtime acceptance, non-atomic snapshots, incomplete seed-admin idempotence/ownership, missing Pi/evidence closure, incomplete integration migration/audit handling, scanner blind spots and naming/documentation drift. Quality review additionally recorded arbitrary-directory purge, dependency-filename shell injection, clean-install rollback failure and recovery-unit write confinement. Production input selection and physical target tests remain external gates; the findings are software gaps and route to build.
- 2026-09-19 build red (round-1 findings): REQ-PLAT-c98ee5fa, REQ-SEC-936214c0, REQ-DATA-0b4ecb84, REQ-TEST-6274da69 command=`python -m unittest 05_Software.scripts.tests.test_release_packaging.DebianPackageTests.test_debian_package_is_deterministic_and_policy_safe 05_Software.scripts.tests.test_release_packaging.OfflineInstallerTests.test_offline_installer_refuses_shell_active_package_names 05_Software.scripts.tests.test_release_packaging.ReleaseEvidenceTests.test_release_evidence_refuses_missing_executed_software_acceptance -v`; exit=1; expected missing behavior=the package has no socket-activated viable helper, unsafe dependency names enter root shell, and release assembly fabricates software passes without supplied evidence.
- 2026-09-19 build red (round-1 findings): REQ-FUNC-ce587dc2 command=`02_Tools/runtime/go/1.27.0/go/bin/go.exe test ./cmd/nilo -run TestMaintenancePurgeRefusesAnUnownedDirectory -count=1`; exit=1; expected missing behavior=confirmed purge accepts and recursively deletes an arbitrary unowned absolute directory.
- 2026-09-19 build red (round-1 findings): REQ-SEC-de8b193c, REQ-REL-6040e6f3 command=`02_Tools/runtime/go/1.27.0/go/bin/go.exe test ./cmd/nilo -run 'Test(SeedAdministrator(IsIdempotentForTheSameEnabledAdministrator|BoundsTheProtectedFileBeforeBootstrap)|MaintenanceSnapshotRefusesANonNILODatabase)' -count=1`; exit=1; expected missing behavior=re-seeding the same enabled administrator is refused and a non-SQLite `nilo.db` is accepted as snapshot state (the 1024-byte boundary already fails closed in the identity policy).
- 2026-09-19 build red (round-1 findings): REQ-SEC-a14ea155, REQ-NFR-2dc6dc61 command=`python -m unittest 05_Software.scripts.tests.test_release_packaging.DeterministicPayloadTests.test_release_scanner_checks_large_and_binary_files 05_Software.scripts.tests.test_release_packaging.DebianPackageTests.test_debian_package_refuses_payload_bytes_not_bound_by_the_manifest -v`; exit=1; expected missing behavior=large/binary content bypasses forbidden markers and Debian construction accepts payload bytes that disagree with their manifest.
- 2026-09-19 build red (round-1 findings): REQ-SEC-936214c0, REQ-REL-e0e85969 command=`python -m unittest 05_Software.scripts.tests.test_release_packaging.OfflineInstallerTests.test_offline_installer_is_self_verifying_and_deterministic -v`; exit=1; expected missing behavior=the generated root installer lacks a lock, NetworkManager/local-only APT preflight, complete transaction phases, clean-install removal and post-install service health verification.
- 2026-09-19 build green (round-1 findings): `python -m unittest 05_Software/scripts/tests/test_packaging_inputs.py 05_Software/scripts/tests/test_release_packaging.py -v` passed all 37 tests under WSL/Linux, including FIFO and symlink defenses, and passed on Windows with four platform/permission skips; focused Go packages `./cmd/nilo ./internal/bench ./internal/maintenance ./internal/store/sqlite` passed. The fixes cover service-unit viability, installer package-name and transaction safety, executed acceptance evidence, ownership-bound purge, idempotent bounded seeding, SQLite identity, streaming content scans and payload-manifest binding.
- 2026-09-19 build deviation: the round-1 fix anchor was absent on resume, so AEGIS could not reconstruct its per-fix evolution; the durable red records, task-base Git diff and fresh focused proof are retained, and a new anchor at `a61f20d` plus a zero-change evolution report now protects subsequent rounds.
- 2026-09-19 build check: `python .agentic/aegis.py check` returned exit 0 with no errors. Thirteen warnings are pre-existing repository bookkeeping findings (five stale-path groups, tracked task sidecars and unrelated task scope/checklist drift); three manual-verification notes belong to unrelated requirements. Review-fix tests were added to all affected requirement links.
- 2026-09-20 validate review round 2: specification compliance=`changes_required`; implementation quality=`changes_required` on candidate `ca9489b480b00d5b19c3648c2fb7d752b9666e158bfd93430a96e5410f1da03c`. Both reviewers found that artifacts are not digest-bound to admitted cache inputs and release acceptance can be self-attested. Recorded corrections also cover executable target acceptance; signed repository/dependency resolution; owned network/process removal; quiesced metadata-preserving atomic snapshots; installer migration, health and rollback ordering; Pi first-boot growth/identity provisioning; executed Sigrok health; and crash-recoverable integration activation. External production-input selection and physical acceptance remain gates rather than software fixes. Candidate changes route to build and invalidate both verdicts.
- 2026-09-20 build red: REQ-PLAT-532864ed command=`python -m unittest scripts.tests.test_release_packaging.ProductionIntegrationTests -v`; exit=1; expected missing behavior=the Sigrok bundle builder accepts manifest-claimed health and exposes no runner with which to execute the three declared target-runtime checks.
- 2026-09-20 build green: REQ-PLAT-532864ed command=`python -m unittest scripts.tests.test_release_packaging.ProductionIntegrationTests -v`; exit=0; the builder executes the fixed runtime-root Python/version/driver-scan commands with a confined PATH, rejects failed execution before publication and records hashed derived health evidence.
- 2026-09-20 build red: REQ-REL-e0e85969 command=`python -m unittest scripts.tests.test_release_packaging.OfflineInstallerTests.test_offline_installer_is_self_verifying_and_deterministic -v`; exit=1; expected missing behavior=the installer substitutes inventory for migration, has no loopback/helper health probes, restores packages before stopping new services and cannot distinguish a clean install from an upgrade when deciding whether to restart the old service.
- 2026-09-20 build green: REQ-REL-e0e85969 commands=`python -m unittest scripts.tests.test_release_packaging.OfflineInstallerTests.test_offline_installer_is_self_verifying_and_deterministic -v`; `../../02_Tools/env/with-go.sh go test ./cmd/nilo -run TestMaintenanceMigrateAppliesAndVerifiesTheCurrentSchema -count=1`; exits=0. The installer now applies and verifies schema migrations before startup, probes the service/helper/loopback GUI and catalogue, stops new units before reverse package work, restarts only a recorded prior NILO installation, and fails recovery with services stopped.
- 2026-09-20 build red: REQ-PLAT-e6a39651 command=`python -m unittest scripts.tests.test_release_packaging.RaspberryPiImageTests.test_image_builder_delegates_only_validated_inputs_and_keeps_hardware_not_run -v`; exit=1; expected missing behavior=the image has no enabled root first-boot preparation unit or marker to grow the deployed filesystem, establish fresh machine identity and assert NetworkManager ownership before NILO starts.
- 2026-09-20 build green: REQ-PLAT-e6a39651 command=`python -m unittest scripts.tests.test_release_packaging.RaspberryPiImageTests scripts.tests.test_release_packaging.DebianPackageTests -v`; exit=0. The image now discovers its root partition rather than assuming partition 2, owns networking through NetworkManager, and gates NILO startup on a root first-boot unit that grows the deployed filesystem, establishes a fresh machine ID and consumes a one-shot marker; physical boot remains `not_run`.
- 2026-09-20 build red: REQ-REL-f4a90899 command=`python -m unittest scripts.tests.test_release_packaging.DeterministicPayloadTests.test_offline_receipt_revalidates_admitted_cache_bytes -v`; exit=1; expected missing behavior=artifact builders trust an offline receipt without rehashing its admitted cache objects, so bytes changed after preparation remain accepted.
- 2026-09-20 build green: REQ-REL-f4a90899 commands=`python -m unittest scripts.tests.test_release_packaging -v`; `python -m unittest scripts.tests.test_packaging_inputs -v`; exits=0. Preparation receipts now bind an absolute cache root; every selected object is revalidated for canonical path, regular-file type, size and digest at build time; release receipts must exactly match the selected lock records; and installer/image external inputs must match admitted bytes.
- 2026-09-20 build red: REQ-REL-6040e6f3 command=`../../02_Tools/env/with-go.sh go test ./cmd/nilo -run TestMaintenanceSnapshotRestorePreservesLinuxFileMetadata -count=1`; exit=1; expected missing behavior=snapshot manifests and copies omit UID, GID and extended attributes, so ownership/ACL/xattr fidelity cannot be verified or restored.
- 2026-09-20 build green (partial correction): REQ-REL-6040e6f3 command=`../../02_Tools/env/with-go.sh go test ./cmd/nilo -run "TestMaintenance(SnapshotRestorePreservesLinuxFileMetadata|RestoreRejectsSnapshotMetadataDamage|SnapshotAndRestoreVerifyApplicationState|SnapshotRefusesANonNILODatabase)" -count=1`; exit=0. Snapshot schema 2 records, verifies and restores UID/GID, mode, user xattrs and POSIX ACL metadata, rejects metadata tampering, and raises free-space headroom to the larger of 256 MiB or ten percent. Set-atomic restore remains in the recorded review correction set.
- 2026-09-20 build green (snapshot correction complete): REQ-REL-6040e6f3 command=`../../02_Tools/env/with-go.sh go test ./cmd/nilo -run TestMaintenance -count=1`; exit=0. Restore now builds and fsyncs a complete sibling state tree, carries retained snapshot/transaction evidence into it, atomically exchanges the whole data directory with Linux `renameat2(RENAME_EXCHANGE)`, and removes the displaced tree only after releasing the maintenance lock.
- 2026-09-20 build red: REQ-REL-6fd2a7b2 command=`../../02_Tools/env/with-go.sh go test ./internal/bench -run TestIntegrationUpgradeRecoversCandidateActivationInterruptedBeforeStateCommit -count=1`; exit=1; expected missing behavior=a crash after the candidate rename but before database commit leaves the new bundle active against the prior digest because activation has no durable journal or startup reconciliation.
- 2026-09-20 build green: REQ-REL-6fd2a7b2 command=`../../02_Tools/env/with-go.sh go test ./internal/bench -run "TestIntegrationUpgrade(RecoversCandidateActivationInterruptedBeforeStateCommit|PreservesStateRefusesBusyAndRollsBackFailedHealth)" -count=1`; exit=0. Upgrade staging is fsynced, each rename/database boundary is durably journaled, rollback renames are fsynced, and manager startup reconciles an active candidate against the authoritative database digest before serving it.
- 2026-09-20 build red: REQ-TEST-6274da69 command=`python -m unittest scripts.tests.test_release_packaging.ReleaseEvidenceTests.test_release_evidence_rejects_self_attested_passes_without_executing_checks -v`; exit=1; expected missing behavior=release assembly has no executable acceptance-plan boundary and continues to trust caller-supplied `status: passed` records plus caller-authored evidence files.
- 2026-09-20 build green: REQ-TEST-6274da69 command=`python -m unittest scripts.tests.test_release_packaging.ReleaseEvidenceTests -v`; exit=0. Release assembly now accepts only schema-2 command plans, executes every software check itself in an observed loopback-only/no-route Linux namespace, derives pass from exit zero, rejects self-attestation, and closes result/stdout/stderr digests into the release; physical rows remain derived `not_run` entries.
- 2026-09-20 build red: REQ-PLAT-4af865a9 command=`python -m unittest scripts.tests.test_packaging_inputs.PackagingInputTests.test_connected_resolver_creates_a_complete_reviewable_lock_and_cache -v`; exit=1; expected missing behavior=the resolver has no signed-APT verification boundary and cannot derive or lock per-package provenance/closure for an OS dependency pool.
- 2026-09-20 build green: REQ-PLAT-4af865a9 commands=`python -m unittest scripts.tests.test_packaging_inputs -v`; `python -m unittest scripts.tests.test_release_packaging -v`; exits=0 with two Windows permission/platform skips in each relevant suite. OS pools now require a digest-pinned keyring and admitted full fingerprints, executed `gpgv` verification of `InRelease`, signed `Packages` binding, exact `.deb` digest/control/architecture checks and an exact recursive root/base closure; the lock, offline receipt and SPDX SBOM retain per-package provenance.
- 2026-09-20 build red: REQ-FUNC-ce587dc2 command=`../../02_Tools/env/with-go.sh go test ./internal/nethelper ./cmd/nilo -run 'Test(CleanupOwnedRetainsTheActiveManagementTunnelUntilLocalConfirmation|MaintenanceInventoryRemoveAndConfirmedPurge)' -count=1`; exit=1; expected missing behavior=the helper exposes no ownership-record cleanup transaction or management-path confirmation policy, and confirmed purge deletes state without first creating a retained verified backup.
- 2026-09-20 build green: REQ-FUNC-ce587dc2 commands=`../../02_Tools/env/with-go.sh go test ./internal/nethelper ./cmd/nilo -run 'Test(CleanupOwned|Maintenance)' -count=1`; `python -m unittest scripts.tests.test_release_packaging.DebianPackageTests.test_debian_package_is_deterministic_and_policy_safe -v`; exits=0. Removal now stops core before invoking a typed root/helper cleanup while the helper remains available; cleanup acts on exact durable tunnel/forwarding/commissioning ownership, retains foreign objects and an active possible management tunnel without local confirmation, reports and leaves failed records retriable, and confirmed purge creates a verified sibling backup before external/helper/application state deletion.
- 2026-09-20 build proof: Under WSL/Linux, `python3 -m unittest scripts.tests.test_packaging_inputs scripts.tests.test_release_packaging -v` passed all 44 tests without skips, `go test ./... -count=1` passed every Go package, and `sh -n` accepted all packaging shell entrypoints and installed `nilo-*` scripts. Windows `py_compile` accepted the changed packaging Python. `git diff --check` reported no whitespace errors.
- 2026-09-20 build evolution: Before the final anchor refresh, the saved task evolution identified 36 candidate paths (12 added, 24 modified); those paths were reconciled against the task-base Git diff and working-tree inventory. The final knowledge-only edits are separately visible in the semantic diff. No unrelated candidate file was accepted; the untracked Camera plan remains outside this task.
- 2026-09-20 build deviation: the round-two red tests were durably recorded, but the required AEGIS anchor was not refreshed between those red observations and their implementations. The final diff, pre-refresh 36-path evolution report and full verification remain available, but per-increment pre-implementation fingerprint provenance cannot be reconstructed retroactively. A post-implementation anchor was recorded only to protect subsequent validation edits.
- 2026-09-20 build check: `python .agentic/aegis.py check --full` returned exit 0. Remaining warnings are five unrelated stale requirement paths, unrelated manual-verification notes, tracked fingerprint sidecars, old-task checklist/scope drift and candidate stage-zone attribution caused by anchor history; no current-task path or test link is missing.
- 2026-09-20 validate knowledge: Reviewed the semantic candidate and updated `repository-toolchain.md`, `bench-integrations.md`, `integration-platform.md` and `bench-core-foundation.md` for signed package admission, executed offline acceptance, target-runtime health, crash-recoverable integration activation, owned helper cleanup and metadata-preserving atomic restore.
- 2026-09-20 validate requirements: Reread all nineteen linked requirements. Every status is `active`; every declared `affected_paths` entry and linked-test file exists; the new trust, cleanup, snapshot and recovery regressions are linked to their governing requirements.
- 2026-09-20 validate review round 3: specification compliance=`changes_required`; implementation quality=`changes_required` on candidate `aff0f796a9105fc3ac24d385290d5ae7a58324fd9d298418d75e3c404bb5d4e7`, unchanged before/after both read-only reviews. Recorded corrections: bind named acceptance to non-substitutable repository-owned target checks and exact artifacts; structurally verify every release artifact; resolve Debian dependency versions/architectures/alternatives/virtual providers with target APT semantics; reuse the canonical strict signed-APT lock contract at assembly; retain the exact simulated prior package transaction independent of the incidental APT cache; fsync installer recovery bytes, lists and parent-directory journal renames; and compare all snapshot metadata before reusing a purge backup. External production-input selection and physical acceptance remain gates rather than software findings. Candidate changes route to build and invalidate both verdicts.
- 2026-09-20 build red (round-3 findings): REQ-PLAT-4af865a9 command=`python -m unittest scripts.tests.test_packaging_inputs.PackagingInputTests.test_connected_resolver_rejects_unsatisfied_debian_version_relationships -v`; exit=1; expected missing behavior=a signed pool containing `Depends: fixture-dependency (>= 2.0)` is accepted with version 1.0 because dependency relationships are reduced to names.
- 2026-09-20 build red (round-3 findings): REQ-REL-e0e85969 command=`python -m unittest scripts.tests.test_release_packaging.OfflineInstallerTests.test_offline_installer_is_self_verifying_and_deterministic -v`; exit=1; expected missing behavior=rollback depends on `/var/cache/apt/archives`, has no simulated changed-package inventory/download retention, and does not fsync its transaction directory.
- 2026-09-20 build red (round-3 findings): REQ-TEST-6274da69, REQ-DATA-0b4ecb84, REQ-REL-f4a90899 command=`python -m unittest scripts.tests.test_release_packaging.ReleaseEvidenceTests.test_release_evidence_rejects_substitute_acceptance_commands scripts.tests.test_release_packaging.ReleaseEvidenceTests.test_release_evidence_rejects_malformed_artifacts_and_weakened_apt_provenance -v`; exit=1; expected missing behavior=acceptance has no artifact-bound repository-owned runner, assembly reaches caller-selected commands before artifact structure, and the release lock accepts weakened signed-APT provenance.
- 2026-09-20 build red (round-3 findings): REQ-REL-6040e6f3 command=`go test ./cmd/nilo -run TestMaintenancePurgeBackupReuseRejectsMetadataOnlyDrift -count=1`; exit=1; expected missing behavior=an existing purge backup is reused after a current file's mode changes without its bytes changing.
- 2026-09-20 build green (round-3 findings): `python3 -m unittest 05_Software.scripts.tests.test_packaging_inputs 05_Software.scripts.tests.test_release_packaging -v` passed all 49 tests under WSL/Linux, the Windows command passed all 49 with four expected POSIX/privilege skips, `go test ./... -count=1` passed every Go package under WSL, all packaging shell entrypoints passed `sh -n`, and `git diff --check` passed. The correction uses a fixed repository-owned exact-target acceptance dispatcher with byte-bound primary and independent rebuild artifacts; structurally verifies every artifact and exact signed installer pool; reparses and revalidates canonical signed-APT inputs with Debian relationship, architecture, alternative and virtual-provider semantics; retains and fsyncs the simulated changed-package transaction independently of the APT cache; and refuses metadata-drifted purge-backup reuse.
- 2026-09-20 build evolution: refreshed the post-round-three report after the final correction and reconciled its packaging, acceptance, maintenance and test paths with the task-base diff; all additions are within the recorded correction set and the unrelated Camera plan remains outside this task.
- 2026-09-20 build check: scoped AEGIS check returned exit 0 with no errors. Its sixteen warnings and three notes are pre-existing stale requirement paths, tracked task sidecars, two other tasks' delivery checklists, already-recorded password-fixture lexical matches, and source-coverage bookkeeping outside the round-three correction; no current requirement path or linked test is missing.
- 2026-09-20 validate review round 4: specification compliance=`changes_required`; implementation quality=`changes_required` on candidate `16d4d9d9fbdd0caf9686a82ad1deb5195ee2dd695d0781bf792ae6576b6cf3d5`, unchanged before and after both read-only reviews. Recorded corrections: make every artifact builder require the trusted input lock and admit extracted packages through its canonical signed pool; exercise real isolated package, distinct-version rollback and integration lifecycle checks; authenticate exact installer/package executable structure before root execution; prove independent builds from distinct builder receipts rather than directory copies; sync recovery-journal temporaries before rename; preserve/compare directory metadata; preserve Sigrok runtime execute modes and exercise the real runner; force stable APT output locale; and publish Pi images through a fsynced sibling atomic rename. External input selection, redistribution evidence, host capacity and physical acceptance remain gates rather than software findings. Candidate changes route to build and invalidate both verdicts.
- 2026-09-20 build red (round-4 packaging findings): REQ-SEC-936214c0, REQ-REL-e0e85969, REQ-PLAT-e6a39651, REQ-TEST-6274da69, REQ-REL-f4a90899 command=`python -m unittest 05_Software.scripts.tests.test_release_packaging.DebianPackageTests.test_debian_package_is_deterministic_and_policy_safe 05_Software.scripts.tests.test_release_packaging.OfflineInstallerTests.test_offline_installer_is_self_verifying_and_deterministic 05_Software.scripts.tests.test_release_packaging.OfflineInstallerTests.test_installer_structure_rejects_a_replaced_root_driver 05_Software.scripts.tests.test_release_packaging.RaspberryPiImageTests.test_image_builder_delegates_only_validated_inputs_and_keeps_hardware_not_run 05_Software.scripts.tests.test_release_packaging.ReleaseEvidenceTests.test_reproducibility_requires_two_independent_build_receipts 05_Software.scripts.tests.test_release_packaging.OperatorScriptTests.test_fixed_acceptance_executes_distinct_package_and_integration_lifecycles 05_Software.scripts.tests.test_release_packaging.OperatorScriptTests.test_artifact_builders_require_a_trusted_input_lock -v`; exit=1; expected missing behavior=trusted-lock admission, authenticated installer logic, durable recovery replacement, stable APT parsing, atomic image publication, independent build receipts and substantive lifecycle dispatch are absent.
- 2026-09-20 build red (round-4 metadata finding): REQ-REL-6040e6f3 command=`../../02_Tools/env/with-go.sh go test ./cmd/nilo -run TestMaintenancePurgeBackupReuseRejectsMetadataOnlyDrift -count=1`; exit=1; expected missing behavior=directory-only mode drift is omitted from the snapshot manifest and stale purge-backup comparison.
- 2026-09-20 build observation (round-4 execute-mode finding): REQ-REL-6fd2a7b2 command=`../../02_Tools/env/with-go.sh go test ./internal/bench -run TestPackageLifecyclePreservesEmbeddedRuntimeExecuteModes -count=1`; exit=0. The installed and upgraded embedded runtime files retained execute bits on Linux, disproving the reviewer's `os.CopyFS` mode-loss premise; the new regression retains that observed contract and no production change is warranted for this finding.
- 2026-09-20 build green (round-4 findings): Windows and WSL commands=`python -m unittest 05_Software.scripts.tests.test_packaging_inputs 05_Software.scripts.tests.test_release_packaging`; exits=0, 54 tests with four expected Windows-only skips and no Linux skips. Artifact builders now require a matching trusted lock; signed-pool package bytes are admitted through locked package records; Debian and installer executable structure is repository-authenticated; reproducibility binds distinct builder receipts; fixed lifecycle checks perform direct package install/reinstall, a distinct-prior-version rollback with state proof, and authenticated Sigrok install/upgrade; APT uses a stable locale; recovery and image publication fsync before sibling atomic replacement.
- 2026-09-20 build green (round-4 metadata/mode findings): WSL commands=`go test ./internal/bench -run TestPackageLifecyclePreservesEmbeddedRuntimeExecuteModes -count=1`; `go test ./cmd/nilo -run 'TestMaintenance(SnapshotRestorePreservesLinuxFileMetadata|PurgeBackupReuseRejectsMetadataOnlyDrift|RestoreRejectsSnapshotMetadataDamage)' -count=1`; exits=0. Snapshot schema 3 preserves and verifies complete directory metadata including default ACLs and rejects directory-only drift; installed and upgraded runtime executables are invoked successfully.
- 2026-09-20 build red (native portability correction): REQ-REL-6040e6f3, REQ-REL-6fd2a7b2 command=`go test ./...` with the pinned Windows Go runtime; exit=1. The new durability walks used read-only handles that Windows refuses to flush, and portable restore attempted to rename a directory while its cooperative lock remained open.
- 2026-09-20 build green (native portability correction): pinned Windows commands=`go test ./cmd/nilo -run TestMaintenance -count=1`; `go test ./internal/bench -run TestIntegrationUpgradePreservesStateRefusesBusyAndRollsBackFailedHealth -count=1`; `go test ./...`; exits=0. Platform-specific file flush helpers use writable handles on Windows and read handles on Unix; production Linux retains the locked atomic directory exchange, while the documented portable fallback releases and removes its cooperative lock around the best-effort rename sequence.
- 2026-09-20 build proof (round 4): WSL `go test ./...` passed every package; the two portability-sensitive selectors passed again under WSL after their correction; all changed packaging shell entrypoints passed `sh -n`; Windows `py_compile` passed; `git diff --check` passed. `python .agentic/aegis.py check` returned no errors; warnings are the existing stale-path/task-sidecar/other-task stage-zone set plus the expected lexical secret heuristic for generated acceptance credentials.
- 2026-09-20 build evolution (round 4): post-anchor evolution records the bounded packaging, acceptance, maintenance and regression-test correction set. Requirement links were extended for trusted-lock enforcement, independent build receipts, fixed lifecycle execution, Debian maintainer authentication and runtime execute-mode proof; all governing requirement statuses remain active.
- 2026-09-20 validate review round 5: specification compliance=`changes_required`; implementation quality=`changes_required` on frozen candidate `a6f61cb5e066802399cc7562e5b346445e1bf6d28bfbeb28022b8c0b1f3401ce`, unchanged before and after both read-only reviews. The final bounded correction set is: dispatch lifecycle acceptance through externally established exact-target disposable containers/emulators; make build orchestration emit non-self-attested independent receipts; execute a successful prior-version upgrade plus clean-install and phase/interruption rollback coverage; authenticate Debian structure before privileged image execution; remove the installer bootstrap dependency on host `dpkg-repack` and clean preflight residue; keep a portable restore exclusion marker and reverse partial rename failure; observe offline Sigrok-health isolation; preserve the Pi credential's root-only source through a narrow seeding handoff; and bind source offers plus component-complete SBOM/licence relationships to admitted inputs. Production-input selection, redistribution approval, host capacity, signing and physical acceptance remain external gates. Candidate changes route to build, invalidate both verdicts, and increment the next review to the final allowed round 6.
- 2026-09-20 build red (round-5 packaging findings): REQ-SEC-936214c0, REQ-REL-e0e85969, REQ-PLAT-e6a39651, REQ-SEC-de8b193c, REQ-TEST-6274da69, REQ-DATA-0b4ecb84, REQ-PLAT-532864ed command=`python -m unittest 05_Software.scripts.tests.test_release_packaging.OfflineInstallerTests.test_offline_installer_is_self_verifying_and_deterministic 05_Software.scripts.tests.test_release_packaging.OfflineInstallerTests.test_installer_archive_rejects_duplicate_members 05_Software.scripts.tests.test_release_packaging.RaspberryPiImageTests.test_image_builder_authenticates_package_before_privileged_execution 05_Software.scripts.tests.test_release_packaging.RaspberryPiImageTests.test_image_credential_remains_root_only_until_narrow_handoff 05_Software.scripts.tests.test_release_packaging.ReleaseEvidenceTests.test_build_set_receipts_are_emitted_from_artifact_builder_receipts 05_Software.scripts.tests.test_release_packaging.ReleaseEvidenceTests.test_release_rejects_source_offer_bytes_not_bound_to_the_admitted_lock 05_Software.scripts.tests.test_release_packaging.ProductionIntegrationTests.test_sigrok_bundle_rejects_health_checks_in_a_routed_namespace 05_Software.scripts.tests.test_release_packaging.OperatorScriptTests.test_fixed_acceptance_executes_distinct_package_and_integration_lifecycles -v`; exit=1; expected missing behavior=the final review's authenticated structure, private rollback tool, empty-state rollback, exact-target harness, emitted receipt, locked source/SBOM, observed isolation, root-only handoff and lifecycle-matrix contracts are absent.
- 2026-09-20 build red (round-5 portable restore finding): REQ-REL-6040e6f3 command=`& '..\..\02_Tools\env\with-go.ps1' go test ./internal/maintenance -run TestPortableDirectoryExchangeReversesAFailedFinalRename -count=1`; exit=1; expected missing behavior=the portable three-rename exchange has no testable rollback primitive and can expose restored state after its final rename fails.

- 2026-09-20 build green (round-5 findings): Windows and WSL commands=`python -m unittest 05_Software.scripts.tests.test_packaging_inputs 05_Software.scripts.tests.test_release_packaging`; exits=0, 60 tests with four expected Windows-only privilege/POSIX skips and no Linux skips. The bounded correction rejects duplicate installer members, authenticates package structure before privileged image mutation, keeps the Pi credential root-only until a narrow one-time handoff, executes the complete upgrade/clean/phase/interruption lifecycle matrix only through fresh externally established digest-pinned target containers, derives build-set evidence from five artifact-builder receipts, binds source offers and runtime component licensing to admitted lock inputs, and observes the Sigrok health namespace route state before and after execution.
- 2026-09-20 build green (round-5 portable restore): pinned Windows command=`& '..\\..\\02_Tools\\env\\with-go.ps1' go test ./internal/maintenance -run TestPortableDirectoryExchangeReversesAFailedFinalRename -count=1`; exit=0. The portable three-rename helper reverses the first two moves when the final move fails, and restore holds a sibling exclusion marker across the unlocked replacement window.
- 2026-09-20 build proof (round 5): pinned Windows and WSL commands=`go test ./... -count=1`; exits=0 across every Go package, including the long command and integration-supervisor suites. All changed shell entrypoints passed `sh -n` under WSL; Windows `py_compile` accepted the changed packaging Python; `git diff --check` reported no whitespace errors.
- 2026-09-20 validate review round 6: specification compliance=`changes_required`; implementation quality=`changes_required` on final allowed candidate `61fd2f9b69c2081b1b83aefcc1acb69d7236475873650dfe83454a1f8f9538ed`, unchanged before and after both independent read-only reviews. Both reviewers found that build-set receipts can still be minted post hoc from copied artifacts rather than being established before and bound through each builder invocation, and that the rollback matrix does not inject failures inside dependency/package transactions or interruptions at every required durable marker. Quality review additionally found that root recovery consumes journals and retained packages below the service-account-writable `/var/lib/nilo` parent. Specification review additionally found that a failed first-boot seed leaves the unprivileged `/run` credential handoff behind, exact target image/kernel identity is not retained in acceptance evidence, SPDX components lack validated deliverable relationships, and the generated installer launcher does not itself authenticate its complete unique manifest before extraction/execution. No cycle remains under `[review].max_fix_cycles = 5`; AEGIS requires the task to remain in validate with `result: blocked` and `needs_user: yes` until an explicit human Ruling changes the review policy or scope. External production-input selection, redistribution approval, host capacity, signing and physical acceptance remain separate gates.
- 2026-09-20 validate ruling: The user explicitly authorized one additional correction cycle. The bounded review counter is reset once under RULING-005; the recorded round-six findings route to build, both prior verdicts are invalidated by candidate edits, and the next paired review is reset round 1.
- 2026-09-20 build red (authorized correction): REQ-SEC-936214c0, REQ-REL-e0e85969, REQ-SEC-de8b193c, REQ-NFR-2dc6dc61, REQ-TEST-6274da69, REQ-DATA-0b4ecb84 command=`python -m unittest 05_Software.scripts.tests.test_release_packaging.OfflineInstallerTests.test_generated_installer_launcher_rejects_duplicate_members_before_extraction 05_Software.scripts.tests.test_release_packaging.RaspberryPiImageTests.test_image_credential_remains_root_only_until_narrow_handoff 05_Software.scripts.tests.test_release_packaging.RaspberryPiImageTests.test_privileged_recovery_state_is_outside_the_service_writable_data_root 05_Software.scripts.tests.test_release_packaging.ReleaseEvidenceTests.test_build_set_receipts_reject_post_hoc_receipts_for_copied_artifacts 05_Software.scripts.tests.test_release_packaging.ReleaseEvidenceTests.test_acceptance_retains_exact_target_image_and_kernel_identity 05_Software.scripts.tests.test_release_packaging.ReleaseEvidenceTests.test_release_sbom_requires_artifact_dependency_relationships 05_Software.scripts.tests.test_release_packaging.OperatorScriptTests.test_fixed_acceptance_executes_distinct_package_and_integration_lifecycles -v`; exit=1 with one expected Windows-only launcher skip; expected missing behavior=root-sealed recovery/bootstrap state, unconditional failed-handoff cleanup, pre-established builder provenance, exact target identity evidence, validated SPDX relationships, launcher-side unique manifest validation, real in-transaction faults and per-marker interruption rows are absent.
- 2026-09-20 build green (authorized correction): Windows command=`python -m unittest discover -s 05_Software/scripts/tests -p "test_*.py"`; exit=0, 183 tests with five expected platform skips. WSL command=`NILO_WSL_HOST_VOLUME=/mnt/f python3 -m unittest discover -s 05_Software/scripts/tests -p 'test_*.py'`; exit=0, 183 tests with 29 expected host/platform skips, including the executable launcher duplicate-member defense. All changed packaging shell entrypoints passed `sh -n`, and changed packaging Python passed `py_compile`. The correction places privileged recovery and bootstrap state below root-only `/var/lib/nilo-installer`, removes failed first-boot handoffs, binds artifact receipts during pre-established builder invocations, retains exact target identity, validates complete SPDX relationship graphs, self-authenticates the launcher inventory and manifest, enters real package-operation faults, and interrupts every durable marker.
- 2026-09-20 build evolution (authorized correction): the post-red anchor report contains exactly seventeen modified packaging, acceptance and regression-test files in the bounded review-finding set; requirement and repository-toolchain documentation were reconciled separately under AEGIS state and no unexpected candidate path was accepted.
- 2026-09-20 build check (authorized correction): `python .agentic/aegis.py check --task TASK-edf96bec` returned exit 0 with no errors. Its eighteen warnings and three notes are the existing stale requirements, tracked sidecars, other tasks' unticked delivery checklists, pre-existing source-coverage bookkeeping, and lexical password/credential heuristics in protected credential handling and tests; inspection found no credential value or new unresolved task defect.
- 2026-09-20 validate semantic review (authorized correction): before either reset reviewer finished, coordinator review found that the lifecycle matrix interrupts the eight forward phases but the active REQ-REL-e0e85969 spec requires interruption coverage for every phase, including `committed`, `rolling_back`, `rolled_back` and `recovery-required`. Both in-flight reviews were stopped before verdict and the candidate routes back to build.
- 2026-09-20 build red (authorized correction, complete phase matrix): REQ-REL-e0e85969, REQ-TEST-6274da69 command=`python -m unittest 05_Software.scripts.tests.test_release_packaging.OperatorScriptTests.test_fixed_acceptance_executes_distinct_package_and_integration_lifecycles -v`; exit=1; expected missing behavior=the acceptance driver has no recoverable forced `recovery-required` seam and the lifecycle dispatcher lacks interruption cases for the four rollback/terminal durable phases.
- 2026-09-20 build green (authorized correction, complete phase matrix): focused lifecycle selector, full Windows discovery and full WSL discovery commands exited 0; each full suite ran 183 tests with five expected Windows and 29 expected WSL host/platform skips. Acceptance now interrupts `committed`, `rolling_back`, `rolled_back` and `recovery-required` in addition to all forward phases, verifies committed-new versus restored-prior state as appropriate, and uses a transient acceptance-only rollback failure to prove recovery from the durable recovery-required marker. Shell syntax, Python compilation and `git diff --check` pass.
- 2026-09-20 build evolution/check (authorized correction, complete phase matrix): post-red evolution contains only the three expected executable acceptance files; the active reliability spec's phase spelling was reconciled. Scoped AEGIS check again returned exit 0 with the same eighteen explained warnings and three unrelated manual-verification notes.
- 2026-09-20 validate reset review round 1: specification compliance=`changes_required`; implementation quality=`changes_required` on candidate `23bc5ebb518dcc7990dfd7f2511bbef879b6c0e1adc339cee7a6fd5b4f11cb81`, unchanged before and after both independent read-only reviews. Confirmed remaining findings: current-schema aggregate/artifact receipts can be hand-authored for copied artifacts because acceptance does not revalidate the underlying receipts or independently execute the two builds; recovery phase parsing and replacement use `[a-z-]*` and therefore cannot process the durable underscore phases `rolling_back` and `rolled_back`; clean-install rollback persists `rolled_back` before removing candidate-created state, so interruption can leave new state while recovery treats the transaction as terminal; and the lifecycle, target-identity, credential and recovery regressions are structural/fabricated-runner tests rather than retained execution of the disposable exact-target harness, which allowed the recovery defects to pass. The other round-six corrections were confirmed present by inspection. The single extra correction cycle granted by RULING-005 is exhausted, so no further candidate edit or strict final gate is authorized without a new human Ruling.
- 2026-09-20 validate ruling: The user explicitly authorized another correction cycle. RULING-006 resets the bounded review counter once for the four reset-round findings, routes them to build, and invalidates both prior verdicts on the first candidate edit.
- 2026-09-20 build red (RULING-006 recovery findings): REQ-REL-e0e85969, REQ-TEST-6274da69 command=`wsl.exe -d Ubuntu-24.04 --cd /mnt/f/2026_Networked_Intelligent_Lab_Orchestrator python3 -m unittest 05_Software.scripts.tests.test_release_packaging.OfflineInstallerTests.test_recovery_helper_executes_underscore_and_clean_terminal_phases -v`; exit=1; expected missing behavior=the recovery helper cannot execute against a disposable root, underscore phases are not recoverable, and terminal clean rollback does not idempotently remove candidate-created state.
- 2026-09-20 build red (RULING-006 reproducibility finding): REQ-NFR-2dc6dc61, REQ-TEST-6274da69 command=`python -m unittest 05_Software.scripts.tests.test_release_packaging.ReleaseEvidenceTests.test_release_evidence_rejects_non_reproducible_artifact_bytes 05_Software.scripts.tests.test_release_packaging.ReleaseEvidenceTests.test_reproducibility_runs_two_fixed_builds_in_fresh_roots 05_Software.scripts.tests.test_release_packaging.ReleaseEvidenceTests.test_build_set_receipts_reject_post_hoc_receipts_for_copied_artifacts -v`; exit=1; expected missing behavior=no fixed acceptance-owned two-build executor exists and copied artifacts with hand-authored current-schema receipts are accepted as independent builds.
- 2026-09-20 build green (RULING-006 findings): focused Windows reproducibility selectors and the WSL executable recovery selector exited 0. The fixed acceptance dispatcher now performs two complete `build-candidate-set.sh` invocations in fresh roots from lock-bound candidate inputs, validates embedded schema-3 artifact receipts and rejects caller-supplied copied-artifact receipts; recovery parses underscore phases and removes clean-install data before or idempotently after its terminal marker.
- 2026-09-20 build executable observation (RULING-006 exact-target evidence): command=`wsl.exe -d Ubuntu-24.04 --cd /mnt/f/2026_Networked_Intelligent_Lab_Orchestrator python3 -m unittest 05_Software.scripts.tests.test_release_packaging.OperatorScriptTests.test_target_acceptance_harness_executes_isolated_exact_target_flow -v`; exit=0. The actual target-harness shell selected the digest-pinned Ubuntu identity, requested `--network none`, exact platform and privileged system container semantics, emitted observed image/kernel/OS/architecture identity and delegated the lifecycle row with the disposable-target marker.
- 2026-09-20 build proof (RULING-006): Windows and WSL commands=`python -m unittest discover -s 05_Software/scripts/tests -p "test_*.py"`; exits=0, 185 tests with seven expected Windows and 29 expected WSL host/platform skips. Packaging Python passed `py_compile`, changed shell entrypoints passed WSL `sh -n`, and `git diff --check` passed.
- 2026-09-20 build evolution/check (RULING-006): post-red evolution records the six expected reproducibility, dispatcher, documentation and test files; the earlier anchored recovery increment remains reconciled through its focused executable proof and current task-base diff. Scoped AEGIS check exited 0 with the same eighteen explained stale-path/sidecar/other-task coverage and lexical credential warnings plus three unrelated manual-verification notes; no credential value or new unresolved candidate defect was found. All governing requirement statuses remain active and links now include fresh-build, forged-receipt, executable recovery and target-harness proof.
- 2026-09-20 validate review round 1 (RULING-006): specification compliance=`changes_required`; implementation quality=`changes_required` on candidate `e7c0b02f5f5be67e2d9e050bfdc14e939e8008bd3de0299fb920c1324b6a771d`, unchanged before and after both independent read-only reviews. Confirmed corrections are: retire a clean `rolled_back` transaction after its one-time cleanup so a later valid installation cannot be deleted; make the two acceptance-owned builds rebuild source-derived binaries, GUI, integration bundles and payloads rather than repackaging caller-provided payload archives; fully validate embedded artifact receipt command/target/version/input bindings; and stop claiming the fake-engine harness smoke test as exact-target lifecycle evidence. Neither Podman nor Docker nor configured digest-pinned target images are present locally, so real exact-target lifecycle execution remains an unmet validation gate rather than proof. The bounded RULING-006 loop has remaining cycles; software findings route to build and both verdicts are invalidated on the first edit.
- 2026-09-20 build red (RULING-006 review round 1): REQ-REL-e0e85969, REQ-NFR-2dc6dc61, REQ-TEST-6274da69 command=`python -m unittest 05_Software.scripts.tests.test_release_packaging.ReleaseEvidenceTests.test_reproducibility_runs_two_fixed_builds_in_fresh_roots 05_Software.scripts.tests.test_release_packaging.ReleaseEvidenceTests.test_build_set_receipts_reject_malformed_embedded_builder_metadata -v`; exit=1; and command=`wsl.exe -d Ubuntu-24.04 --cd /mnt/f/2026_Networked_Intelligent_Lab_Orchestrator python3 -m unittest 05_Software.scripts.tests.test_release_packaging.OfflineInstallerTests.test_recovery_helper_executes_underscore_and_clean_terminal_phases -v`; exit=1. Expected missing behavior=candidate inputs still require prebuilt payloads rather than source/cache inputs, schema-3 validation accepts a Debian receipt claiming the installer command, and terminal rollback journals are not retired before later recovery runs.
- 2026-09-20 build green (RULING-006 review round 1): focused Windows reproducibility/receipt selectors and the WSL executable recovery selector exited 0. Completed rollback transactions now move to root-sealed history after cleanup so repeated recovery cannot delete later state; embedded receipts require the exact artifact command, target, version and current input bytes; and each acceptance-owned rebuild starts from the identified Git revision plus lock-bound Go, npm and per-target runtime archives to rebuild binaries, GUI, integrations, payloads and all five artifacts.
- 2026-09-20 build proof (RULING-006 review round 1): complete Windows and WSL script discovery commands exited 0 with 186 tests, seven expected Windows skips and 29 expected WSL host/platform skips. The changed packaging module passed on both hosts without Linux skips; shell syntax, Python compilation and `git diff --check` pass. Post-red evolution contains the six expected implementation/document/test files. Scoped AEGIS check exits 0 with the same eighteen explained warnings and three unrelated notes.
- 2026-09-20 validation evidence correction: the earlier Log line labelled the fake-engine harness smoke test as exact-target evidence. It proves only construction of the digest-pinned, network-none container command and capture interface; it does not execute the nested dispatcher or lifecycle implementation. The test and requirement links now name that narrower role. No Podman/Docker engine or configured pinned images are available, so real exact-target lifecycle execution remains an unmet gate and is not counted as acceptance proof.
- 2026-09-20 validate review round 2 (RULING-006): specification compliance=`changes_required`; implementation quality=`changes_required` on candidate `b8632f28cd0862b49ac2aa1d020f9eb8b8ab4f10acd30d82fee3dd0cb1c7abb2`, unchanged before and after both independent reviews. Both confirmed rollback retirement, source-layer rebuild orchestration, strict receipt metadata and the honest fake-engine evidence correction. Remaining software findings are that authentic installer receipts include signed-pool bindings rejected by the new exact-input rule, and the source build still reads mutable checkout `VERSION`/`with-go.sh` outside the archived revision. The active exact-target lifecycle requirement also remains unverified without the unavailable engine/images; this is an explicit blocker, not a software pass. The bounded loop has remaining cycles, so the two software defects route to build and invalidate both verdicts on edit.
- 2026-09-20 build red (RULING-006 review round 2): REQ-NFR-2dc6dc61, REQ-TEST-6274da69 command=`python -m unittest 05_Software.scripts.tests.test_release_packaging.ReleaseEvidenceTests.test_build_set_receipts_are_emitted_from_artifact_builder_receipts 05_Software.scripts.tests.test_release_packaging.OperatorScriptTests.test_candidate_builder_uses_archived_revision_and_pinned_tools -v`; exit=1; expected missing behavior=production-shaped installer receipts with a `pool/*` binding are rejected, and the candidate builder reads mutable checkout version/wrapper files rather than archived-revision metadata with pinned tool version checks.
- 2026-09-20 build green (RULING-006 review round 2): the same focused receipt/source-builder selectors exit 0; full packaging modules pass 48 tests on both Windows (five expected skips) and WSL (no skips). Installer and image receipts now require and validate non-empty `pool/*` bindings, while the candidate builder reads version/toolchain policy from the archived revision, refuses mismatched Go/Node/npm versions and executes build-environment tools without a mutable checkout wrapper. Shell syntax, Python compilation and `git diff --check` pass.
- 2026-09-20 build evolution/check (RULING-006 review round 2): post-red evolution contains exactly the candidate builder, receipt validator and regression test; scoped AEGIS check exits 0 with the unchanged eighteen explained warnings and three unrelated notes.
- 2026-09-20 validate review round 3 (RULING-006): specification compliance=`changes_required`; implementation quality=`approved` on candidate `cc4c477bbac74acf6da424038b5cfcb36878b4cf7da24ad986ff9c70d88b14a1`, unchanged before and after both independent reviews. Both found no remaining software defect in the bounded correction set and confirmed production-shaped pool bindings plus archived-revision version/tool selection. Specification approval is blocked because active `REQ-TEST-6274da69` requires a real network-none exact-target lifecycle run and `REQ-NFR-2dc6dc61` still lacks an actual two-build run with complete production caches. This host has no engine, pinned images or complete locked inputs, so no in-scope candidate edit can create the missing evidence. The strict final gate is not run because both current verdicts are not approved.

## Rulings

- RULING-001 | date=2026-09-19 | decision=Continue the remaining implementation through AEGIS in autopilot to delivery | reason=The user explicitly requested continuation after approving the foundation handoff and had already required uninterrupted AEGIS execution | source=user | supersedes=none
- RULING-002 | date=2026-09-19 | decision=Remain on `master` and leave all work unstaged and uncommitted | reason=The user's existing Git constraint has not been revoked | source=user | supersedes=none
- RULING-003 | date=2026-09-19 | decision=Implement all hardware-independent PKG01-PKG09 behavior but never infer physical acceptance | reason=The user stated Raspberry Pi and mini-PC hardware are unavailable | source=user | supersedes=none
- RULING-004 | date=2026-09-19 | decision=Treat the validated uncommitted TASK-4703beac foundation as an in-worktree prerequisite | reason=The approved no-commit handoff intentionally left those files in this same checkout | source=user | supersedes=none
- RULING-005 | date=2026-09-20 | decision=Reset the bounded review counter once and authorize one additional correction cycle for the final round-six findings | reason=The user explicitly authorized another correction cycle after both final-round reviews required changes | source=user | supersedes=none
- RULING-006 | date=2026-09-20 | decision=Reset the bounded review counter once and authorize one additional correction cycle for the four reset-round findings | reason=The user explicitly authorized another correction cycle after both reset-round reviews required changes | source=user | supersedes=none

## Handoff


current: validate
status: blocked
next: Provide a Podman/Docker-capable offline acceptance host with the reviewed digest-pinned Ubuntu amd64 and Debian 13 arm64 images plus complete locked production inputs, or issue a human Ruling that changes the active verification scope. Then execute the real lifecycle matrix and two complete source builds before resuming review.
must_read: .agentic/tasks/TASK-edf96bec.md, .agentic/steps/4_build.md, .agentic/knowledge/repository-toolchain.md, .agentic/knowledge/bench-integrations.md, .agentic/knowledge/integration-platform.md, .agentic/knowledge/bench-core-foundation.md
blockers: none for the newly authorized software correction cycle; exact external inputs, redistribution evidence, host capacity, signing and physical acceptance remain explicit production-artifact gates.

## Stage report

stage: validate
result: blocked
current: validate
changed: Paired review round 3 found no remaining software defect; implementation quality approved, while specification compliance requires unavailable real exact-target lifecycle and complete production-cache reproducibility execution.
evidence: Both reviewers matched frozen candidate `cc4c477bbac74acf6da424038b5cfcb36878b4cf7da24ad986ff9c70d88b14a1` before and after review. The Log records all focused and cross-platform software proof. Local inspection confirms Podman/Docker, pinned target images and complete production inputs are unavailable.
issues: Active REQ-TEST-6274da69 and REQ-NFR-2dc6dc61 execution evidence cannot be completed on this host. Exact external inputs, redistribution approval, host capacity, signing and physical acceptance remain additional release gates.
next: User supplies the offline acceptance environment/inputs or changes scope by explicit Ruling; then resume validate without treating structural/fake-runner tests as the missing proof.
needs_user: yes
spec_compliance: changes_required
implementation_quality: approved
review_round: 3
