<!--
Copyright (c) 2026 DDP Engineering
This software is licensed under the PolyForm Noncommercial License 1.0.0.
Commercial use requires a separate commercial license.
See the LICENSE.txt in the .agentic folder for full license details.
SPDX-License-Identifier: LicenseRef-PolyForm-Noncommercial-1.0.0
-->


# TASK-e4b3da95 - MB-07 Modbus live register workspace, traffic, writes and trends

stage: done
type: feature
path: full
interaction: autopilot
requirements: REQ-UX-04cd31fe, REQ-UX-9d7fd3a6, REQ-UX-35328cf5, REQ-UX-e0682e6a, REQ-FUNC-c7473d9f
parent:
branch: ddp/modbus-client
base: df1e53da4b4217f7e9ec3bf09c1c6d261e9ec93f
opened: 2026-09-30

## Done checklist (current stage)

- [x] validated candidate confirmed unchanged; otherwise returned to validate
- [x] validation evidence, diff, exclusions, and outcome presented
- [x] delivery confirmed by the human
- [x] journal and report included in the delivery commit
- [x] agreed outcome delivered, normally a PR
- [x] close-out committed after the outcome succeeded, or carried by the delivery commit where nothing could fail after it

## Frame

Problem: MB-04/05/06 provide Core sessions, maps, workspaces and endpoint navigation, but the RealTime View is a placeholder. Gitea #34 is the next work package.
Desired outcome: An operator can load a saved TCP endpoint workspace, explicitly connect, inspect current register values and traffic, control acquisition, preview and submit permitted writes, and view bounded trends.
Actors: Operator, Viewer, administrator, browser, Bench Core, Modbus runner.
Primary scenario: Open a named endpoint -> select a saved workspace/profile and inspect it without I/O -> explicitly Connect through policy -> poll multiple units -> see fresh/stale mapped samples and paired request/results -> explicitly preview and submit a write -> Disconnect.
Examples: Two poll groups on one TCP endpoint report different unit values; pausing display retains Core polling; a timeout preserves the previous value and marks it stale.
Counterexamples: Loading a workspace connects automatically; a failed poll renders zero as fresh; changing a live poll plan bypasses admission; a lost write response is shown as a safe retry.
In scope: MB-07 live endpoint UI, typed grid, polling controls, traffic, explicit write preview/submit, bounded trends, Core support missing for independent group pause and telemetry, client contracts, EN/DE/RO accessibility and responsive tests.
Out of scope: MB-08 durable recording persistence and offline viewer, MB-09 scans/diagnostics, serial RTU UI, physical hardware acceptance, Git staging/commit/push, seven unrelated planning/media paths.
Constraints: STATED TCP-only; plugin policy enables successive writes without per-operation approval; shared definitions and one live owner; same branch; AEGIS autopilot. OBSERVED Core has bounded events and write preview but lacks frontend live actions and independent poll-group pause. Saving/loading definitions must remain inert.
Success signals: Authenticated Core-driven UI handles independent groups, quality/age/gaps, explicit write outcomes, bounded traffic/trends, keyboard/locale/viewport cases; focused and full suites pass.
Evidence: STATED user thread; OBSERVED Gitea #34, 00_Planning/Modbus_Plugin_Plan.md sections 5-7, .agentic/knowledge/modbus-integration.md, Core modbus_session/poll/write and frontend ModbusWorkspacePage/api; INFERRED MB-07 needs browser work plus small Core telemetry/control additions. UNKNOWN hardware-specific response. Recording boundary resolved by RULING-001.
Assumptions: The existing read-only Sessions page remains MB-08's responsibility. Browser labels traffic as request/result metadata unless exact raw frames are available from Core.
Unknowns: Hardware-specific responses and physical-device behavior remain for later target acceptance.
Decision-blocking questions: None; user deferred recording to MB-08.
Proposed behaviors: Explicit session connect/disconnect; workspace and group controls; mapped grid with freshness; write preview and conservative outcome; bounded paired traffic and trends.
Path and reason: Full path because new observable browser behavior and Core interfaces are required. Reuse ddp/modbus-client under the user's same-branch instruction. No Git mutation authorized.

| Quick-path criterion | Yes | No | Evidence |
|---|---|---|---|
| No new externally observable behavior | | x | New live UI and controls. |
| No new interface/data format | | x | Browser client and group telemetry. |
| No new dependency | x | | Existing stack. |
| Existing requirement covers behavior | | x | Core contracts do not cover browser view. |
| No security/data-handling change | | x | Explicit live writes and polling. |

## Requirement coverage

| Frame outcome | Actor/system | Trigger or state | Response | Positive case | Boundary or failure | Observable result |
|---|---|---|---|---|---|---|
| Live workspace | Operator/browser | Select saved workspace then Connect | Inert load, pinned admission and live controls | Two units | Offline, Ask, denied, stale | Actual session state |
| Grid | Browser | Core read event | Decode mapped values with freshness | Good multiword sample | Timeout, disconnect, gap | Raw/typed cell and quality |
| Write | Owner/browser/Core | Edit point then Submit | Exact preview then one write | Acknowledged | Invalid, denied, delivery_unknown | Payload and distinct result |
| Traffic/trends | Browser | Poll events | Bounded paired list and series | Continuous samples | Gap, pause, clear | Honest bounded display |
| Group control | Owner/Core | Pause/resume one group | Preserve admission, update telemetry | Other group advances | Unknown group, slow reply | Group counters and status |

| Frame outcome | Atomic behavior | Requirement | Positive case | Boundary or failure | Verification | Design decision |
|---|---|---|---|---|---|---|
| Live workspace | Explicit inert load and connect/control | REQ-UX-04cd31fe | Connected workspace | Offline/denied/stale | UI/API tests | Core policy job |
| Grid | Raw/typed value with freshness | REQ-UX-9d7fd3a6 | Mapped sample | Error/gap | Event component tests | Pinned map revisions |
| Write | Preview then submit one command | REQ-UX-35328cf5 | Acknowledged | Unknown/invalid | UI/Core tests | Core codec and write preview |
| Traffic/trends | Bounded live display | REQ-UX-e0682e6a | Paired events/series | Pause/clear/gap | UI tests | Cursor and capped buffers |
| Group control | Independent pause and telemetry | REQ-FUNC-c7473d9f | Other group advances | Unknown/slow | Core/API/CLI/MCP tests | Per-group state in session |


## Design

Requirements coverage: `REQ-UX-04cd31fe` uses inert workspace detail and pinned profile/map loads, then existing finite Connect and live status. `REQ-UX-9d7fd3a6` renders exact raw words and Core codec interpretations with event-derived quality. `REQ-UX-35328cf5` uses Core's point encoder and write PDU preview before one explicit write. `REQ-UX-e0682e6a` caps browser event and chart buffers and marks gaps. `REQ-FUNC-c7473d9f` adds per-group pause and observed metrics to Core session state.

Component responsibilities: Core owns authentication, policy, pinned plans, Modbus codecs and device I/O; runner owns one transport; browser owns only presentation buffers, filtering and visible pause. Saving/opening a shared workspace never connects.

Interfaces: Existing `GET maps/workspaces/{id}/revisions/{revision}` supplies exact definitions. Add Core point codec endpoints for decode and encode of a pinned map point using `modbus.DecodePoint` / `EncodePoint`; no JS reimplementation of 64-bit arithmetic. Existing connect/read/poll/events/write/preview/disconnect endpoints get typed frontend client methods. Add per-group pause control with group ID and status metrics to HTTP/CLI/MCP; invalid group or changed plan returns an error. [REQ-UX-04cd31fe, REQ-UX-9d7fd3a6, REQ-UX-35328cf5, REQ-FUNC-c7473d9f]

Data and flow: A browser-selected workspace provides up to 32 enabled poll groups with their unit/function/address/count. Connect pins the compiled plan. The browser awaits the finite job and polls session status; Ask stays in the human approval surface. Polling starts only after admission. Each group is independently pausable; global stop retains the pinned plan and ends the session only on Disconnect. Samples retain timestamp, source group and revision context. [REQ-UX-04cd31fe, REQ-FUNC-c7473d9f]

Failure and recovery: A late or missing event marks a gap; timeout/error never replaces the last good value. Display pause and Clear display touch local presentation only, never Core polling or later recording. A write keeps one idempotency key from preview through submit; delivery_unknown is terminal. Read-back is a separate explicit read. [REQ-UX-9d7fd3a6, REQ-UX-e0682e6a, REQ-UX-35328cf5]

Security/concurrency: Owner-only Core control remains authoritative; viewer UI cannot offer live mutation, and Core rechecks current authority. The scheduler remains single transport owner with no overlapping request. Browser polling is bounded and cleans up timers on navigation. [REQ-UX-04cd31fe, REQ-FUNC-c7473d9f]

