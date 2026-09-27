<!--
Copyright (c) 2026 DDP Engineering
This software is licensed under the PolyForm Noncommercial License 1.0.0.
Commercial use requires a separate commercial license.
See the LICENSE.txt in the .agentic folder for full license details.
SPDX-License-Identifier: LicenseRef-PolyForm-Noncommercial-1.0.0
-->


# TASK-d4a1fded - Implement serial administration and terminal GUI

stage: done
type: feature
path: full
interaction: autopilot
requirements: REQ-UX-7338451f
parent: TASK-5344100e
branch: master
base: d2bb8be8803edb07b2f471937c12b5a6119f2320
opened: 2026-09-27

## Done checklist (current stage)

- [x] Implementation and integrated validation complete
- [x] Requested Pi deployment verified
- [x] User authorized task closure and local commit
- [x] Close-out included in this delivery commit

## Frame

Problem: STATED: USB serial instruments need administration, terminal and saved sessions.
Desired outcome: STATED: Camera-style navigation with bounded terminal and log management.
Actors: Administrators configure; authorized operators connect/send; readers inspect permitted data.
Primary scenario: Discover/configure a port, approve connection, receive/send and retain/export logs.
Examples: UTF-8 chunk boundaries; absent adapter; approval waiting; completed log export.
Counterexamples: Arbitrary device paths, hidden serial writes, unbounded browser history.
In scope: Frontend only and its requirement/test links.
Out of scope: Core/runner/client implementation, deployment, Git mutations.
Constraints: STATED: autopilot; OBSERVED: shared shell, i18n, CSRF and preferences ownership.
Success signals: Focused UI/API/formatting tests, build and responsive browser checks.
Evidence: accepted plan; usb-serial spec; camera UI and shell sources.
Assumptions: INFERRED: exact API contract is implemented by parent.
Unknowns: Physical adapter behavior; deferred by user.
Decision-blocking questions: None.
Proposed behaviors: Spec GUI section, including failure and authority status.
Path and reason: Full; new frontend feature across navigation, network boundary and interactive views.

## Requirement coverage

REQ-UX-7338451f maps to shell routing, serial API, administration, bounded terminal, sessions and focused component/browser tests.

## Design

Use existing generic plugin/device lifecycle APIs for admin. A dedicated serial API owns authenticated finite calls. A bounded terminal model handles bytes separately from virtualized presentation; settings persist only via preferences store. Shell routes serial device destinations to realtime/sessions components. Core owns all authorization, approvals, bytes and retention. No source ownership outside frontend. Risks: stale requests on device switch, duplicate TX after network failure, malformed binary rendering; mitigate abortable polling, retained send keys and plain text decoding. Existing Camera behavior remains under its current component. Shared Camera-style admin markup may be parameterized narrowly.

## Log

- 2026-09-27 frame: Child frontend scope accepted from parent under user autopilot. Git stays master; no mutation authorized.
- 2026-09-27 specify: REQ-UX-7338451f and shared usb-serial spec cover success, bounds and authority failures.
- 2026-09-27 design: Dedicated serial API/model/views and shared shell; no external dependencies or backend ownership.
- 2026-09-27 build red: REQ-UX-7338451f; command=`.\02_Tools\env\with-tools.ps1 npm.cmd --prefix .\05_Software\frontend run test:ci -- src/components/shell/serialNavigation.test.ts`; exit=1; expected missing behavior=managed serial inventory rejected, no grouped serial destinations and serial admin visible to viewer.

- 2026-09-27 build green: REQ-UX-7338451f; command=`.\02_Tools\env\with-tools.ps1 npm.cmd --prefix .\05_Software\frontend run test:ci -- src/components/shell/serialNavigation.test.ts src/components/shell/pages.test.ts src/components/pages/serialBuffer.test.ts src/components/pages/SerialWorkspacePage.test.tsx src/api/serial.test.ts src/components/pages/CameraPluginPage.test.tsx src/i18n/messages.test.ts`; exit=0; 34 tests cover managed navigation, empty discovery, split UTF-8/control escaping, history bounds, validation, CSRF, send retries, exports/deletion, Camera regressions and translated catalogue.
- 2026-09-27 build green: from frontend, command=`..\..\02_Tools\env\with-tools.ps1 node node_modules/@playwright/test/cli.js test --config playwright.serial.config.ts`; exit=0; four Chromium cases pass at 320x640, 800x480, 1024x600, 1920x1080 with keyboard expansion, target height and overflow checks. Mock HTTP is test-only, no physical claim.
- 2026-09-27 build: lint exit=0 and production build exit=0. Vite reports existing monolithic bundle size advisory; output remains functional. Current shared source contains parent/other child edits, which remain outside this child's ownership.
- 2026-09-27 validate: frontend contract and implementation reviewed; requirement active, GUI knowledge current. Parent owns the final integrated strict full-suite command per task assignment; child remains validate until that evidence is available. No Git or deployment operations performed.

