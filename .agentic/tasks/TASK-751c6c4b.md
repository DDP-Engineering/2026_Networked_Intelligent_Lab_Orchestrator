<!--
Copyright (c) 2026 DDP Engineering
This software is licensed under the PolyForm Noncommercial License 1.0.0.
Commercial use requires a separate commercial license.
See the LICENSE.txt in the .agentic folder for full license details.
SPDX-License-Identifier: LicenseRef-PolyForm-Noncommercial-1.0.0
-->


# TASK-751c6c4b - Camera plugin navigation and appliance administration

stage: done
type: feature
path: full
interaction: autopilot
requirements: REQ-UX-4f9b8d21, REQ-UX-37d846a0, REQ-UX-6ab5e73f, REQ-UX-c7e3c428, REQ-FUNC-a46dc9e4
parent:
branch: NILO_camera_plugin
base: b83555684d11c359c41676a962dc0f0860e1335d
opened: 2026-09-25

## Done checklist (current stage)

- [x] validated candidate confirmed unchanged
- [x] validation evidence, diff, exclusions, and outcome presented
- [x] delivery confirmed by the human
- [x] journal and report included in the local delivery commit
- [x] agreed local commit outcome delivered
- [x] close-out carried by the local delivery commit

## Frame

Problem: Camera work, package lifecycle and persistent configuration share one Plugins page; the content scrollport ends before the workspace edge.
Desired outcome: System owns appliance Camera administration, Bench owns named physical Camera work, and the sidebar presents nested destinations with an edge-aligned scrollport.
Actors: Administrator, operator, viewer, Bench Core and the Camera runner.
Primary scenario: Administrator configures a discovered physical cam-N; Bench then shows its assigned name, Snapshot and Settings, while System offers lifecycle and persistent configuration.
Examples: The attached Pi USB camera is cam-1 and can be named Board view without changing its route ID.
Counterexamples: An unconfigured provisional camera is not a Bench instrument; a stale device route must not capture a different camera; a viewer cannot mutate appliance settings.
In scope: Camera route tree, inventory refresh, page separation, RBAC presentation, scrollbar placement, localization, focused tests and Pi inspection/deployment under the user's earlier authorization.
Out of scope: Sigrok reimplementation and synthetic Camera devices/images. Live View remains gated by CAM-10 physical feasibility and accepted CAM-11 design; the unavailable HDMI dongle and Ubuntu mini PC cannot be claimed as tested.
Constraints: Physical Camera runner only; existing server permissions and stable cam-N records; no Git state changes without a specific user instruction.
Success signals: Camera appears in System when installed and Bench when enabled; configured records have stable nested routes; admin-only forms stay in System; physical snapshot workflow remains usable; scrollbar reaches the workspace edge.
Evidence: STATED—the user requested these GUI changes and excluded Sigrok and simulated Camera data. OBSERVED—`PluginsPage.tsx` combined lifecycle and Camera work; `pages.ts` used older integration inventory; `.page-body` max width limited its scrollport; `internal/httpapi/bench.go` protects mutation with plugins.manage.
Assumptions: Current physical Camera package inventory is available through the authenticated listPlugins endpoint.
Unknowns: CAM-10/11 performance and transport decision on the unavailable hardware matrix.
Decision-blocking questions: None for the authorized navigation and administration implementation.
Proposed behaviors: Recursive Camera routes, persistent configuration on System pages, physical work on Bench pages, stale inventory retention and edge scrollbar.
Path and reason: Full path because visible navigation and permission-scoped page contracts change. The explicit user request to implement the plan authorizes stage continuation, recorded here as autopilot; repository Git authority still forbids Git mutations.

## Requirement coverage

| Frame outcome | Requirement | Positive case | Boundary | Verification | Design |
|---|---|---|---|---|---|
| Nested Camera routes | REQ-UX-4f9b8d21 | installed/enabled configured Camera | disabled, unconfigured, stale read | pages and sidebar tests | package inventory drives stable route IDs |
| Administration split | REQ-UX-37d846a0 | admin System mutation | viewer and Bench route | component and HTTP checks | scoped page views reuse guarded client |
| Edge scrollport | REQ-UX-6ab5e73f | long Plugins page | narrow/zoomed viewport | manual Chrome inspection | full-width `.page-body` |
| Physical Camera work | REQ-UX-c7e3c428 | snapshot and retained viewer | absent device | existing GUI and Pi tests | preserve CameraSection work controls |
| Persistent record config | REQ-FUNC-a46dc9e4 | revisioned save | stale/rebind/forbidden | existing component and backend tests | preserve managed record API |

