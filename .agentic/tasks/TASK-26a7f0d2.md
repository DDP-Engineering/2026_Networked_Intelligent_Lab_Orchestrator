<!--
Copyright (c) 2026 DDP Engineering
This software is licensed under the PolyForm Noncommercial License 1.0.0.
Commercial use requires a separate commercial license.
See the LICENSE.txt in the .agentic folder for full license details.
SPDX-License-Identifier: LicenseRef-PolyForm-Noncommercial-1.0.0
-->


# TASK-26a7f0d2 - CAM-05 through CAM-08 physical Camera administration, workspace and package access

stage: build
type: feature
path: full
interaction: autopilot
requirements: REQ-UX-c7e3c428, REQ-FUNC-a46dc9e4, REQ-PLAT-26896f80, REQ-PLAT-c65256e9
parent:
branch: NILO_camera_plugin
base: b83555684d11c359c41676a962dc0f0860e1335d
opened: 2026-09-25

## Done checklist (current stage)

- [x] Complete physical Camera administration and composition paths.
- [x] Integrate interpreter-closed Camera bundles into offline release candidates.
- [x] Verify available Pi hardware and retain deferred hardware acceptance gaps.

## Frame

Problem: Camera operation existed but its administration and browser workflow were incomplete, and the bench bundle depended on target Python.
Desired outcome: An administrator can configure physical cameras and the package; an operator can compose and inspect retained snapshots; appliance packaging grants device access and can carry a verified interpreter.
Actors: Administrator, operator, viewer, appliance service and package builder.
Primary scenario: Connect a USB camera, discover/configure it, capture a composed image, inspect its owner-scoped history, then run the same package after deployment.
Examples: Pi FaceCam `cam-1` captured a 320x240 PNG with crosshair and text overlay, transaction `01a0d53c-470d-7580-b1b4-43cb357e34ca`.
Counterexamples: A camera without readiness cannot submit, an active camera cannot be reconfigured, and stale or unauthorised administration is refused.
In scope: CAM-05 to CAM-08 browser, admin, package builder and V4L2 access policy; physical cameras only.
Out of scope: Camera simulation code/data, HDMI/Ubuntu acceptance until equipment is available, and phase-2 preview until its hardware gate.
Constraints: Revisions, manifest schemas, owner-scoped images, bounded preview, exact-ID confirmation, offline target closure and non-world-readable device nodes.
Success signals: Chrome physical snapshot, green focused and full suites, target package with embedded interpreter, automatic `nilo` video access.
Evidence: STATED user requested the full physical Camera plan and explicitly excluded simulated Camera code/data; OBSERVED `CameraSection.tsx`, `managed_devices.go`, `build-camera-bundle.py`, packaged Pi transaction.
Assumptions: A preverified architecture-matched Python runtime and pinned wheelhouse can be supplied to the offline builder.
Unknowns: HDMI capture and Ubuntu mini PC behavior; font licensing and target runtime closure until input artifacts are available.
Decision-blocking questions: None for current host implementation.
Proposed behaviors: Revisioned admin forms; generated controls and composition; saved-image viewer; indexed target runtime and udev group access.
Path and reason: Full path for new user interface, package and service-access contracts; user instruction allows stage progression without repeated checkpoints.

## Requirement coverage

| Outcome | Requirement | Positive | Boundary | Proof |
|---|---|---|---|---|
| Browser Camera workflow | REQ-UX-c7e3c428 | physical snapshot and retained preview | unavailable camera | GUI suite and Pi Chrome |
| Admin configuration | REQ-FUNC-a46dc9e4 | revisioned settings and rebind | stale/active/unauthorised | store and browser tests |
| Target runtime | REQ-PLAT-26896f80 | embedded interpreter | wrong arch/tampered inventory | builder test; target closure pending |
| Device access | REQ-PLAT-c65256e9 | video-group rule | world-readable nodes refused | package test; reboot proof pending |

## Design

D1: Keep browser Camera state in `CameraSection` and route its calls through the shared authenticated Bench client. Stored snapshots remain artifact-owned; viewer zoom, pan and markers stay local.
D2: Validate administrator package and device settings against installed schemas, persist revisions, and reject active work atomically before changing a control baseline. Exact ID protects rebind/forget.
D3: Assemble target bundles from pinned wheels and an architecture-matched runtime manifest. The bench build can use host Python, while production must provide its own runtime root. The udev rule keeps nodes at mode 0660 for the video group; the package adds `nilo` to that group and the unit declares it.
Test seams: browser component tests, SQLite active-work regression, Camera package/probe tests, package builder inventory test, Pi live transaction and later reboot acceptance.

## Log