- 2026-09-27 validate check: advisory errors=0; scoped coverage warnings concern concurrent parent/other-child Core files; outside-layout serial package paths are parent configuration reconciliation. Existing stale-path, tracked-sidecar and unrelated deliver warnings are pre-existing. Handoff format corrected.

- 2026-09-27 review round 1: state events could be reordered relative to later bytes on an open partial line, and UTF-8 C1 controls were invisible. Returned to build for these two findings. Focused command `.\02_Tools\env\with-tools.ps1 npm.cmd --prefix .\05_Software\frontend run test:ci -- src/components/pages/serialBuffer.test.ts` exit=1; expected missing behavior chronology and escaped C1, observed `[AB<U+0085>, state]` instead of `[A, state, B\x85]`.

- 2026-09-27 review fixes green: terminal regression and workspace tests exit=0 (10 tests); state events close the display line and C1 controls are escaped. Visual inspection found the Find target at 29.1875px; browser command from frontend `..\..\02_Tools\env\with-tools.ps1 node node_modules/@playwright/test/cli.js test --config playwright.serial.config.ts --grep 1024` exit=1 demonstrated the 44px violation. CSS corrects search/select/summary targets; full dedicated browser command exit=0 (four viewport cases). Screenshots in ignored `02_Tools/cache/serial-1024.png` and `serial-320.png` show wrapping, readable text and independent terminal scrolling.
- 2026-09-27 review round 2: specification compliance approved; implementation quality approved. Reviewed bound enforcement, separate direction decoders, ASCII/UTF-8/HEX and control escaping, virtualized output/find, approval and RTS/CTS control lock, permissions, error classes, idempotent send retry, exports/deletion and retention editor. No remaining actionable frontend finding. GUI knowledge original LF restored to remove format-only churn. Final integrated proof remains parent-owned.

- 2026-09-27 validate completed: parent TASK-5344100e reported final unchanged-candidate proof ` .\02_Tools\env\with-tools.ps1 python .agentic/aegis.py check --task TASK-5344100e --strict --run` exit=0. Test phase 427.2s covered tooling, native Go/Python, GUI tests/build, Linux appliance browser and integration contracts; lint 9.5s. Advisory errors=0, warnings=5 (pre-existing stale-path, tracked fingerprint and old journal findings), notes=4. Parent journal is the integrated evidence authority. Source remained frozen through this result.
- 2026-09-27 deliver: validated frontend candidate handed back to parent unpublished. Implementation and scoped validation complete; no Git or deployment action authorized or performed. Only this child journal changed after final proof.

- 2026-09-27 close-out authorized: User requested "close tasks, commit". Complete this task with the repository-local delivery commit, incorporating integrated full proof and Pi deployment in `07_Delivery/NILO_0.11.0_RPi_Live_Test.md`. Physical RX/TX and unplug/replug acceptance remain outside the completed implementation scope. No push requested.

- 2026-09-27 closed: Implementation, regression verification and requested Pi delivery are complete. Closing with the authorized local commit; latest integrated full tests/lint passed (414.0s/9.5s). No executable changes since that proof. Live physical acceptance limitations remain documented in the deployment record.

## Rulings


## Handoff

current: done
status: ready
next: No further implementation work; user live testing may continue
must_read: .agentic/tasks/TASK-d4a1fded.md, 07_Delivery/NILO_0.11.0_RPi_Live_Test.md
blockers: none

## Stage report

stage: deliver
result: completed
current: done
changed: Completed task closed in authorized local delivery commit; version 0.11.0 deployed
evidence: Latest integrated full tests 414.0s and lint 9.5s passed; Pi deployment verified
issues: Existing repository advisories; physical RX/TX and unplug/replug acceptance remain user testing
next: No further implementation work; user live testing
needs_user: no
spec_compliance: approved
implementation_quality: approved
review_round: 2

## Delivery report (2026-09-27)

- stage: deliver; type: feature; path: full; branch: master
- requirements:
  - REQ-UX-7338451f - Provide responsive serial terminal and sessions
- deviations: none recorded
- explicit review: enabled; spec_compliance=approved; implementation_quality=approved; review_round=2
- diff vs task base d2bb8be: 161 file(s) changed, 11462 line(s) added
- checks: errors=0 warnings=74 notes=4
- checklist: 3/5 done; open: Git delivery authorized by user; Authorized delivery outcome performed and recorded
