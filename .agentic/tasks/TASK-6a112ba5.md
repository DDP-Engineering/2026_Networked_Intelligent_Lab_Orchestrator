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
Keep each log line to one sentence.

Stage report is one replaceable latest snapshot, not an evidence store.
Use plain one-line fields with no fences or bullets inside that section.
stage is the reported stage; current must match the journal header after any
transition. result is in_progress | completed | paused | blocked | skipped.
Keep all observations and decisions in the append-only Log as well.
Leave unknown fields blank; summary prints not recorded, never an assumed pass.
Refresh after work or decisions even if current has not changed.
-->

# TASK-6a112ba5 — Add deterministic simulation inventory API

stage: done
type: feature
path: full
requirements: REQ-FUNC-22f4f69c, REQ-FUNC-346ffa07, REQ-FUNC-9835998b, REQ-FUNC-7f4afa6b, REQ-PLAT-30618e85, REQ-SEC-7fd3869a
parent:
branch: master
base: 61bc632749b4d0d2e45fe60a8cf3920223fad4a7
opened: 2026-08-31

## Done checklist (current stage)

- [x] validated candidate confirmed unchanged; otherwise returned to validate
- [x] validation evidence, diff, exclusions, and outcome presented
- [x] delivery confirmed by the human
- [x] journal and report included in the delivery commit
- [x] agreed outcome delivered, normally a PR
- [x] close-out committed after the outcome succeeded, or carried by the delivery commit where nothing could fail after it

## Log

