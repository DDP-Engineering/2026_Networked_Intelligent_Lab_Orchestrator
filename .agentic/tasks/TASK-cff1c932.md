<!--
Copyright (c) 2026 DDP Engineering
This software is licensed under the PolyForm Noncommercial License 1.0.0.
Commercial use requires a separate commercial license.
See the LICENSE.txt in the .agentic folder for full license details.
SPDX-License-Identifier: LicenseRef-PolyForm-Noncommercial-1.0.0
-->


# TASK-cff1c932 - CAM-02 managed device records

stage: build
type: feature
path: full
interaction: autopilot
requirements: REQ-DATA-49c54101, REQ-REL-1d5863b6, REQ-DATA-3e86a281, REQ-SEC-09df2645
parent:
branch: NILO_camera_plugin
base: b83555684d11c359c41676a962dc0f0860e1335d
opened: 2026-09-24

## Done checklist (current stage)

- [ ] Add failing repository and reconciliation tests before behavior changes.
- [ ] Build generic managed records, migrations, identity rules and revisions.
- [ ] Route discovery, readiness, administration and submission through records.
- [ ] Keep Sigrok behavior and full suite green.

## Frame

Problem: Bench Core stores a replaceable Sigrok-specific device JSON list and has no durable named camera records.
Desired outcome: Generic stable managed device records survive discovery, configuration and hardware movement.
Actors: Core, administrator, operator and agent.
Primary scenario: Runner discovers hardware; core reconciles its physical identity; administrator accepts and names the record; permitted work resolves its validated internal endpoint.
Examples: The Pi FaceCam has a capture and metadata node and no serial; two identical serial-less cameras in separate ports stay distinct.
Counterexamples: Metadata-only nodes are not capturable; two devices with a duplicate serial never collapse into one record; stale revision updates fail.
In scope: CAM-02 migration, repository, reconciliation, generic device administration, Sigrok migration and submission checks.
Out of scope: Camera runner, UI and live preview, owned by other packages.
Constraints: NFKC Casefold names, monotonic per-plugin IDs, durable weak-serial classification, existing job/artifact history, permission and audit rules.
Success signals: Stable identity across restarts and changed ports as specified; presence and readiness transitions; revision conflicts; Sigrok compatibility.
Evidence: STATED camera plan and user implementation direction; OBSERVED Pi webcam identity in `.agentic/evidence/camera/pi-webcam/`; OBSERVED current `bench_plugins.devices_json` and `Discover` overwrite in core.
Assumptions: Existing Sigrok analyzer ids can be migrated at first discovery without changing their capture request grammar.
Unknowns: Other camera and HDMI hardware identity quality; simulated fixtures cover the algorithm.
Decision-blocking questions: None for host implementation.
Proposed behaviors: New tables and repository assign/reuse records, reconcile presence, guard operations and expose revisioned admin actions.
Path and reason: Full path for on-disk layout and public resources; current branch retained under repository Git authority.

## Requirement coverage

| Frame outcome | Atomic behavior | Requirement | Positive case | Boundary | Verification | Decision |
|---|---|---|---|---|---|---|
| Stable identity | Match serial or port and preserve weak classification | REQ-DATA-49c54101 | replug | duplicate serial | repository tests | D1 |
| Truthful presence | Reconcile atomically | REQ-REL-1d5863b6 | present/absent | failed scan unknown | discovery tests | D2 |
| Administration | Validate revisioned settings and names | REQ-DATA-3e86a281 | rename/configure | stale conflict | repository/route tests | D3 |
| Safe endpoint | Resolve internal record for operations | REQ-SEC-09df2645 | valid device id | hostile path | submission tests | D4 |


## Design

D1: SQLite holds `bench_devices`, per-plugin monotonic sequences and persistent weak serial keys. Record ids (`cam-N`, `la-N`) are distinct from physical identity and endpoint. A duplicate/junk serial is permanently demoted to port matching.

D2: Discovery validates the runner result before a single transaction marks observed records present and unseen records absent. Startup begins unknown; failed discovery does not assert absence. New camera records remain unconfigured.

D3: Name keys are computed in Go with Unicode NFKC Casefold, stored under a uniqueness constraint; every update compares a revision and writes audit metadata.

D4: Public requests contain only record ids and typed parameters. Core injects endpoint identity from the validated record into the runner request after the lease is acquired.