## Design

D1 (REQ-UX-4f9b8d21): `AppShell` polls authenticated `listPlugins` and keeps last good inventory; `pages.ts` builds stable Camera System and Bench routes; recursive `SideNav` renders the tree. An initial unreadable inventory leaves Camera routes unknown until retry.
D2 (REQ-UX-37d846a0, REQ-UX-c7e3c428, REQ-FUNC-a46dc9e4): `PluginsPage` selects catalog, Camera administration, overview, snapshot, settings or legacy Sigrok view. `CameraSection` confines persistent configuration to administration and binds physical work to its exact record ID. Server RBAC remains authoritative.
D3 (REQ-UX-6ab5e73f): `.page-body` fills the grid workspace and owns vertical scrolling. Tables retain their local horizontal scrollers.
Failure/recovery: inventory read failures preserve tabs; lifecycle/configuration errors remain visible; disabling a package prunes unavailable work tabs after a successful refresh. No Camera simulation provider is added.
Compatibility: existing Sigrok runner and route continue unchanged. CAM-10/11 Live View remains gated on physical feasibility and is omitted from the registry until supported.
Test seams: pages and SideNav tests, PluginsPage role/workflow tests, full GUI suite, physical Pi browser inspection.

### Proposed architecture - TASK-751c6c4b

```text
Bench Core listPlugins -> AppShell -> pages.ts -> SideNav
                                      |          -> System Plugins / Camera admin
                                      -> Bench Camera / cam-N / Snapshot / Settings
PluginsPage -> CameraSection -> authenticated Bench API -> physical runner
```

## Log

- 2026-09-25 frame/specify/design: Full Camera-focused scope accepted from the user's implementation instruction; three navigation, administration and scroll requirements created, two physical Camera requirements reused. Sigrok replacement and CAM-10/11 hardware gate remain separate.
- 2026-09-25 build red/green: New pages tests failed in three route cases and recursive sidebar test failed on nested Settings, then passed after route and sidebar implementation. The page split initially broke 10 legacy page tests; their routes were updated to explicit views and all passed. Full GUI suite reached 521 before two more RBAC/catalog tests were added.
- 2026-09-25 build/validate: Scoped Camera catalog, System administration, overview, exact-device Snapshot and Settings views; added admin/viewer and catalog tests. GUI suite reached 523, lint and production build passed. GUI-only Pi bundle SHA-256 d34e20a2e4a4b6ddb1424de9140a4b44459fc2b9eed57b100c530a486563bfe1 was installed with rollback archive `/var/backups/nilo/pre-camera-navigation-20260925T1002Z/gui.tar.gz`; service active and new JS served.
- 2026-09-25 validate finding: First strict gate failed because the already edited Camera packaging input lock had unsorted incomplete reasons and Go was absent from the invoking shell PATH. Sorted the lock and will rerun through the repository-local tool wrapper. The late evolution anchor observes the current tree only and cannot reconstruct the pre-task candidate.
- 2026-09-25 validate correction/proof: Converted the Camera bundle check to the repository's unittest runner, sorted generated missing-input reasons and completed the Camera archive inventory fixture. The 21 packaging input tests passed (2 Windows skips). Final command `.\02_Tools\env\with-tools.ps1 python .agentic/aegis.py check --task TASK-751c6c4b --strict --run` exited 0: full test and lint commands passed, with 85 advisory warnings from the broader in-progress branch and 4 manual notes. The built index and Pi HTTPS index both reference `index-CkIcMUsi.js` and `index-CqD429I4.css`; `git diff --check` is clean.
- 2026-09-25 validate limitation: Authenticated Pi Chrome inspection awaits the user's sign-in after the browser session expired. CAM-10/11 physical live-preview acceptance awaits the HDMI capture target and Ubuntu mini PC; no synthetic Camera stream or menu was added.
- 2026-09-25 validate completion: The user reported that the deployed live GUI is verified and everything is OK. This accepts the Pi GUI navigation and Camera administration behavior; the specific viewport and zoom matrix has no separate observation record. The final strict check already exited 0 after the last candidate change. Advanced to deliver without changing the validated code.
- 2026-09-25 deliver presentation: The Camera GUI candidate is deployed on the Pi and accepted by the user. The branch also contains broader in-progress Camera work and remains uncommitted; no Git operation is authorized by the user's request to close tasks. CAM-10/11 live view, target hardware checks and offline runtime inputs remain separate open work.
- 2026-09-25 validate deviation: Accepted the user's successful live Pi GUI check as manual acceptance for this task without separate observations for every planned viewport and zoom setting. That detailed matrix remains unproved.
- 2026-09-25 deliver decision: The user explicitly requested a commit. The approved outcome is one local commit of the current Camera source, tests, requirements, plans and journals on `NILO_camera_plugin`; generated `.repowise/` cache is excluded. No push or PR is authorized. Other Camera journals remain open for their own unfinished work.
- 2026-09-25 deliver close-out: The GUI task is complete for the approved local-commit outcome; this record and its delivery reports are included in that commit. No remote publication is claimed.