- 2026-08-31 frame: Scope is deterministic healthy and unavailable simulation catalogs plus authenticated integration and device inventory endpoints; runner execution, real discovery, operations, GUI, MCP, and persistence are excluded.
- 2026-08-31 frame: The full path is required because this task adds externally visible HTTP schemas and scenario-selection CLI behavior.
- 2026-08-31 frame: Expected requirements cover integration inventory, device inventory, deterministic scenario selection, and exclusion of real hardware entries from simulation.
- 2026-08-31 frame: The user approved continuation immediately after the committed vertical-slice roadmap, which names this inventory slice next.
- 2026-08-31 frame: The catalog and API are tightly coupled and remain one parent task without children.
- 2026-08-31 frame deviation: The first repository inspection looked for `integration.toml`; the fake package uses `manifest.json`, which was then read with its Python discovery implementation.
- 2026-08-31 specify: Added focused requirements and exact schemas for integration inventory, device inventory, deterministic scenario selection, unsupported-scenario rejection, and simulation-only marking; reused the protected HTTP authentication requirement.
- 2026-08-31 design: Revised the Bench Core knowledge architecture to the intended immutable simulation catalog feeding the protected HTTP inventory routes.
- 2026-08-31 build red: command=`F:\2026_Networked_Intelligent_Lab_Orchestrator\02_Tools\env\with-tools.ps1 go test ./internal/simulation -run '^TestParseScenarioDefaultsAndSelectsSupportedStates$'`; exit=1; expected missing behavior=scenario type, supported constants, and `ParseScenario` are undefined.
- 2026-08-31 build red: command=`F:\2026_Networked_Intelligent_Lab_Orchestrator\02_Tools\env\with-tools.ps1 go test ./internal/simulation -run '^TestParseScenarioRejectsUnsupportedIdentifier$'`; exit=1; expected missing behavior=`ParseScenario` accepts an unsupported scenario identifier.
- 2026-08-31 build red: command=`F:\2026_Networked_Intelligent_Lab_Orchestrator\02_Tools\env\with-tools.ps1 go test ./internal/simulation -run '^TestCatalogReturnsStableScenarioIntegrationInventory$'`; exit=1; expected missing behavior=`NewCatalog` and the stable simulated integration model are undefined, including the required simulation-only marking.
- 2026-08-31 build red: command=`F:\2026_Networked_Intelligent_Lab_Orchestrator\02_Tools\env\with-tools.ps1 go test ./internal/simulation -run '^TestCatalogReturnsStableScenarioDeviceInventory$'`; exit=1; expected missing behavior=the stable simulated device model and immutable device inventory are undefined, including the required simulation-only marking.
- 2026-08-31 build red: command=`F:\2026_Networked_Intelligent_Lab_Orchestrator\02_Tools\env\with-tools.ps1 go test ./internal/httpapi -run '^TestIntegrationInventoryReturnsExactDeterministicSimulationEnvelope$'`; exit=1; expected missing behavior=the HTTP handler has no simulation catalog dependency or integration inventory route.
- 2026-08-31 build red: command=`F:\2026_Networked_Intelligent_Lab_Orchestrator\02_Tools\env\with-tools.ps1 go test ./internal/httpapi -run '^TestDeviceInventoryReturnsExactDeterministicSimulationEnvelope$'`; exit=1; expected missing behavior=the authenticated device inventory route returns 404.
- 2026-08-31 build red: command=`F:\2026_Networked_Intelligent_Lab_Orchestrator\02_Tools\env\with-tools.ps1 go test ./cmd/nilo -run '^TestNiloServeCommandSelectsUnavailableScenario$'`; exit=1; expected missing behavior=the serve command rejects the unimplemented `--scenario` flag before listening.
- 2026-08-31 build deviation: The first impacted-gate wrapper yielded after 30 seconds without preserving its session ID, so its partial failure-free output was not treated as final evidence and no duplicate was started until process inspection showed the visible Python processes were unrelated MCP helpers.
- 2026-08-31 build: The repeated impacted gate was polled through its preserved session and passed all discovered Go and Python suites in 34.7 seconds.
- 2026-08-31 build: Evolution matched the expected catalog, HTTP, CLI, test, planning, and documentation files with no removals; AEGIS check reported zero errors, warnings, and notes.
- 2026-08-31 validate deviation: The first requirement reread used `.md` extensions rather than the repository's registered `.toml` requirement paths, so that read command exited non-zero; the exact TOML paths were then resolved and read without changing the candidate.
- 2026-08-31 validate: Semantic diff review found the implementation within scope with no specification, error-path, portability, security, or unrequested-work findings; explicit review is disabled, so both optional review verdicts are disabled.
- 2026-08-31 validate: Re-read all six active requirements and three linked specifications; their prose, affected paths, linked tests, and automated verification match the candidate, and no Manual or Inspection verification is declared.
- 2026-08-31 validate: Reconciled Bench Core knowledge with the built catalog and inventory routes, reran the conservative impacted gate after that edit, and passed 9 tooling tests, all Go packages, 32 SDK tests, and 7 fake-integration tests with 1 declared skip.
- 2026-08-31 validate: Live Windows-to-WSL smoke verification rebuilt the Linux binary, ran scenario `unavailable`, and authenticated status, integration, and device requests from Windows; responses had stable simulation-only identities and unavailable readiness as designed.
- 2026-08-31 validate: Final evolution reported 9 expected changed candidate files with 1 added, 8 modified, and 0 removed.
- 2026-08-31 validate: Final command=`F:\2026_Networked_Intelligent_Lab_Orchestrator\02_Tools\env\with-tools.ps1 python .agentic\aegis.py check --task TASK-6a112ba5 --strict --run`; exit=0; configured test and lint both passed; summary errors=0 warnings=0 notes=0; strict=ok.
- 2026-08-31 deliver: The user approved the presented validated candidate for one repository-local commit on master with no push or pull request.
- 2026-08-31 deliver deviation: The report-time advisory snapshot recorded one expected `unticked-deliver` warning because close-out was deliberately still open during report generation; the close-out is resolved in the same commit-only delivery.
- 2026-08-31 deliver: The repository-local commit is the entire authorized outcome, so the task is closed before committing and this same commit carries the final delivery record.

## Rulings

<!-- Append decisions here; never edit history. Supersede with a new ruling.
- RULING-001 | date=YYYY-MM-DD | decision=<decision> | reason=<reason> | source=user | supersedes=none
source: user | repository | requirement | technical
-->