Test seams: SQLite migration/repository tests, adapter discovery tests, HTTP permission tests and existing Sigrok job tests.


## Log

- 2026-09-24 frame: Full CAM-02 path accepted in autopilot under the user's instruction to implement before missing HDMI/Ubuntu hardware checks.
- 2026-09-24 specify/design: Reused four existing camera requirements and specs; D1-D4 assign persistence, reconciliation, administration and endpoint resolution.
- 2026-09-24 build red: REQ-DATA-49c54101; command=`.\02_Tools\env\with-go.ps1 go -C 05_Software/nilo_assistant test ./internal/store/sqlite -run TestOpenConfiguresDurableDatabaseAndMigratesAllFoundationTables -count=1`; exit=1; expected missing behavior=the database lacks managed-device records, monotonic sequences and persistent weak-serial classification tables.
- 2026-09-24 build green: REQ-DATA-49c54101; focused migration test exits 0. Migration 019 adds managed records, monotonic sequences and persistent weak-serial classification without changing existing bench tables.
- 2026-09-24 build red: REQ-DATA-49c54101; command=`.\02_Tools\env\with-go.ps1 go -C 05_Software/nilo_assistant test ./internal/store/sqlite -run 'TestManagedDevice(ReconcileKeepsSerialAndPortIdentities|DuplicateSerialStaysWeak)' -count=1`; exit=1; expected missing behavior=there is no managed-device repository to reconcile stable serials, port-bound cameras or persistently weak duplicate serials.
- 2026-09-24 build green: REQ-DATA-49c54101; both focused repository tests exit 0. Strong serials follow port moves, serial-less devices retain absent records and take fresh ids at new ports, and duplicate serials remain port-bound after unplug.
- 2026-09-24 build red: REQ-REL-1d5863b6; command=`.\02_Tools\env\with-go.ps1 go -C 05_Software/nilo_assistant test ./internal/store/sqlite -run TestManagedDeviceFailedDiscoveryLeavesUnknown -count=1`; exit=1; expected missing behavior=the repository cannot mark records unknown before startup discovery or read back that state after a failed scan.
- 2026-09-24 build green: REQ-REL-1d5863b6; focused managed-device repository suite exits 0. Startup can mark prior records unknown and discovery failure leaves them unknown rather than claiming absence; new records remain unconfigured.
- 2026-09-24 build red: REQ-DATA-3e86a281; command=`.\02_Tools\env\with-go.ps1 go -C 05_Software/nilo_assistant test ./internal/store/sqlite -run TestManagedDeviceNamesAndStaleRevisionsConflict -count=1`; exit=1; expected missing behavior=the repository has no revision-checked camera configuration and canonical Unicode name uniqueness.
- 2026-09-24 build green: REQ-DATA-3e86a281; focused managed-device repository suite exits 0. Configured names use Unicode NFKC then case fold uniqueness, accepted settings retain an initializing state, and stale revisions conflict without changing the record.
- 2026-09-24 build: Added revision-checked enable/disable, retry, rebind and forget operations; rebind tombstones the provisional record, rejects its history, preserves the configured record, and resets its baseline. HTTP, CLI, MCP and browser administration surfaces now carry exact-ID confirmation for rebind/forget. Focused repository, manager, CLI, MCP and browser tests pass.

## Rulings


## Handoff


current: build
status: ready
next: Review device action concurrency and authorization, complete physical rebind acceptance when a second camera or port move can be arranged, then validate the integrated candidate.
must_read: .agentic/tasks/TASK-cff1c932.md; .agentic/steps/4_build.md; .agentic/specs/REQ-DATA-49c54101.md; .agentic/specs/REQ-REL-1d5863b6.md
blockers: None for host work; other target hardware acceptance is deferred.

## Stage report

stage: build
result: in_progress
current: build
changed: Added durable managed records, reconciliation, revisioned configuration and administration, baseline readiness transitions, and public management surfaces.
evidence: Managed-device repository and manager tests pass; full Go suite and 519 GUI tests passed; the Pi camera returns present and ready after restart and completed physical snapshots.
issues: Two-camera rebind acceptance and Sigrok managed-device migration remain open.
next: Validate device action concurrency and missing hardware cases; complete Sigrok migration.
needs_user: no
spec_compliance:
implementation_quality:
review_round:
