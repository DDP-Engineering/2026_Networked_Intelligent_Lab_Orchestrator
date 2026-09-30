<!--
Copyright (c) 2026 DDP Engineering
This software is licensed under the PolyForm Noncommercial License 1.0.0.
Commercial use requires a separate commercial license.
See the LICENSE.txt in the .agentic folder for full license details.
SPDX-License-Identifier: LicenseRef-PolyForm-Noncommercial-1.0.0
-->


# TASK-4ac392a0 - MB-06 Modbus administration and Instruments navigation

stage: done
type: feature
path: full
interaction: autopilot
requirements: REQ-UX-5843b688, REQ-FUNC-df8cf7c1, REQ-FUNC-d2db4ee3, REQ-FUNC-7bed9136, REQ-PLAT-ca19fbda, REQ-UX-3f615e7d, REQ-UX-aa51d4be
parent:
branch: ddp/modbus-client
base: af63d00a1877acf3f473e9b58652826df3da152f
opened: 2026-09-30

## Done checklist (current stage)

- [x] validated candidate confirmed unchanged; otherwise returned to validate
- [x] validation evidence, diff, exclusions, and outcome presented
- [x] delivery confirmed by the human
- [x] journal and report included in the delivery commit
- [x] agreed outcome delivered, normally a PR
- [x] close-out committed after the outcome succeeded, or carried by the delivery commit where nothing could fail after it

## Frame

Problem: MB-02/04/05 provide Core Modbus definitions, sessions and maps, but the browser has no Modbus administration or named endpoint navigation. Gitea #33 is the next work package.
Desired outcome: Administrators configure and inspect Modbus lifecycle, endpoints, profiles and bounded connection tests; all authorized users reach named endpoint workspaces, including offline targets and inert saved definitions.
Actors: Administrator, Operator, Viewer, Bench Core, supervised Modbus runner, browser user.
Primary scenario: An administrator opens System Plugins Modbus, discovers USB adapters or saves a network target and profile, tests a connection through Core policy, and an Operator opens that named endpoint's RealTime View, Sessions or Register Maps without an implicit connection.
Examples: An enabled TCP endpoint remains listed when unreachable; an unplugged RTU adapter keeps its name and offline status; one successful test closes its temporary lease.
Counterexamples: Discovery opens a tty, Viewer changes an endpoint, Test bypasses plugin policy, an offline target disappears from Instruments, or Uninstall silently makes retained definitions inaccessible.
In scope: Modbus-specific browser administration and endpoint navigation; revisioned endpoint/profile forms, adapter discovery presentation, policy-controlled bounded connection testing, lifecycle/busy feedback, Modbus-specific operational settings aligned with #25, saved-definition access after disable/uninstall, responsive keyboard-friendly EN/DE/RO states and tests. Core additions only where an accepted seam is missing.
Out of scope: MB-07 live grid/traffic/trends and typed write UI, MB-08 recording persistence/viewer, MB-09 scans/advanced functions, #26 global collapsed shell/About landing rewrite, physical I/O or deployment, Git staging/commit/push, unrelated planning and media files.
Constraints: STATED user choices: client only, RTU-over-TCP only for RTU over IP, plugin-level successive-write policy, shared definitions with one live owner, same branch, AEGIS autopilot. OBSERVED #33 limits scope to MB-06 and does not authorize Git mutation. Existing Core `/api/v1/modbus` routes require plugin management for saves and Bench read for lists; existing discovery owns USB observations without opening descriptors. No new Bench section is created.
Success signals: Admin/role tests reject unauthorized changes; discovery is read-only; test connection has finite deadline and policy admission; named offline/empty states stay visible; disabled/uninstalled definitions remain readable; keyboard/viewport/language checks pass; legacy Camera/Serial navigation remains working.
Evidence: STATED current user instruction; OBSERVED Gitea #33, #25, #26, `00_Planning/Modbus_Plugin_Plan.md` sections 3/7/9, `.agentic/knowledge/modbus-integration.md`, Core `modbus_definitions.go`, `modbus_session.go`, frontend `pages.ts`, `SideNav.tsx`, `CameraPluginPage.tsx`, `api/client.ts`. INFERRED MB-06 should add a distinct Modbus browser page and reuse existing Core metadata/lifecycle services. UNKNOWN physical adapter availability and target-specific limits.
Assumptions: The pending #26 global navigation change remains separate; MB-06 adds only the Modbus subtree under current Instruments/System behavior. MB-07/08 own live-grid and retained-recording content; MB-06 supplies honest initial endpoint pages and definition links.
Unknowns: Exact Modbus-specific setting ranges and effect on active sessions must be aligned with Core capacity and runner bounds during design. Device-specific connection responses remain unverified without hardware.
Decision-blocking questions: None; the ticket, confirmed plan and current Core boundaries settle implementation scope. Physical acceptance remains explicitly later.
Proposed behaviors: Add a Modbus admin page driven by authenticated Core APIs, a policy-controlled finite connection test, saved settings where needed, dynamic named endpoint navigation, and useful inert endpoint pages. Verify role, error, offline, lifecycle and locale behavior against mock and Core tests.
Path and reason: Full path: new browser actions, public connection-test behavior, persistent settings and navigation change observable behavior and data handling. Existing branch `ddp/modbus-client` is reused under the user's continuing-ticket instruction; no subagents or Git mutation.