Compatibility: Additive endpoints/fields; existing Core RTU support remains but the new browser workflow is TCP/RTU-over-TCP only per user. No schema migration or new dependency. [user TCP constraint]

Tests: Core fake-runner tests for independent groups, telemetry and codec boundary; frontend API parser and component tests for inert load, status, quality, write and bounds; EN/DE/RO keyboard and responsive checks. Full repository gate and strict AEGIS check at validate. [all requirements]

Risks and open decisions: Raw TX/RX frame capture is not in Core's event contract; browser pairs canonical request/result metadata. Recording controls and persistence are deferred to MB-08 by user decision. Physical device acceptance is later.

Expected files: Core bench poll/session/codec, HTTP/CLI/MCP Modbus and tests; frontend ModbusWorkspacePage, api/modbus/client, i18n, CSS and tests; knowledge page, changelog and AEGIS records.

### Proposed architecture - TASK-e4b3da95

`ModbusWorkspacePage -> BenchCoreClient -> HTTP Modbus -> bench.Manager -> single runner`
`bench.Manager -> revisioned workspace/map and point codec`
`Core events/status -> bounded browser grid, traffic and trends`


## Log

- 2026-09-30 frame: Full MB-07 path on existing branch; user autopilot and TCP-only choices persist. Recording boundary asked asynchronously; independent live behavior proceeds. No Git mutation authorized.
- 2026-09-30 specify: Created five draft requirements for inert live admission, quality grid, exact write flow, bounded traffic/trends and independent group telemetry; no existing requirement was changed.
- 2026-09-30 design: Design fixes Core authority and codec, browser bounded presentation and additive group state. Recording split remains open; live implementation can proceed.
- 2026-09-30 build red: command=`with-tools.ps1 go test ./internal/bench -run '^TestModbusPollGroupsPauseIndependently$' -count=1` (from `05_Software/core`); exit=1; expected missing behavior=`Manager.SetModbusPollGroupPaused` and per-group status do not exist (REQ-FUNC-c7473d9f).
- 2026-09-30 build red: command=`with-tools.ps1 go test ./internal/bench -run '^TestModbusPointCodecUsesPinnedMapRevision$' -count=1` (from `05_Software/core`); exit=1; expected missing behavior=Core has no pinned-map decode/encode service for the live grid and exact write preview (REQ-UX-9d7fd3a6, REQ-UX-35328cf5).
- 2026-09-30 build red: command=`with-tools.ps1 npm.cmd --prefix .\05_Software\frontend run test:ci -- src/components/pages/ModbusWorkspacePage.test.tsx`; exit=1; expected missing behavior=RealTime View has no inert workspace selector or explicit Connect control (REQ-UX-04cd31fe).
- 2026-09-30 build red: command=`with-tools.ps1 npm.cmd --prefix .\05_Software\frontend run test:ci -- src/components/pages/ModbusWorkspacePage.test.tsx`; exit=1; expected missing behavior=live view has no event refresh, mapped quality grid or paired traffic (REQ-UX-9d7fd3a6, REQ-UX-e0682e6a).
- 2026-09-30 build red: command=`with-tools.ps1 npm.cmd --prefix .\05_Software\frontend run test:ci -- src/components/pages/ModbusWorkspacePage.test.tsx -t 'previews exact typed write bytes'`; exit=1; expected missing behavior=register row has no Write action or exact preview/Submit flow (REQ-UX-35328cf5).
- 2026-09-30 build red: command=`with-tools.ps1 npm.cmd --prefix .\05_Software\frontend run test:ci -- src/components/pages/ModbusWorkspacePage.test.tsx`; exit=1; expected missing behavior=no Duplicate/Save workspace group controls (REQ-UX-04cd31fe).
- 2026-09-30 build red: command=`with-tools.ps1 go test ./internal/bench -run '^TestModbusConnectPinsSavedWorkspacePlan$' -count=1` (from `05_Software/core`); exit=1; expected missing behavior=admitted session has no saved workspace revision identity or plan match check (REQ-UX-04cd31fe).
- 2026-09-30 build red: command=`with-tools.ps1 go test ./internal/bench -run '^TestModbusPollGroupStopsOnErrorWithoutStoppingPeer$' -count=1` (from `05_Software/core`); exit=1; expected missing behavior=per-group timeout and stop-on-error fields are absent (REQ-FUNC-c7473d9f).
- 2026-09-30 decision: User deferred functional recording and Start/Stop logging to MB-08; MB-07 remains an explicit live display and acquisition work package.
- 2026-09-30 build green: WSL `go test ./internal/bench ./internal/httpapi ./internal/mcp ./cmd/nilo -run Modbus -count=1` passed Core pinned admission, independent pause/timeout/stop-on-error, codec, HTTP and CLI tests; `go test ./internal/mcp -run TestBenchToolsUseCurrentTokenScopes -count=1` passed MCP tool discovery and authorization. Covers REQ-FUNC-c7473d9f and Core portions of REQ-UX-04cd31fe/9d7fd3a6/35328cf5.
- 2026-09-30 build green: Frontend focused `test:ci -- src/components/pages/ModbusWorkspacePage.test.tsx` passed six live-view tests covering inert load, quality/error/gap/disconnect, exact write submit and unconfirmed response, bounded buffers and display pause. Frontend suite then passed 612/612 tests; production build passed. Covers all four UX requirements.
- 2026-09-30 build inspection: Chrome Playwright `modbus-layout.check.ts` passed 12/12 EN/DE/RO keyboard, light/dark and 320/800x480/1024x600/desktop reflow cases; simulated Core HTTP/CLI/MCP tests exercised actual Manager routes. Browser geometry used the existing controlled fixture.
- 2026-09-30 build integration: Python Modbus suite passed 181/184; three supervised child-process tests timed out waiting for the first SDK reply on the WSL-mounted checkout. An isolated rerun passed two of three and reproduced one timeout. No Python runtime source changed in this ticket.
- 2026-09-30 build evolution: `aegis evolution --task TASK-e4b3da95` recorded 26 task paths, with seven unrelated planning/media paths excluded. Five requirements activated with current paths and linked tests.
- 2026-09-30 validate review round 1: Specification compliance required a TCP/RTU-over-TCP live-view guard; implementation quality required paused displays to avoid manual-read and write-traffic updates. Both findings were fixed in the existing browser seam with a focused RTU regression and pause assertion.
- 2026-09-30 validate review round 2: Specification compliance approved against all five active requirements and RULING-001. Implementation quality approved after source/error/authority/portability review and seven passing focused browser tests; Core still owns session admission, codec and write authority.
- 2026-09-30 validate review follow-up: A trend-panel inspection found that the graph omitted a local gap/quality label while its numeric history correctly skipped failed samples. Added the label and regression assertion; round-2 verdicts were invalidated by this candidate change.
- 2026-09-30 validate review round 3: Specification compliance and implementation quality approved on the revised candidate after seven focused live-view tests; the trend now names quality and event gaps in its own panel, and failed samples do not enter numeric history.
- 2026-09-30 validate review round 4: Strict Windows gate failed lint on frontend formatting and hook dependency/state patterns. Prettier, a props-derived status fallback and stable refresh dependencies corrected the candidate; targeted ESLint, focused tests and build passed. The same gate's full test hit Linux-only `os.geteuid` in three Modbus workspace tests when routed through Windows Python; this host mismatch remains for WSL proof.
- 2026-09-30 validate review round 5: Full frontend tests exposed an existing asynchronous Modbus settings assertion that read the field before loading. It now waits for the field, and the focused six-test administration suite passes. Both round-4/5 verdicts were invalidated by candidate changes; final review awaits the full rerun.
- 2026-09-30 validate review round 6: Specification compliance and implementation quality approved on the final candidate: 614/614 frontend tests, 12/12 browser viewport/locale/keyboard checks, frontend format and ESLint pass; live Core authority and failure paths were reread. WSL strict Core/integration proof remains to run.
- 2026-09-30 validate proof: WSL strict `check --task TASK-e4b3da95 --strict --run` exited 1 at the default Go ten-minute package timeout in unrelated `cmd/nilo` CLI tests; lint passed. With `GOFLAGS=-buildvcs=false -timeout=30m`, the full Go and Python phases advanced to the contract suite, where an old ignored `integrations/fake_python` local fixture caused one assertion failure; lint passed. The fixture was preserved outside the checkout at `D:\ProjectsDDP\nilo-validation-quarantine-20260930\fake_python`, and the focused contract suite then passed 11/11. The final strict rerun was canceled when the user directed full-suite execution only at the end of the overall Modbus plan. No strict gate is claimed green.
- 2026-09-30 validate targeted: WSL `go test ./internal/bench ./internal/httpapi ./internal/mcp ./cmd/nilo -run Modbus -count=1` exited 0 (MCP package had no matching name); targeted MCP authorization/discovery passed earlier. Frontend live-view suite passed 7/7; earlier full frontend suite passed 614/614, browser layout/locale/keyboard 12/12, and frontend format, ESLint and build passed. `git diff --check` on task paths exited 0. Evolution refreshed at 2026-09-30T19:57:49Z with 27 task paths (3 added, 24 modified), excluding seven unrelated dirty planning/media paths. All five task requirements are active, and round-6 specification and quality verdicts remain approved for the unchanged candidate.
- 2026-09-30 validate deviation: Per explicit user direction, full repository verification and a successful AEGIS strict run are deferred until the end of the full Modbus plan; MB-07 advances to delivery with targeted and segmented integration proof. CI-level confidence for this ticket is consequently limited, and the failed/canceled strict attempts remain visible above.
- 2026-09-30 transition: Validate accepted under the user's verification schedule; delivery is a presentation checkpoint. No Git staging, commit or push is authorized by the current instruction.
- 2026-09-30 deliver advisory: `aegis check --task TASK-e4b3da95 --strict` without `--run` exited 0 (`strict: ok`, 0 errors, 9 warnings, 10 notes). Existing task/register warnings remain; diff-based catalog checks were skipped because its internal `git diff` call failed, while task-scoped `git diff --check` exited 0. This advisory does not replace the deferred full gate.
- 2026-09-30 deliver approval: User explicitly authorized a local MB-07 commit on `ddp/modbus-client` and continuation to the next ticket on that branch. No push is authorized. The validated source candidate is unchanged; targeted proof and the deferred full-suite deviation were presented before this decision.
- 2026-09-30 deliver close-out: The AEGIS report was appended (0 errors, 16 warnings, 4 notes). Its base-diff count includes seven unrelated dirty paths, so the exact staged candidate was independently inspected: 35 MB-07 paths, all task files, no unrelated paths, and staged `git diff --check` exits 0. The approved outcome is one repository-local commit without push or PR; close-out is carried by that commit.

