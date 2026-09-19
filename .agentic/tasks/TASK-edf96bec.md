<!--
Copyright (c) 2026 DDP Engineering
This software is licensed under the PolyForm Noncommercial License 1.0.0.
Commercial use requires a separate commercial license.
See the LICENSE.txt in the .agentic folder for full license details.
SPDX-License-Identifier: LicenseRef-PolyForm-Noncommercial-1.0.0
-->


# TASK-edf96bec - Complete offline release packaging and artifact builders

stage: build
type: feature
path: full
interaction: autopilot
requirements: REQ-PLAT-da89bf3b, REQ-SEC-628657e1, REQ-REL-f4a90899, REQ-SEC-dd110b06, REQ-TEST-c442102f, REQ-PLAT-4af865a9, REQ-NFR-2dc6dc61, REQ-PLAT-532864ed, REQ-REL-6fd2a7b2, REQ-SEC-de8b193c, REQ-FUNC-ce587dc2, REQ-REL-6040e6f3, REQ-PLAT-c98ee5fa, REQ-SEC-936214c0, REQ-REL-e0e85969, REQ-PLAT-e6a39651, REQ-DATA-0b4ecb84, REQ-TEST-6274da69, REQ-SEC-a14ea155
parent:
branch: master
base: 47862fcde11d5af29275ba37b8afedc7a055d888
opened: 2026-09-19

## Done checklist (current stage)

- [ ] each automated behavior increment has a pre-implementation red record with requirement IDs, exact selector or command, non-zero exit, and expected failure reason, plus one final focused green result; each manual increment has a consolidated post-implementation observation
- [ ] repository anchor recorded after red and evolution reviewed after implementation
- [ ] implementation makes those tests pass
- [ ] focused project tests green, with their actual scope recorded
- [ ] `affected_paths` and `linked_tests` updated on every requirement
- [ ] `status = "active"` on every requirement this task delivers — or `"retired"`, with the reason in its prose, where this task deliberately removed the behavior
- [ ] `check` clean, or each remaining warning explained in the log
- [ ] `stage: validate`

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

## Rulings

- RULING-001 | date=2026-09-19 | decision=Continue the remaining implementation through AEGIS in autopilot to delivery | reason=The user explicitly requested continuation after approving the foundation handoff and had already required uninterrupted AEGIS execution | source=user | supersedes=none
- RULING-002 | date=2026-09-19 | decision=Remain on `master` and leave all work unstaged and uncommitted | reason=The user's existing Git constraint has not been revoked | source=user | supersedes=none
- RULING-003 | date=2026-09-19 | decision=Implement all hardware-independent PKG01-PKG09 behavior but never infer physical acceptance | reason=The user stated Raspberry Pi and mini-PC hardware are unavailable | source=user | supersedes=none
- RULING-004 | date=2026-09-19 | decision=Treat the validated uncommitted TASK-4703beac foundation as an in-worktree prerequisite | reason=The approved no-commit handoff intentionally left those files in this same checkout | source=user | supersedes=none

## Handoff


current: build
status: ready
next: Fix only the recorded round-1 findings, beginning with destructive-path and root-installer safety, then rerun focused proof and return both changed verdicts to validation round 2.
must_read: .agentic/tasks/TASK-edf96bec.md, .agentic/steps/4_build.md, .agentic/specs/REQ-PLAT-c98ee5fa.md, .agentic/specs/REQ-SEC-936214c0.md, .agentic/specs/REQ-REL-e0e85969.md, .agentic/specs/REQ-TEST-6274da69.md
blockers: none for the bounded software fix cycle; exact external inputs, redistribution evidence, host capacity and physical acceptance remain explicit production-artifact gates.

## Stage report

stage: validate
result: in_progress
current: build
changed: Reconciled the candidate against every linked specification and updated component knowledge before two independent round-1 reviews.
evidence: Specification and implementation-quality reviewers independently inspected the same tree and both returned `changes_required`; their recorded findings identify functional and security gaps rather than unavailable external inputs or hardware.
issues: Root-installer safety, destructive-path ownership, package service viability, offline admission, lifecycle execution/evidence and several contract closures require implementation changes.
next: Fix only the recorded review findings test-first, rerun focused proof, and return to validation round 2 with both prior verdicts invalidated.
needs_user: no
spec_compliance:
implementation_quality:
review_round: 1