| Quick-path criterion | Yes | No | Evidence |
|---|---|---|---|
| No new externally observable behavior | | x | New admin and endpoint pages. |
| No new interface/data format | | x | Connection test and settings interfaces. |
| No new dependency | x | | Existing Core and frontend stack. |
| Existing requirement covers behavior | | x | MB-06 needs explicit new contracts. |
| No security/data-handling change | | x | Admin-only configuration and policy-controlled I/O. |

## Requirement coverage

| Frame outcome | Actor/system | Trigger or state | Response | Positive case | Boundary or failure | Observable result |
|---|---|---|---|---|---|---|
| Plugin administration | Administrator/browser | Opens Modbus page or selects lifecycle/policy | Show verified lifecycle and action outcome | Update and View activity | Busy, absent or other role | Controls and feedback |
| USB discovery | Administrator/Core | Explicit refresh | Read metadata and reconcile | Present adapter | Unplugged or failed discovery | Retained endpoint state |
| Endpoint/profile editing | Administrator/browser/Core | Save named definition | Validate and CAS | Offline TCP target saved | Stale revision or invalid target | Persisted revision or error |
| Test connection | Administrator/Core | Submit test | Policy job probes and closes | Auto completes | Ask, stale, timeout, failure | Job result and no active lease |
| Operating settings | Administrator/Core | Save settings | CAS and apply to new sessions | Raise capacity | Bad range, stale, lowered capacity | Persisted settings and admission result |
| Named workspaces | Bench reader/browser | Open Instruments Modbus | Navigate endpoint and inert definitions | Online or offline endpoint | Disabled/uninstalled plugin | Pages or retained definition access |
| Accessible presentation | Browser user | Admin/endpoint page visible | Localized keyboard and reflow behavior | EN/DE/RO desktop | 320/field width, keyboard | Usable controls and no overflow |