## Rulings


## Handoff


current: done
status: ready
next: Continue the remaining Camera software and physical acceptance tasks; publish the local commit only on separate instruction.
must_read: .agentic/tasks/TASK-751c6c4b.md
blockers: None for this GUI task; CAM-10/11 and target hardware checks remain separate tasks.

## Stage report

stage: deliver
result: completed
current: done
changed: Camera inventory route tree, recursive navigation, scoped System and Bench pages, edge scrollport, localization and regression tests.
evidence: Final strict AEGIS check exited 0 after full tests and lint; 523 GUI tests pass. Pi serves the deployed bundle; the user verified the live GUI and reported it OK.
issues: Viewport/zoom matrix lacks separate observations; CAM-10/11 and target hardware/input checks remain open in other tasks. The broader branch has advisory warnings.
next: Continue broader Camera tasks; the local commit is ready for later publication if requested.
needs_user: no
spec_compliance: approved
implementation_quality: approved
review_round: 1

## Delivery report (2026-09-25)

- stage: deliver; type: feature; path: full; branch: NILO_camera_plugin
- requirements:
  - REQ-UX-4f9b8d21 - Navigate installed Camera packages and named physical devices
  - REQ-UX-37d846a0 - Separate Camera appliance administration from physical work
  - REQ-UX-6ab5e73f - Place the page scrollbar at the workspace edge
  - REQ-UX-c7e3c428 - Operate physical cameras in the browser
  - REQ-FUNC-a46dc9e4 - Configure and administer managed cameras
- deviations: none recorded
- explicit review: enabled; spec_compliance=approved; implementation_quality=approved; review_round=1
- diff vs task base b835556: 369 file(s) changed, 16114 line(s) added; 200 file(s) not content-scanned
- checks: errors=0 warnings=292 notes=4
- checklist: 2/6 done; open: delivery confirmed by the human; journal and report included in a delivery commit; agreed published outcome delivered; close-out committed after the outcome succeeded

## Delivery report (2026-09-25)

- stage: deliver; type: feature; path: full; branch: NILO_camera_plugin
- requirements:
  - REQ-UX-4f9b8d21 - Navigate installed Camera packages and named physical devices
  - REQ-UX-37d846a0 - Separate Camera appliance administration from physical work
  - REQ-UX-6ab5e73f - Place the page scrollbar at the workspace edge
  - REQ-UX-c7e3c428 - Operate physical cameras in the browser
  - REQ-FUNC-a46dc9e4 - Configure and administer managed cameras
- deviations: none recorded
- explicit review: enabled; spec_compliance=approved; implementation_quality=approved; review_round=1
- diff vs task base b835556: 369 file(s) changed, 16131 line(s) added; 200 file(s) not content-scanned
- checks: errors=0 warnings=292 notes=4
- checklist: 2/6 done; open: delivery confirmed by the human; journal and report included in a delivery commit; agreed published outcome delivered; close-out committed after the outcome succeeded