- RULING-001 | date=2026-08-31 | decision=Deliver incrementally on master instead of creating a task branch | reason=The user explicitly requested incremental work on master | source=user | supersedes=none
- RULING-002 | date=2026-08-31 | decision=Treat commit and continue as confirmation of the next roadmap slice | reason=The user approved continuation after the roadmap explicitly identified deterministic simulation inventory as Slice 2 | source=user | supersedes=none
- RULING-003 | date=2026-08-31 | decision=Create the presented repository-local delivery commit without pushing or opening a pull request | reason=The user explicitly answered commit at the delivery checkpoint | source=user | supersedes=none

## Handoff

<!-- Replace these fields with the latest minimal resume snapshot.
status: ready | waiting | blocked
-->

current: done
status: ready
next: Create the approved repository-local commit on master; no push or pull request follows.
must_read: this journal; final staged diff and repository status
blockers: none

## Stage report

stage: deliver
result: completed
current: done
changed: Implemented immutable scenario catalogs, authenticated integration/device routes, CLI scenario selection, exact tests, and executable documentation.
evidence: Focused red-green records, two complete impacted passes, live Windows-to-WSL API verification, strict test/lint proof, human approval, and the delivery report are recorded.
issues: The report-time close-out warning is expected and resolved in this commit-only outcome; no validation warnings or pre-existing failures remain.
next: The approved local commit is the complete outcome; no push or pull request is authorized.
needs_user: no
spec_compliance: disabled
implementation_quality: disabled
review_round:

## Design

- Components: new `internal/simulation` owns immutable scenario catalogs; `internal/httpapi` serializes them; `cmd/nilo` selects the scenario before application or listener startup. [REQ-FUNC-22f4f69c, REQ-FUNC-346ffa07, REQ-FUNC-9835998b]
- Interface: authenticated `GET /api/v1/integrations` and `GET /api/v1/devices` return `nilo.http/1` envelopes with a stable scenario and ordered items. [REQ-FUNC-22f4f69c, REQ-FUNC-346ffa07]
- Scenario: `--scenario` defaults to `healthy`, accepts only `healthy` or `unavailable`, and rejects any other value before composition. [REQ-FUNC-9835998b, REQ-FUNC-7f4afa6b]
- Isolation: catalog construction takes only a typed scenario and performs no filesystem, process, network, USB, or serial access; every entry has simulation prefixes and flags. [REQ-PLAT-30618e85]
- Data: integration and device values are copied out of the catalog so callers cannot mutate later responses; operations and capabilities remain lexically ordered. [REQ-FUNC-22f4f69c, REQ-FUNC-346ffa07]
- Rejected alternative: launching the fake Python runner for every inventory request was rejected because package installation and long-lived supervisor composition are not yet part of the server and would mix transport inventory with process lifecycle work. [REQ-PLAT-30618e85]
- Test seams: catalog table tests cover scenario states, determinism, copies, and marking; handler tests cover exact authenticated envelopes; command tests cover default/selected/unsupported scenario startup. [all task requirements]
- Expected files: `internal/simulation/{catalog,catalog_test}.go`, `internal/httpapi/{server,server_test}.go`, `cmd/nilo/{main,main_test}.go`, README/development documentation, Bench Core knowledge, and task artifacts.

## Delivery report (2026-08-31)

- stage: deliver; type: feature; path: full; branch: master
- requirements:
  - REQ-FUNC-22f4f69c - List simulated integrations
  - REQ-FUNC-346ffa07 - List simulated devices
  - REQ-FUNC-9835998b - Select a deterministic simulation scenario
  - REQ-FUNC-7f4afa6b - Reject unsupported simulation scenarios
  - REQ-PLAT-30618e85 - Exclude real hardware from simulation inventory
  - REQ-SEC-7fd3869a - Authenticate protected HTTP API requests
- deviations: none recorded
- explicit review: disabled; existing validation remains required
- diff vs task base 61bc632: 21 file(s) changed, 2098 line(s) added
- checks: errors=0 warnings=1 notes=0
- checklist: 5/6 done; open: close-out committed after the outcome succeeded, or carried by the delivery commit where nothing could fail after it