## Rulings

- RULING-001 | date=2026-09-30 | decision=Defer functional recording and Start/Stop logging to MB-08 | reason=User explicitly selected the MB-08 recording boundary | source=user | supersedes=none
- RULING-002 | date=2026-09-30 | decision=Use targeted tests per Modbus ticket and run the full suite only at the end of the complete Modbus plan | reason=User explicitly changed the verification schedule after repeated long full runs | source=user | supersedes=none

## Handoff


current: done
status: ready
next: Continue the next Modbus ticket in a new AEGIS journal on this branch.
must_read: .agentic/AGENTS.md, .agentic/knowledge/modbus-integration.md
blockers: none

## Stage report

stage: deliver
result: completed
current: done
changed: MB-07 candidate spans 27 task paths: pinned workspace admission, Core group controls and codec, live grid/traffic/write/trends, localized responsive UI, tests, changelog and knowledge.
evidence: Targeted Modbus Go and 7/7 live-view tests pass; contract 11/11, earlier frontend 614/614, browser 12/12, format/lint/build pass; strict full run did not pass and is deferred by user direction.
issues: Full gate deferred to end of Modbus plan by user direction; existing catalog warnings remain. No push or PR authorized.
next: Continue next Modbus ticket on the same branch with targeted tests.
needs_user: no
spec_compliance: approved
implementation_quality: approved
review_round: 6

## Delivery report (2026-09-30)

- stage: deliver; type: feature; path: full; branch: ddp/modbus-client
- requirements:
  - REQ-UX-04cd31fe - Operate a live Modbus endpoint workspace
  - REQ-UX-9d7fd3a6 - Show fresh and stale mapped Modbus register values
  - REQ-UX-35328cf5 - Preview and submit explicit Modbus writes
  - REQ-UX-e0682e6a - Inspect bounded Modbus traffic and trends
  - REQ-FUNC-c7473d9f - Control Modbus poll groups independently
- deviations: none recorded
- explicit review: enabled; spec_compliance=approved; implementation_quality=approved; review_round=6
- diff vs task base df1e53d: 42 file(s) changed, 4643 line(s) added; 5 file(s) not content-scanned
- checks: errors=0 warnings=16 notes=4
- checklist: 0/6 done; open: validated candidate confirmed unchanged; otherwise returned to validate; validation evidence, diff, exclusions, and outcome presented; delivery confirmed by the human; journal and report included in the delivery commit; agreed outcome delivered, normally a PR; close-out committed after the outcome succeeded, or carried by the delivery commit where nothing could fail after it