- 2026-09-25 frame: Full CAM-05 to CAM-08 physical-only scope accepted in autopilot from the user's implementation direction; unavailable HDMI/Ubuntu checks remain explicit later acceptance.
- 2026-09-25 specify/design: Wrote four browser, administration, package and device-access requirements and D1-D3 boundaries.
- 2026-09-25 build: Added Camera overview tiles, composition and encoding, local viewer controls, per-camera history, revisioned device settings form, active-work configuration refusal, public Camera terminal errors, packaged video-group rule and service membership, and optional interpreter-closed bundle construction. Browser Pi transaction `01a0d53c-470d-7580-b1b4-43cb357e34ca` completed as image/png at 320x240 with two overlays. Full GUI suite (513), Camera/probe suite (29), and focused Go/packaging tests passed.
- 2026-09-25 build red: command=`python -m unittest 05_Software/scripts/tests/test_release_packaging.py -k ReleaseEvidenceTests`; exit=1; expected missing behavior=candidate input contract did not accept locked Camera build inputs.
- 2026-09-25 build: Offline candidate construction now binds per-target Camera dependency/wheel archives through the input lock and builds self-contained Camera bundles from those inputs and the verified target runtime. Camera dependency notices are embedded. The Pi GUI now presents Camera text in English, German and Romanian, verified in Chrome at normal and 390-pixel widths.
- 2026-09-25 build deviation: Actual arm64/amd64 interpreter-closed package execution is deferred because the release input lock has no target runtime or Camera dependency archives and the Ubuntu mini PC is unavailable; packaging code tests do not substitute for that check.
- 2026-09-25 build proof: Full Go suite, 514 GUI tests, 29 Camera/probe tests, 48 release-packaging tests (5 skips), GUI lint/build and Camera Ruff passed. Pi service stayed active after GUI deployment; Chrome showed present/ready `cam-1`, saved physical PNG, two saved image comparisons and working language changes. The overview quick action produced physical JPEG transaction `01a0d558-5ce0-7002-87fa-fd9a1be7522f`, which completed after Ask approval. Pi `/dev/video0` and `/dev/video1` were `0660 root:video`; the development service still runs as `dpasare` in that group. AEGIS check reported 0 errors and 79 warnings, chiefly pre-existing repository/task coverage findings.
- 2026-09-25 build: Added local viewer rotation/flips/invert and mapped placed markers through view and requested capture coordinates. Focused geometry/UI tests moved from missing module to green; full GUI suite reached 517. On the Pi, marked physical JPEG transaction `01a0d55e-996a-7709-9805-d3ac7c8da554` completed after Ask approval; clearing the local marker left the red crosshair visible in the retained image.
- 2026-09-25 build: Added line, rectangle and calibrated scale-bar composition controls in three languages. Focused browser tests, GUI build and lint passed. Deployed GUI bundle `index-Bz4yS4_o.js` to the Pi; physical `/dev/video0` JPEG transaction `01a0d563-ab36-7b2d-ac85-6b9e0f76f5e6` completed, and Chrome rendered its line and rectangle in the retained frame. No Camera runtime simulation source or generated-frame data was found.
- 2026-09-25 build: Camera discovery now carries bounded V4L2 frame intervals and the browser offers only advertised choices for the selected format and size. Rebuilt core, GUI and indexed arm64 Camera package; applied the package via NILO's upgrade lifecycle after correcting runner execute mode in the catalog. Chrome completed physical JPEG transaction `01a0d571-84e6-726f-91e8-973af826cbd5` at requested 1/15 s and retained its effective interval and actual controls. Full Go suite, 519 GUI tests, 31 Camera/probe tests, GUI lint/build and Camera Ruff passed.
- 2026-09-25 build: Pi transaction `01a0d574-7cbb-78c6-8191-0c30fa36da86` completed with capture metadata exposed in the browser result. Upgraded the indexed Pi Camera package with bundled licensed DejaVu Sans; Chrome transaction `01a0d57a-0891-7cb8-8b11-1cbeb7192689` completed from the physical webcam and displayed `Șurub Ж` in the retained image. Camera/probe suite reached 32 tests.

## Rulings


## Handoff


current: build
status: ready
next: Review the integrated phase-one candidate; retain missing target input archives and hardware acceptance as explicit follow-up evidence.
must_read: .agentic/tasks/TASK-26a7f0d2.md; .agentic/steps/4_build.md; 00_Planning/Camera_Plugin_Plan.md; 06_SoftwareDocu/Camera_Integration.md
blockers: HDMI dongle and Ubuntu mini PC are unavailable for target acceptance; locked runtime and Camera build input archives are absent.

## Stage report

stage: build
result: in_progress
current: build
changed: Added physical Camera browser composition/administration in three languages, including line, rectangle and calibrated scale bar editing; offline release Camera bundle wiring and packaged video-group access.
evidence: Full Go suite, 519 GUI tests, 32 Camera/probe tests, 48 release packaging tests, GUI lint/build and Camera Ruff pass; Pi Chrome showed real PNG/JPEG snapshots, red marker, white line/rectangle and Unicode text overlays, two-up image comparison, three languages, a narrow layout and a completed 15 fps physical capture with result metadata; video nodes were 0660 root:video.
issues: Sigrok managed-device migration remains in CAM-02. Locked target runtimes and Camera build inputs are absent; interpreter closure and udev reboot acceptance remain unproved on both targets. HDMI and Ubuntu physical checks await hardware.
next: Complete remaining phase-one software gaps, review the integrated candidate and validate current evidence; retain target execution as an open acceptance check.
needs_user: no
spec_compliance:
implementation_quality:
review_round:
