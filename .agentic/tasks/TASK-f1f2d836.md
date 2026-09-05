<!--
Copyright (c) 2026 DDP Engineering
This software is licensed under the PolyForm Noncommercial License 1.0.0.
Commercial use requires a separate commercial license.
See the LICENSE.txt in the .agentic folder for full license details.
SPDX-License-Identifier: LicenseRef-PolyForm-Noncommercial-1.0.0
-->

<!--
Task journal. One file per task, at .agentic/tasks/TASK-<id>.md, kept
after delivery as the task's historical record.

The header is the block of "key: value" lines directly under the title.
Keep it exactly in that form — one key per line, no blank lines inside
the block, no comments between the lines — so it stays machine-readable.

  stage         frame | specify | design | build | validate | deliver | done
                (adoption journals: survey | components | close | done)
  type          feature | fix | change | adopt
  path          full | quick
  requirements  comma-separated requirement IDs, empty until specify
  parent        optional top-level TASK-id; blank for an ordinary task
  branch        the branch this task is delivered on
  base          the commit this task started from, filled in at creation;
                the delivery report and `check --task` measure the diff
                against it, so leave it alone once it is written. Empty
                when the repository had no history yet.
  opened        YYYY-MM-DD

Header lines are edited in place; log lines are only ever appended.
The Log is a progress-and-evidence ledger, not an activity transcript. Append
only durable events that change the task's stage, scope, engineering outcome,
verification status, authority, or ability to proceed: stage transitions;
material scope or implementation decisions not already captured elsewhere;
consolidated automated or manual verification results; material discoveries;
blockers; approvals; deviations; and delivery outcomes.

Do not log routine searches, reads, edits, internal tool mechanics or corrected
command retries, and do not copy requirements, designs, specifications,
knowledge pages, Rulings, Handoff, or Stage report content into the Log. A
failed command belongs only when it exposes a candidate problem, changes the
implementation or tests, or remains an accepted verification limitation. Keep
each entry concise and self-contained; reference durable artifacts instead of
repeating them. Existing entries remain append-only under later contracts.

Stage report is one replaceable latest snapshot, not an evidence store.
Use plain one-line fields with no fences or bullets inside that section.
stage is the reported stage; current must match the journal header after any
transition. result is in_progress | completed | paused | blocked | skipped.
Leave unknown fields blank; summary prints not recorded, never an assumed pass.
Refresh at a stage transition, pause or session end, blocker, or completed
validation or delivery outcome. Do not refresh after routine work.
-->

# TASK-f1f2d836 — Sigrok integration with managed lifecycle and shared bench operations

stage: done
type: feature
path: full
requirements: REQ-FUNC-26332084, REQ-FUNC-74c69ce6, REQ-FUNC-8a6445ff, REQ-FUNC-1707e62e, REQ-FUNC-b198e8c0, REQ-FUNC-d21b7686, REQ-FUNC-93c2ec4b, REQ-FUNC-28779550, REQ-REL-49026029, REQ-REL-a49eb835, REQ-SEC-6b1dd726, REQ-SEC-b22b7951, REQ-DATA-721ec2ea, REQ-UX-30e2e009, REQ-PLAT-d1427e1e, REQ-REL-94d1b464
parent:
branch: master
base: e78080ec4e59ac42fd3171ea10c3b2b9653df1f9
opened: 2026-09-05

## Done checklist (current stage)

- [x] Validated candidate confirmed unchanged.
- [x] Validation evidence, diff, exclusions and intended local outcome presented.
- [x] Delivery confirmed by the human.
- [x] Journal and report included in the delivery commit.
- [x] Approved local commit delivered on master, without push or PR.
- [x] Close-out carried by the delivery commit.

## Frame

Problem: The NILO integration foundation is not composed into usable real instrument workflows.
Desired outcome: Managed sigrok integration with consistent human and agent interfaces and durable jobs/evidence.
Actors: Administrator, human operator, scoped MCP/CLI client, Bench Core and isolated integration runner.
Primary scenario: Install and enable sigrok -> discover ATK-DL16 -> request finite capture -> inspect job and retrieve saved artifact through NILO.
Examples: Restart only sigrok; uninstall while retaining prior capture; a repeated submission returns the same job.
Counterexamples: PWM access, arbitrary shell/driver settings, a disabled plugin admitting a new capture, treating driver correctness as proven by integration smoke.
In scope: Curated package installation and bundled deployment, persisted enablement, restart/uninstall, discovery, finite capture and saved UART analysis adapter, jobs, approvals, artifacts, GUI/CLI/MCP access, integration-only Pi deployment checks.
Out of scope: PulseView/sigrok fixes or correctness qualification, known-signal hardware certification, PWM, other hardware families, marketplace, continuous acquisition, full PulseView browser replacement, update/rollback UI in this first integration slice.
Constraints: Same master branch; never push; pinned uv/Ruff for Python; no change to unrelated untracked files; preserve installed Pi access/networking; use AEGIS. No branch or commit is implicitly authorized beyond the user's stated scope.
Success signals: Lifecycle and operational parity with authenticated routes; tests cover forbidden PWM, leases, cancellation, persistence, artifacts and enabled state; Pi integration smoke recorded separately from future driver correctness work.
Evidence: STATED user approved implementation, master, no push, Pi deployment, PWM exclusion, focus on integration and uv/Ruff. OBSERVED sources in 00_Planning/PulseView_Sigrok_Integration_Plan.md; Pi CLI detects ATK-DL16 at conn=1.3 and produces a 434-byte srzip for a 1024-sample D0/D1 capture at 1 MHz. INFERRED initial finite capture bridge can use pinned CLI output. UNKNOWN signal wiring, complete driver correctness, performance limits and calibration.
Assumptions: Curated bundled packages are the first installation source; no third-party catalog is needed; initial capture limits are deliberately small; a disabled plugin retains configuration.
Unknowns: Current Pi runtime differs from local release records; record actual runtime and do not replace it for driver correctness work.
Decision-blocking questions: None; existing discussion and explicit proceed instruction authorize this scope.
Proposed behaviors: Shared package manager and job service, Python SDK sigrok runner, one authenticated API service reached by GUI/CLI/MCP, server-owned capture jobs and confined downloadable artifacts.
Path and reason: Full; new externally visible behavior, interface, package dependency, security/data handling and requirements all fail quick-path criteria. One coordinated task; review setting absent/disabled, no delegated work.

## Requirement coverage

| Frame outcome | Atomic behavior | Requirement | Positive case | Boundary or failure | Verification | Design decision or no impact |
|---|---|---|---|---|---|---|
| Integration platform | Install curated integration packages | REQ-FUNC-26332084 | Install a valid curated package and reject a tampered package | See requirement verification | functional suite | Shared core service and runner contract |
| Integration platform | Enable integrations explicitly | REQ-FUNC-74c69ce6 | Enable an installed healthy runner and restart the service | See requirement verification | functional suite | Shared core service and runner contract |
| Integration platform | Disable integrations without losing evidence | REQ-FUNC-8a6445ff | Disable with a running job and attempt new capture | See requirement verification | functional suite | Shared core service and runner contract |
| Integration platform | Restart an integration independently | REQ-FUNC-1707e62e | Restart an enabled idle runner and observe health without core restart | See requirement verification | functional suite | Shared core service and runner contract |
| Integration platform | Uninstall optional integrations | REQ-FUNC-b198e8c0 | Uninstall an idle package and observe absent installed files, retained jobs/artifacts and refusal while busy. | See requirement verification | functional suite | Shared core service and runner contract |
| Integration platform | Discover digital ATK analyzers | REQ-FUNC-d21b7686 | Replay discovery output containing D0-D15 and PWM0/PWM1 | See requirement verification | functional suite | Shared core service and runner contract |
| Integration platform | Capture finite digital signals | REQ-FUNC-93c2ec4b | Submit finite capture through policy and observe one leased execution and a saved session artifact | See requirement verification | functional suite | Shared core service and runner contract |
| Integration platform | Decode saved UART captures | REQ-FUNC-28779550 | Decode a saved simulated UART capture and inspect sample-indexed annotations | See requirement verification | functional suite | Shared core service and runner contract |
| Integration platform | Keep jobs across client disconnects | REQ-REL-49026029 | Submit twice with the same idempotency key and disconnect | See requirement verification | reliability suite | Shared core service and runner contract |
| Integration platform | Cancel and reconcile instrument jobs | REQ-REL-a49eb835 | Cancel an active job and reopen the server with interrupted state | See requirement verification | reliability suite | Shared core service and runner contract |
| Integration platform | Exclude PWM from sigrok access | REQ-SEC-6b1dd726 | Attempt PWM channels, unknown settings and injected driver arguments | See requirement verification | security suite | Shared core service and runner contract |
| Integration platform | Authorize integration operations consistently | REQ-SEC-b22b7951 | Exercise viewer/operator/admin and scoped token requests through HTTP and MCP | See requirement verification | security suite | Shared core service and runner contract |
| Integration platform | Preserve confined capture evidence | REQ-DATA-721ec2ea | Register a captured artifact and retrieve it by ID | See requirement verification | data suite | Shared core service and runner contract |
| Integration platform | Operate integrations from every interface | REQ-UX-30e2e009 | Drive installation, enablement, discovery, capture and job inspection from GUI, CLI and MCP against the same server | See requirement verification | ux suite | Shared core service and runner contract |
| Integration platform | Bound concurrent commands and correlate delayed replies | REQ-REL-94d1b464 | Conflicting resources serialize, independent devices progress and replies retain IDs | Overflow and deadlines settle explicitly | queue/concurrency suite | Shared bounded scheduler and durable replies |
| Integration platform | Deploy integrations with NILO or later | REQ-PLAT-d1427e1e | Deploy a curated package bundle, install from it, disable/uninstall and restart | See requirement verification | platform suite | Shared core service and runner contract |

<!-- A broad Frame outcome may produce several atomic behavior rows. Each
atomic behavior maps to exactly one requirement. Keep this table current
through design.
| Frame outcome | Atomic behavior | Requirement | Positive case | Boundary or failure | Verification | Design decision or no impact |
|---|---|---|---|---|---|---|
-->

## Design

<!-- Filled at design when the task is non-trivial; otherwise record why skipped. -->

All 16 requirements map to the exact contracts in specs/REQ-FUNC-26332084.md.
Core bench package owns curated installation, persisted plugin state/device
inventory and jobs. Extend operation.Service with Prepare/Run while retaining
its synchronous Submit. Reuse existing policy snapshots, leases, SQLite stores,
artifact confinement and supervised NDJSON processes. Each job owns a scoped
supervisor and records result/progress; core resolves staging and input paths.
Python SDK runner owns sigrok CLI invocation/parsing and deterministic simulation.
Discovery uses typed execute result discover_devices, avoiding a wire-version
change solely to enrich the status-only discovery control. This is the selected
alternative to modifying every v1 control codec during the first integration.

Server-owned contexts survive HTTP disconnects. Durable job metadata references
existing sessions; startup marks interrupted work incomplete. Default ask policy
uses existing request-bound approvals, permitted only to authorized browser
humans. Digital finite captures claim the resolved USB resource; offline decode
does not. Disable blocks new work; busy restart/uninstall refuse pending cleanup.
All client parameters are strict; PWM and paths cannot pass through.

Curated package bundles include a digest index, runner/modules, schemas and
Python dependencies prepared with pinned uv; target Python is a declared host
dependency. GUI/CLI/MCP use shared bench methods with distinct permissions.
The GUI provides lifecycle, acquisition forms and result/download views; full
waveform editor and broader decoders remain later planned increments.

Proposed architecture: GUI/CLI/MCP -> bench.Manager -> operation.Service ->
supervisor.ProcessSupervisor -> SDK SigrokIntegration -> installed sigrok-cli;
bench.Manager -> SQLite plugin/jobs/devices and artifact.Store.

Test seams: Python subprocess/SDK fixtures; operation prepare/run tests; bench
service with real SQLite and controlled runner; HTTP/MCP credential tests;
CLI API fixture; React/browser workflow tests; authorized Pi integration smoke.
No source changes in PulseView repo. Actual decoder correctness is deferred.
Legacy integration/gui knowledge pages need routing/boundary reconciliation.

## Log

- 2026-09-05 frame: Full path accepted under explicit user proceed instruction; remain on master; no push.
- 2026-09-05 specify: Created 15 product requirements for lifecycle, shared operations, authorization, jobs, artifacts and bundled deployment; driver correctness qualification excluded by user steering.

- 2026-09-05 design: Selected shared bench service and Python SDK runner; specification records exact lifecycle, jobs, privileges and evidence seams. No unresolved user decisions; proceed to build.

- 2026-09-05 build red: REQ-SEC-b22b7951, REQ-UX-30e2e009; command=`02_Tools/env/with-tools.ps1 go -C 05_Software/nilo_assistant test ./internal/httpapi -run TestPluginManagementRouteRequiresAuthentication -count=1`; exit=1; expected missing behavior=authenticated plugin management route returns 404 instead of explicit service state.

- 2026-09-05 build green: Authenticated plugin route regression passes (go test, exit=0).
- 2026-09-05 build red: REQ-FUNC-26332084, REQ-SEC-6b1dd726; command=`02_Tools/env/with-tools.ps1 uv run --project 05_Software/integration_sdk/python --no-sync pytest -q 05_Software/integrations/sigrok/tests/test_package.py`; exit=1; expected missing behavior=installable sigrok operation manifest absent.
- 2026-09-05 scope clarification: User explicitly requires concurrency, queued commands/replies, transaction IDs and delayed-response timeouts; added product requirement REQ-REL-94d1b464 and exact scheduling semantics.

- 2026-09-05 build green: Python adapter boundary tests: 14 passed; Ruff check/format completed.
- 2026-09-05 build red: REQ-REL-94d1b464, REQ-REL-49026029; command=`02_Tools/env/with-tools.ps1 go -C 05_Software/nilo_assistant test ./internal/operation -run TestPreparePersistsWithoutExecutingAndRunUsesSameTransaction -count=1`; exit=1; expected missing behavior=operation cannot acknowledge durable transaction before executing.

- 2026-09-05 build green: operation Prepare/Run suite and bench queue tests pass: idempotent transactions, same-device serialization, independent concurrency, bounded admission, approval expiry, cancellation, disable/uninstall preserving history. GUI production build passes; broader suites exposed expected inventory/matrix fixture updates and a Windows simulation-path startup defect, corrected.
- 2026-09-05 build red/green: Pi assembled-bundle smoke exposed Go describe validation rejecting Python SDK metadata. Focused TestDescribeAcceptsSDKMetadataButRejectsUnknownFields failed as expected, then the full protocol suite passed after selecting the existing DescribeResult codec. No wire format change.

- 2026-09-05 build green: Full portable Go suite passes, Python adapter 14/14 and GUI 430/430 component tests plus lint/build pass. Pi run4 passed real HTTP/CLI/MCP install/enable/discovery/approval/capture/retry/replies/download/restart/uninstall smoke; assembled-bundle describe passed. Report: /home/dpasare/nilo-integration-check-20260905/run4/report.json. Signal correctness remains explicitly deferred.

- 2026-09-05 validate: Build complete. Pi final bundle run5 passed HTTP/CLI/MCP smoke. Browser on isolated Pi simulation backend completed install/enable/discovery, session approval, capture with preview and saved UART result; reload restored inventory. GUI lint/build and 430 tests, full Go suite and 14 Python adapter tests pass. All 16 product requirements activated and linked; repository tooling remains workflow documentation only.

- 2026-09-05 target/browser verification: Final source bundle and binary passed Pi run6 HTTP/CLI/MCP smoke (report /home/dpasare/nilo-integration-check-20260905/run6/report.json; transaction 01a06ee7-6f46-7b4f-9cd7-b7ec9e890119). Browser reload restored transactions; uninstall retained readable capture/preview. Temporary browser, SSH forwarding and simulation server stopped; original nilo.service remains active.

- 2026-09-05 final validation: Command: 02_Tools/env/with-tools.ps1 node C:/Users/dpasare/.codex/plugins/cache/personal/aegis/0.9.1/scripts/aegis.mjs run --root . --task TASK-f1f2d836 --kind validate. Bridge job 8c71e92c-86f3-4d7e-9e7a-7837da6f0f8c completed exit=0, strict.passed=true, candidate_unchanged=true, digest=627e51978eebd7e4781fb5f91811e11bb99b4efb8f5368e725573e6c14dcddd1. Full tests passed in 338.2s (script tests, all Go/Python suites, 430 GUI unit tests, production build, 38 WSL2 browser tests); lint passed in 11.8s (Go vet, Python compilation, Prettier and ESLint). Complete gate outputs: C:/Users/dpasare/.cache/aegis/integrations/8c71e92c-86f3-4d7e-9e7a-7837da6f0f8c.stdout.log and matching .stderr.log. Twelve unrelated advisory warnings remain: four stale diagnostics requirement links, six prior task-journal findings, unscanned existing Preview.mp4 and out-of-layout existing WORK_Ubuntu.conf; one existing manual-verification note. No current-task catalog errors or warnings.
- 2026-09-05 DEVIATION validate: Retained 545 kB frontend bundle-size warning; production build and browser checks pass, and bundle optimization is outside this integration scope. No verification was skipped or weakened.
- 2026-09-05 deliver presentation: 34 tracked files changed and 62 new task files (96 total): .agentic 25, planning 2, tooling 4, software 60, software docs 3, delivery docs 1, changelog 1. Intended local commit includes integration code/tests, product requirements/specification/journal/knowledge and deployment documentation. Excludes 09_Others/Preview.mp4, WORK_Ubuntu.conf and all ignored runtimes/build/test outputs. No child tasks. No commit, push or PR performed; awaiting the required concrete delivery approval. Pi deployment was isolated testing, not replacement of the existing service.

