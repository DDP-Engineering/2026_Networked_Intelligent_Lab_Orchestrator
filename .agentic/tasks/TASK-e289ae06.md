<!--
Copyright (c) 2026 DDP Engineering
This software is licensed under the PolyForm Noncommercial License 1.0.0.
Commercial use requires a separate commercial license.
See the LICENSE.txt in the .agentic folder for full license details.
SPDX-License-Identifier: LicenseRef-PolyForm-Noncommercial-1.0.0
-->


# TASK-e289ae06 - MB-10 Modbus offline bundles and appliance acceptance

stage: build
type: feature
path: full
interaction: autopilot
requirements: REQ-PLAT-ffa360cd, REQ-TEST-687a1250, REQ-TEST-93b22e9a
parent:
branch: ddp/modbus-client
base: 7b6cdb52c43d333ebc75a0279029bb015392e001
opened: 2026-10-01

## Done checklist (current stage)

- [x] failing tests first, then implementation
- [ ] requirements activated with linked tests
- [x] focused regression tests pass
- [x] implementation and test evidence recorded
- [ ] stage advanced to validate

## Frame

Problem: The implemented Modbus client cannot run as a verified offline appliance bundle, and its release/target acceptance is unproven.
Desired outcome: Build and verify Modbus bundles for Linux amd64/arm64, exercise deterministic peers and integration gates, and deploy the current candidate to the reachable Raspberry Pi for visible inspection.
Actors: appliance builder, installer, non-root Bench Core/plugin runner, administrator, acceptance operator.
Primary scenario: select Modbus bundle -> builder verifies exact offline inputs and target closure -> installer exposes indexed bundle -> non-root runner loads on target -> browser can inspect/operate a configured endpoint under existing policy.
Examples: universal PyModbus and pySerial wheels plus license sources are admitted by their SHA-256 pins; a changed wheel is rejected.
Counterexamples: a wheel-only Python bundle without its native Pydantic extension or interpreter must not be called a self-contained ARM64 appliance bundle.
In scope: MB-10 issue #37 packaging seam; deterministic simulated TCP/RTU-over-TCP/PTY faults; cross-plugin and browser integration; target architecture and Pi smoke; acceptance evidence and documentation.
Out of scope: unsafe writes to an unnamed real device; publication to Git remote; assigning a release version; changing unrelated working-tree files.
Constraints: same branch; AEGIS autopilot; no commit or push authorization for this ticket; targeted tests during build, full suite only at plan end; deployment requested by user; repository-local tools and pinned/offline inputs; physical acceptance distinct from simulation.
Success signals: indexed hash-verified bundles on both architectures; missing/tampered inputs refused; non-root runner handshake on target; deterministic protocol failures and concurrency reported; container/browser gates pass; Pi exposes Modbus plugin; unmet physical gates named.
Evidence: STATED user requests MB-10, Pi deployment, autopilot, targeted tests until plan end. OBSERVED issue #37 and `00_Planning/Modbus_Plugin_Plan.md` define MB-10; `02_Tools/scripts/build-appliance.sh` lacks Modbus; `02_Tools/scripts/prepare-modbus-dependencies.py` and `05_Software/integrations/modbus/evidence/dependencies.json` pin wheels/notices; `02_Tools/packaging/config/release-inputs.json` has no sources; Pi at 10.205.148.11 is ARM64 and runs 0.12.0. INFERRED a Modbus bundle must be supplied before an end-to-end Pi preview. UNKNOWN physical peer identity and read-only register, whether complete release inputs can be assembled locally.
Assumptions: use existing plugin catalog/import mechanism and no real-device I/O without an identified safe peer.
Unknowns: physical RS-485 adapter/gateway; exact release input availability.
Decision-blocking questions: none for offline packaging and simulated acceptance; physical acceptance awaits a named safe target.
Proposed behaviors: verify complete architecture-specific offline closure; publish deterministic indexed bundle; include in appliance build and release staging; run independent simulated and integrated gates; report physical evidence honestly.
Path and reason: full; new target bundle format/integration and runtime dependency. Quick-path criteria: observable behavior=no, interface/data=no, new dependency=no, existing requirement=no, security/data unchanged=unknown. Existing branch is expressly required by user.

## Requirement coverage

Behavior inventory: Offline bundle selection -> builder -> verified immutable target package or refusal -> inventory/runner oracle; acceptance run -> harness -> distinct deterministic/integrated results or failure -> gate report; target report -> operator -> named observations or pending -> acceptance record.