| Frame outcome | Atomic behavior | Requirement | Positive case | Boundary or failure | Verification | Design decision or no impact |
|---|---|---|---|---|---|---|
| Plugin administration | Administrator lifecycle and policy interface | REQ-UX-5843b688 | Ready plugin controls | Other roles/busy/absent | UI role and lifecycle tests | Reuse common plugin actions |
| USB discovery | Metadata-only adapter refresh | REQ-FUNC-df8cf7c1 | Present adapter | Unplugged/failure/forbidden | Fake runner and Core tests | Core trusted reconciliation |
| Endpoint/profile editing | Revisioned form submission | REQ-FUNC-d2db4ee3 | Offline TCP and USB records | Stale/invalid/non-admin | Client/UI form tests | Existing Core definition API |
| Test connection | Finite one-use policy probe | REQ-FUNC-7bed9136 | Auto completes and closes | Ask/stale/timeout/cancel | Fake runner and surface tests | `connect` policy, test-only parameter |
| Operating settings | Persist and enforce new-admission settings | REQ-PLAT-ca19fbda | Raise session capacity | Invalid/stale/lowered capacity | SQLite/Manager tests | Recording limits stored for MB-08 |
| Named workspaces | Endpoint subtree and retained definitions | REQ-UX-3f615e7d | Named pages offline and online | Disabled/uninstalled plugin | Nav/page tests | No #26 shell rewrite |
| Accessible presentation | Locale/keyboard/responsive page contracts | REQ-UX-aa51d4be | Three languages and desktop | 320/field viewports | UI and geometry checks | Existing shell conventions |

## Design

Requirements coverage: `REQ-UX-5843b688` uses a dedicated Modbus administrator page over existing plugin lifecycle/policy actions. `REQ-FUNC-d2db4ee3` uses the existing revisioned TCP endpoint/profile services and a typed browser client. `REQ-FUNC-7bed9136` adds a test-only connect intent. `REQ-PLAT-ca19fbda` adds one revisioned SQLite setting record. `REQ-UX-3f615e7d` builds named endpoint pages from Core definitions; `REQ-UX-aa51d4be` uses existing responsive and localization primitives. `REQ-FUNC-df8cf7c1` is retired for TCP-only scope.

Component responsibilities: Bench Core owns authority, endpoint/profile normalization, policy jobs, transport claims and settings. The browser only sends canonical intents and renders results. The supervised runner retains transport ownership. The `plugins` lifecycle remains common to Camera/Serial/Modbus; Modbus gains its own admin page because endpoints and profiles are distinct from managed camera/serial device records. [six active requirements]

Interfaces: `POST /api/v1/modbus/endpoints/{id}/test` submits the existing `connect` policy operation with `test_only` and a pinned profile; CLI/MCP mirror it. `GET/PUT /api/v1/modbus/settings` read/replace one CAS record. Existing endpoint/profile and plugin routes remain unchanged. The frontend `api/modbus.ts` validates full response bodies and defines typed requests; `BenchCoreClient` routes them through its authenticated exchange. [REQ-FUNC-d2db4ee3, REQ-FUNC-7bed9136, REQ-PLAT-ca19fbda]

Data and flow: Migration 027 seeds the bounded settings defaults; `bench.Manager` loads them at startup under its lock. Session admission checks the current max count; each new session snapshots lease/idle lifetimes so updates never shorten active work. Retention values are persisted and clearly marked future recording controls until MB-08 consumes them. Settings changes and test requests are audited. [REQ-PLAT-ca19fbda, REQ-FUNC-7bed9136]

Test-only connect follows existing claim, runner start and readiness verification; after readiness or error it cancels, waits for cleanup within its finite job deadline and returns a terminal result without exposing a controllable live session. Policy Ask remains the existing approval route. Idempotency uses the ordinary finite job key, so repeating a key does not run a second probe. The probe never sends a Modbus PDU. [REQ-FUNC-7bed9136]

The admin page lists existing endpoints and saved profiles separately. The editor creates and changes TCP peers and unit ranges and never accepts a client tty path. Existing RTU definitions remain read-only. CAS errors keep the form draft and show the error. Lifecycle controls use existing verified local upgrade and confirmation patterns. The UI remains available to an administrator after uninstall to expose retained definitions. [REQ-UX-5843b688, REQ-FUNC-d2db4ee3]