- 2026-09-05 deployment: User authorized replacement of the normal Pi service. Current backend, GUI and curated bundle deployed; service healthy, Sigrok enabled in ask mode and ATK DL16 D0-D15 discovered. Accounts/settings preserved; rollback backup /var/backups/nilo/20260905T054733Z. Deployment report saved under ignored 02_Tools/runtime/pi-deployment-report-20260905.json. No product candidate changed.
- 2026-09-05 delivery approval: User explicitly requested committing the current files as they are, deferring flow/interface improvements. Candidate digest remains 627e51978eebd7e4781fb5f91811e11bb99b4efb8f5368e725573e6c14dcddd1; approved outcome is one local commit on master, no push/PR, excluding the two unrelated untracked files and ignored outputs.

- 2026-09-05 close-out: Record local-only delivery and task closure in the approved integration commit itself, as required when the commit is the complete outcome. UI/flow improvements are future work; no remote publication.

## Rulings


- RULING-001 | date=2026-09-05 | decision=Implement on existing master and permit Pi deployment checks; never push | reason=Explicit user instruction | source=user | supersedes=none
- RULING-002 | date=2026-09-05 | decision=Focus integration; defer PulseView/sigrok correctness qualification; PWM excluded | reason=Explicit user scope and prior plan | source=user | supersedes=none
- RULING-003 | date=2026-09-05 | decision=uv/Ruff and repository administration are workflow notes only | reason=Requirements describe final product only | source=user | supersedes=none

<!-- Append decisions here; never edit history. Supersede with a new ruling.
- RULING-001 | date=YYYY-MM-DD | decision=<decision> | reason=<reason> | source=user | supersedes=none
source: user | repository | requirement | technical
-->

## Handoff

current: done
status: ready
next: None for this task; interface improvements are a separate future change.
must_read: .agentic/tasks/TASK-f1f2d836.md
blockers: None.

## Stage report

stage: deliver
result: completed
current: done
changed: 96 integration files committed locally on master.
evidence: Unchanged validated digest; full suites and Pi checks passed; delivery report included.
issues: Existing advisories and accepted bundle-size warning; signal correctness deferred.
next: Interface improvements in a separate change; human alone publishes.
needs_user: no - user explicitly approved the local commit.
spec_compliance: disabled
implementation_quality: disabled
review_round:

## Delivery report (2026-09-05)

- stage: deliver; type: feature; path: full; branch: master
- requirements:
  - REQ-FUNC-26332084 - Install curated integration packages
  - REQ-FUNC-74c69ce6 - Enable integrations explicitly
  - REQ-FUNC-8a6445ff - Disable integrations without losing evidence
  - REQ-FUNC-1707e62e - Restart an integration independently
  - REQ-FUNC-b198e8c0 - Uninstall optional integrations
  - REQ-FUNC-d21b7686 - Discover digital ATK analyzers
  - REQ-FUNC-93c2ec4b - Capture finite digital signals
  - REQ-FUNC-28779550 - Decode saved UART captures
  - REQ-REL-49026029 - Keep jobs across client disconnects
  - REQ-REL-a49eb835 - Cancel and reconcile instrument jobs
  - REQ-SEC-6b1dd726 - Exclude PWM from sigrok access
  - REQ-SEC-b22b7951 - Authorize integration operations consistently
  - REQ-DATA-721ec2ea - Preserve confined capture evidence
  - REQ-UX-30e2e009 - Operate integrations from every interface
  - REQ-PLAT-d1427e1e - Deploy integrations with NILO or later
  - REQ-REL-94d1b464 - Bound concurrent commands and correlate delayed replies
- deviations: 1 recorded
  - validate: Retained 545 kB frontend bundle-size warning; production build and browser checks pass, and bundle optimization is outside this integration scope. No verification was skipped or weakened.
- explicit review: disabled; existing validation remains required
- diff vs task base e78080e: 98 file(s) changed, 10684 line(s) added; 1 file(s) not content-scanned
- checks: errors=0 warnings=13 notes=1
- checklist: 5/6 done; open: Close-out carried by the delivery commit.