| Frame outcome | Atomic behavior | Requirement | Positive case | Boundary or failure | Verification | Design decision |
|---|---|---|---|---|---|---|
| Offline target package | Verified indexed bundle | REQ-PLAT-ffa360cd | amd64 and arm64 closure | missing/tampered/wrong architecture | bundle test and target import | atomic bundle and fixed release staging; self-contained target gate pending |
| Integrated confidence | Deterministic and appliance gates | REQ-TEST-687a1250 | peers, contention, browser | injected faults and gate failure | focused tests and final suite | existing peer tests plus canonical container/appliance gates |
| Truthful hardware result | Distinct target evidence | REQ-TEST-93b22e9a | named target checks | absent hardware remains pending | inspect acceptance report | dated Pi delivery record with explicit pending gates |

## Design

REQ-PLAT-ffa360cd: Add `build-modbus-bundle.py` alongside the Serial/Camera builders. It consumes the four pinned offline archives through `prepare-modbus-dependencies.py`, the copied SDK/Pydantic dependency tree, and an optional verified architecture-matched Python runtime. It writes an atomic stage with `runner`, schemas, notices, `runtime.json`, resolved manifest integrity and `files.sha256.json`; no resolver or network is called. Bundle publication rejects an existing destination and malformed or linked inputs. The appliance build gets an explicit Modbus opt-in with architecture-specific inputs. The shared release builder already stages all `integration-bundles/*` from a supplied release payload; no release format change is planned.

REQ-TEST-687a1250: Existing independent peer tests remain the protocol seam. Add focused bundle refusal and runner handshake tests, then execute canonical container, contract, browser and appliance-profile gates once at the end of the plan. Report each observed result. Core owns resource claims; the builder only packages the runner. No new runtime session state is introduced.

REQ-TEST-93b22e9a: Keep named target/physical observations in a dated delivery acceptance record. ARM64 import/runner startup on the Pi is distinct from simulated PTY and TCP results. Missing physical peers remain pending. The user's requested Pi preview uses a reversible staging/rollback approach and never performs unnamed real-device writes.

Interfaces and failure: CLI takes project, dependency-root, wheelhouse, architecture, output and optional runtime-root. Missing or wrong inputs fail before destination publication. The produced runner remains executable by a non-root service and read-only otherwise. `runtime.json` states whether the bundle is self-contained. Rollback restores the prior Pi binaries/GUI/catalog if deployment health fails.

Risks/open decisions: available verified arm64 interpreter closure and full release-input lock are unconfirmed. A bench bundle using target system Python is useful for a Pi preview but cannot satisfy self-contained offline release acceptance. Physical adapter/gateway identity and safe read address are pending. Avoid classifying these as passes.

Expected files: `02_Tools/scripts/build-modbus-bundle.py`, `build-appliance.sh`, `02_Tools/tests/software/test_modbus_bundle.py`, Modbus README, toolchain/Modbus knowledge, changelog and acceptance evidence. No architecture diagram is owed: builder follows existing appliance/bundle flow.

## Log