`AppShell` fetches endpoints independently of plugin inventory, retains the last good list on refresh failure, and passes it to `buildPages`. Only installed/enabled Modbus yields named endpoint children (RealTime View, Sessions, Register Maps), including configured-offline enabled targets. A read-only Modbus definitions destination remains for Bench readers after disable/uninstall; no page mount starts a runner. MB-07/08 fill live grid and recording views later; MB-06 pages report actual session/definition state and honest unimplemented recording status. The pending #26 shell rewrite is not copied into this ticket. [REQ-UX-3f615e7d]

Failure and recovery: A timed-out/cancelled test releases or fences the claim under the existing cleanup rule. Invalid settings and stale revisions publish nothing. GUI handles 403/401 separately from Core's rejected/error states and does not clear a last good inventory on transient refresh failure. [REQ-FUNC-7bed9136, REQ-PLAT-ca19fbda, REQ-UX-5843b688]

Security and compatibility: `plugins.manage` guards lifecycle, settings, endpoint/profile saves and connection tests in Core and the UI; Bench read guards shared definition reads. Existing `connect` policy controls tests. No implicit connection or write appears on page open; no new browser-to-device path or extra MCP server. Migration 027 adds a table without changing stored endpoints, profiles, maps or active plugin records. [six active requirements]

Tests: Core fake-runner tests prove test cleanup; SQLite/Manager tests prove settings CAS and new-admission behavior; HTTP/CLI/MCP tests prove strict scope and payloads. Frontend API, page and navigation tests prove roles, online/offline/empty/uninstalled states and localized responsive keyboard flows. The final offline full gate is required; physical hardware acceptance remains outstanding. [six active requirements]

Alternative: Calling the ordinary Connect action from the browser and immediately disconnecting was rejected because a closed tab or lost response could leave a long-lived live lease. A Core-owned test-only finite job controls cleanup independently of browser lifetime. [REQ-FUNC-7bed9136]

Risks: Retention settings have no recording engine yet; the page must state that boundary. The current shared Manager lock protects transport claims and plugin lifecycle; the new runner wait must occur outside it. Open decisions: none for this ticket; physical performance and device-specific limits remain target acceptance.

Expected files: Core `internal/bench/{modbus_adapter,modbus_session,modbus_settings,manager}.go`, `internal/httpapi/modbus.go`, `internal/mcp/modbus.go`, `cmd/nilo/modbus.go`, SQLite migration 027 and adjacent tests; frontend `api/{modbus,client}.ts`, `components/pages/{ModbusPluginPage,ModbusWorkspacePage}.tsx`, `components/shell/{pages,AppShell}.tsx`, `i18n/modbus.ts`, responsive CSS and focused tests; current-state context, component knowledge and changelog.

### Proposed architecture - TASK-4ac392a0

`ModbusPluginPage -> api/modbus -> BenchCoreClient -> HTTP modbus routes -> bench.Manager`
`CLI / MCP -> HTTP or bench.Manager -> test-only connect job -> policy -> temporary runner and claim`
`bench.Manager -> SQLite modbus_settings and existing definition tables`
`AppShell -> ModbusEndpoints -> buildPages -> named ModbusWorkspacePage destinations`

## Log