- 2026-10-01 frame: Full path accepted in autopilot on the existing branch; package implementation and acceptance remain separate from release publication.
- 2026-10-01 specify: Created three requirements for offline bundle, integrated gates, and truthful physical evidence; no prior requirement covered this Modbus package behavior.
- 2026-10-01 design: Selected atomic offline bundle builder and existing release staging seam; target and physical evidence remain distinct, with unavailable inputs recorded as open decisions.
- 2026-10-01 build red: REQ-PLAT-ffa360cd; command=`python -m unittest 02_Tools.tests.software.test_modbus_bundle -v`; exit=1; expected missing behavior=Modbus offline builder absent, so both bundle tests fail loading its required path.
- 2026-10-01 build red: REQ-PLAT-ffa360cd; command=`python -m unittest 02_Tools.tests.software.test_modbus_bundle.ModbusBundleTests.test_appliance_build_has_explicit_modbus_bundle_switch -v`; exit=1; expected missing behavior=appliance build has no Modbus bundle selection.
- 2026-10-01 build green: `python -m unittest 02_Tools.tests.software.test_modbus_bundle -v` exit=0, 4 focused tests; ARM64 bench bundle built from pinned Modbus archives and the verified 292-file Camera dependency inventory, transfer SHA-256 matched on Pi (`c5841a90cb425c4009b5a4cfa58b58032f95219dbe8009d6de5b41e3f1ecd1fa`), non-root Python imported PyModbus 3.15.0/pySerial 3.5/Pydantic 2.13.5/core 2.46.5, runner hello/describe returned correct Modbus contract. This bench bundle uses Pi Python 3.13.5, so self-contained release acceptance remains open.
- 2026-10-01 build green: WSL focused Modbus supervised/runner/scheduler tests 47 passed; Go `TestSerialAndModbusConcurrentPhysicalAdmissionHasOneOwner` and `TestModbus*` passed. No physical peer used.
- 2026-10-01 build red: REQ-PLAT-ffa360cd; command=`python -m unittest 02_Tools.tests.software.test_release_packaging.OperatorScriptTests.test_candidate_builder_uses_archived_revision_and_pinned_tools -v`; exit=1; expected missing behavior=the fixed release candidate builder does not stage Modbus on amd64/arm64.
- 2026-10-01 build green: fixed candidate release builder now stages Modbus for both targets from the build image's pinned transport archives, Camera SDK/Pydantic closure and Sigrok target runtime; focused candidate script test and `sh -n` passed. No full release artifact was built because `release-inputs.json` has no sources and fixed candidate construction uses committed Git revisions.
- 2026-10-01 manual observation: REQ-TEST-93b22e9a; inspected `07_Delivery/NILO_MB10_Modbus_Pi_Preview_20261001.md` against Pi service, bundle, TLS/API, backup and token-revocation checks. It distinguishes verified ARM64 bench startup from pending self-contained release and unnamed physical RS-485/gateway checks.
- 2026-10-01 evolution: 10 product/delivery paths changed since anchor; all belong to MB-10 and no previously present user file changed. `aegis check --full` reported no errors; remaining warnings are pre-existing task state and unrelated user additions.
- 2026-10-01 validation limitation: The canonical isolated full suite passed 224 tooling tests and all Go packages except the unchanged supervisor `one_MiB_overflow` case, which returned a pipe-closure error; exit=1. The focused case then passed 5/5 repetitions. The Go failure stopped the canonical command before Python/frontend/contract routing. Per the user's one-final-suite preference, a second full run and redundant `check --strict --run` are not used as a substitute for this nonzero result. Focused Modbus Python (47), frontend (21), contract (11), bundle (4 plus one Linux link guard), amd64/arm64 bench handshakes and Pi preview provide separate evidence; final appliance-browser result is pending.
- 2026-10-01 build review: Offline wheelhouse link traversal was rejected before license preparation; the new Linux refusal test passed and the four Windows-focused tests passed with that one Linux-only test skipped. The fixed candidate builder must explicitly assemble Modbus for both targets before generic payload staging; this resolved a gap in the original design note, with no package format change.
- 2026-10-01 validation evidence: amd64 bench runner `hello`/`describe` passed under WSL2; ARM64 bench runner and Pi catalog passed earlier. Modbus frontend 21/21, integration contract 11/11, and disposable appliance-profile helper/Core/browser stage 46/46 passed. The latter browser suite does not install the Modbus package; the combined browser-to-Modbus-runner scenario remains unproved. Pi `nilo.service` remained active and the deployed Core/bundle SHA-256 values matched the delivery record. Final evolution listed the 10 expected product/delivery paths; `check --task TASK-e289ae06 --strict` exited 0 with no errors and 14 unrelated/pre-existing warnings. `--run` was not invoked because it repeats the full suite, whose last result is recorded as nonzero.
- 2026-10-01 acceptance boundary: Requirements REQ-PLAT-ffa360cd and REQ-TEST-687a1250 remain draft because no complete release-input lock or verified self-contained amd64/arm64 runtime was available for artifact/target acceptance, and the combined Modbus browser scenario is not established. REQ-TEST-93b22e9a remains active and its manual evidence is the dated delivery record; named physical USB-RS485/gateway reads remain pending without peer details. No release artifact, commit, or push is claimed.
- 2026-10-01 review round 1: Specification compliance requires changes for the missing self-contained release/target and combined browser gates. Implementation quality of the current source candidate is approved after the wheelhouse link guard and focused proof; the nonzero canonical suite remains an independent validation limitation. Resume the bounded review when the unavailable inputs and acceptance scenario can be exercised.

## Rulings


## Handoff


current: build
status: waiting
next: Resolve the self-contained release-input and physical-peer gates, then finish the combined browser acceptance.
must_read: .agentic/steps/4_build.md, .agentic/requirements/platform/REQ-PLAT-ffa360cd.toml, .agentic/requirements/test/REQ-TEST-687a1250.toml, .agentic/requirements/test/REQ-TEST-93b22e9a.toml, 07_Delivery/NILO_MB10_Modbus_Pi_Preview_20261001.md
blockers: complete release-input lock and verified target runtimes unavailable; physical peer details pending; canonical full suite nonzero on unchanged supervisor case

## Stage report

stage: build
result: paused
current: build
changed: indexed Modbus bundle builder, appliance and fixed candidate build integration, refusal tests, documentation and Pi preview
evidence: focused Python 47, frontend 21, contract 11, bundle 5, appliance browser 46, both bench runner handshakes, ARM64 Pi service/catalog; strict advisory exited 0
issues: full suite exit 1 at unchanged supervisor test; no release artifact or self-contained target runtime; combined Modbus browser path and named physical peer checks pending
next: complete release and physical inputs, exercise combined browser path, then final validation
needs_user: yes; supply verified release inputs and a named safe physical peer if available
spec_compliance: changes_required
implementation_quality: approved
review_round: 1