- 2026-09-30 frame: Full MB-06 path on the existing `ddp/modbus-client` branch; scope follows #33 and the confirmed plan, with #26 shell rewrite and MB-07/08/09 content excluded. User's autopilot preference carries forward; no Git mutation authorized.
- 2026-09-30 specify: Created seven draft atomic requirements and two specs (zero revised/reused/retired). Existing Core definition authorization remains governed by its active requirement; MB-06 adds administrator browser behavior, trusted refresh, finite test and settings contracts.
- 2026-09-30 design: `## Design` fixes Core ownership, revisioned settings and test-only policy admission, with separate browser administration and inert named endpoint pages. `aegis check` reports zero format errors; prospective paths and unrelated older findings remain advisory.
- 2026-09-30 build red: `with-tools.ps1 npm.cmd --prefix .\05_Software\frontend run test:ci -- src/components/shell/pages.test.ts` exited 1 as expected: the Modbus group, offline endpoint children and retained definition destination are absent (REQ-UX-3f615e7d).
- 2026-09-30 build: The corrected shell test failed because Core endpoints were not fetched; `pages.test.ts` and `shell.test.tsx` now pass together (58 tests). The authenticated client parses Core's `endpoints` envelope, the shell refreshes definitions independently and retains last-good data on transient failure. Frontend build passed.
- 2026-09-30 build deviation: Retired `REQ-FUNC-df8cf7c1` and removed the proposed explicit USB discovery route, CLI/MCP action and test because the user's only-TCP constraint excludes it; Core's existing RTU behavior remains unchanged.
- 2026-09-30 validation: Canonical AEGIS full test exited 1 in Go after 220 repository-tooling tests passed. The manifest test expected one operation although the established package declares `connect` and `serve`; several parallel Windows Go tests missed timing deadlines. The exact six failed selectors passed in a focused serialized rerun. Corrected the stale manifest assertion as a related unplanned test-file change; full proof is being rerun.
- 2026-09-30 test: test exit 1 (464.0s)
- 2026-09-30 test: test exit 1 (428.5s)
- 2026-09-30 build: Focused Windows Go settings, HTTP, MCP and CLI tests and Linux fake-runner session/admission tests passed. Frontend API/navigation tests passed (60), administrator page tests passed (6), and Chrome EN/DE/RO keyboard/reflow checks passed (6) at 320, 800x480, 1024x600 and desktop. Lint and frontend build passed. Settings, test-only admission, revisioned forms, named navigation and localization are implemented; only the navigation increment has a durable preimplementation red test record.
- 2026-09-30 build: Evolution reviewed 39 paths: 38 ticket paths plus concurrent `09_Others/Nilo_Overview_2026-09-30.pptx`, which is not this task's candidate. Other concurrent planning/media paths remain untouched. Advisory check found zero errors and no new candidate-specific warnings; its 14 warnings and four notes concern older records and unrelated concurrent files. The Linux full Go run reached its ten-minute per-package timeout in nested Git/LFS helper builds; focused Linux Modbus tests passed. Validation will rerun with VCS stamping disabled in the test process.
- 2026-09-30 validate review round 1: Specification verdict approved after checking all six active requirements, the retired TCP-only USB draft, public routes, settings bounds, policy admission, offline navigation and EN/DE/RO layout evidence. Implementation-quality verdict approved after reviewing Core/CLI/MCP/browser diffs, permission and cleanup paths, database migration, error/CAS behavior, portability, `git diff --check`, and `gofmt -l`; page tests were strengthened for profile revisions, policy busy rejection and RTU read-only state. The current-state Modbus knowledge page was consolidated to the component template. No manual-only requirement verification is declared; physical device acceptance remains a separate target gate. Refreshed evolution shows 38 ticket paths and two concurrent unrelated files (`00_Planning/Overview.md`, `09_Others/Nilo_Overview_2026-09-30.pptx`).
- 2026-09-30 validate: Windows `check --strict --run` passed full Go and lint but exited 1 in three established Linux-only runner workspace tests (`os.geteuid` absent on Windows). A separate full frontend suite exposed eight German and three Romanian labels copied from English; corrected the catalog, then 607/607 frontend tests, production build and six Chrome layout tests passed. Review round 2 specification and quality verdicts approved for that localized candidate after inspecting all changed strings and their layout. WSL `check --strict --run` with host-volume set passed Go/Python integration suites and lint but exited 1 in the final contract test because an ignored local `integrations/fake_python` fixture predating this task existed. Preserved it in ignored `02_Tools/cache/mb06-fake-python-stash`; the targeted WSL contract suite then passed 11/11. Full strict proof will rerun from this clean test state.
- 2026-09-30 validate final: WSL2 `GOFLAGS=-buildvcs=false GOMAXPROCS=2 NILO_WSL_HOST_VOLUME=/mnt/d .agentic/aegis.py check --task TASK-4ac392a0 --strict --run` exited 0: configured tooling, complete Go and Python integration/contract tests passed in 388.4s; lint passed in 161.3s. Windows full frontend suite passed 607/607, build passed, Chrome EN/DE/RO layout passed 6/6, and Windows `check --task TASK-4ac392a0 --strict` exited 0 with zero errors. WSL catalog could not run Git-diff checks on the shared checkout; Windows catalog covered them. Its eight warnings concern older requirements/journals; Windows catalog's 14 warnings additionally concern unrelated concurrent planning/media. Physical hardware acceptance remains outstanding. Restored the ignored fake-Python fixture after proof. Refreshed evolution after final candidate edit; two unrelated concurrent paths in its report were excluded from this task. No Git state changed.
- 2026-09-30 deliver presentation: The unchanged validated candidate consists of 50 currently modified/untracked task paths: Core settings and test-only policy admission with HTTP/CLI/MCP, browser administration/navigation/localization and their tests, changelog and AEGIS records. Seven concurrent planning/media paths are excluded. No child task was used. No current user instruction authorizes staging, committing or pushing MB-06; delivery remains waiting for a specific Git instruction under root `AGENTS.md`.
- 2026-09-30 deliver approval: User instructed a local commit for the reviewed MB-06 candidate. Branch and 50 candidate paths are unchanged; the staging area is empty. This instruction authorizes staging and committing only, with no push or PR.
- 2026-09-30 deliver close-out: The approved outcome is one local MB-06 commit on `ddp/modbus-client`; the delivery report and this close-out record are part of that commit. The seven unrelated concurrent planning/media paths remain excluded. No push or PR is part of this outcome.

## Rulings

- RULING-001 | date=2026-09-30 | decision=Omit new RTU and USB discovery controls from MB-06 while preserving established Core behavior | reason=The user's earlier only-TCP direction governs this feature | source=user | supersedes=none

## Handoff


current: done
status: ready
next: Verify the local delivery commit and excluded worktree paths
must_read: .agentic/steps/6_deliver.md, .agentic/tasks/TASK-4ac392a0.md
blockers: none

## Stage report

stage: deliver
result: completed
current: done
changed: Prepared approved local MB-06 delivery with its report and close-out; seven unrelated concurrent paths excluded
evidence: WSL strict full gate exit 0; Windows frontend 607/607, build, Chrome 6/6 and strict diff catalog pass; user authorized local commit
issues: WSL diff checks required separate Windows catalog; unrelated catalog warnings remain; hardware acceptance is later
next: Verify the local commit hash and confirm no push occurred
needs_user: no; local commit authorized, push remains unauthorized
spec_compliance: approved
implementation_quality: approved
review_round: 2

## Delivery report (2026-09-30)

- stage: deliver; type: feature; path: full; branch: ddp/modbus-client
- requirements:
  - REQ-UX-5843b688 - Administer Modbus plugin lifecycle and policy
  - REQ-FUNC-df8cf7c1 - Refresh trusted Modbus adapter discovery
  - REQ-FUNC-d2db4ee3 - Manage named Modbus endpoints and profiles
  - REQ-FUNC-7bed9136 - Test a Modbus connection through finite policy admission
  - REQ-PLAT-ca19fbda - Store bounded administrator Modbus operating settings
  - REQ-UX-3f615e7d - Navigate named Modbus endpoints and offline definitions
  - REQ-UX-aa51d4be - Present Modbus administration accessibly across supported displays
- deviations: none recorded
- explicit review: enabled; spec_compliance=approved; implementation_quality=approved; review_round=2
- diff vs task base af63d00: 58 file(s) changed, 4077 line(s) added; 5 file(s) not content-scanned
- checks: errors=0 warnings=15 notes=4
- checklist: 3/6 done; open: journal and report included in the delivery commit; agreed outcome delivered, normally a PR; close-out committed after the outcome succeeded, or carried by the delivery commit where nothing could fail after it
