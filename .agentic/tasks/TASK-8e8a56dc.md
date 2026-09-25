<!--
Copyright (c) 2026 DDP Engineering
This software is licensed under the PolyForm Noncommercial License 1.0.0.
Commercial use requires a separate commercial license.
See the LICENSE.txt in the .agentic folder for full license details.
SPDX-License-Identifier: LicenseRef-PolyForm-Noncommercial-1.0.0
-->


# TASK-8e8a56dc - CAM-03 camera integration package

stage: build
type: feature
path: full
interaction: autopilot
requirements: REQ-FUNC-0141b9d4, REQ-DATA-8082c1dc, REQ-FUNC-ac306a12
parent:
branch: NILO_camera_plugin
base: b83555684d11c359c41676a962dc0f0860e1335d
opened: 2026-09-24

## Done checklist (current stage)

- [ ] Add failing package tests before each production behavior increment.
- [ ] Implement bounded V4L2 operations against the package contract.
- [ ] Keep requirement links, protocol fixtures and package documentation current.
- [ ] Run focused and full package contract suites before validation.

## Frame

Problem: No installable camera runner exists; the browser has only a simulated preview.
Desired outcome: One package implements phase-1 camera discovery, typed control operations and bounded retained snapshots.
Actors: Bench Core, operator, administrator and agent through the common job protocol.
Primary scenario: A configured camera is discovered; snapshot is submitted; the runner captures, renders and declares one verified image artifact.
Examples: Pi FaceCam 1000X has a capture node and a sibling metadata node; simulation has two deterministic cameras.
Counterexamples: A metadata node cannot be opened for capture; arbitrary device paths or unbounded dimensions cannot be accepted from a user.
In scope: CAM-03 runner, schemas, tests and package metadata, with simulation and provisional real backend.
Out of scope: Managed-device storage, UI, live preview and packaging system integration, owned by their respective work packages.
Constraints: Manifest and protocol v1, 8 MiB artifact ceiling, limited pixels, control read-back, bounded cancellation and private source metadata.
Success signals: Contract tests and deterministic simulation pass; available Pi camera exercises real capture and controls; other target acceptance remains deferred by user direction.
Evidence: STATED user requested full implementation and deferred missing HDMI/Ubuntu tests; OBSERVED `.agentic/evidence/camera/pi-webcam/evidence.json` shows both bindings capture; OBSERVED Sigrok package and Python SDK provide the protocol shape.
Assumptions: Use pyrav4l2 as the provisional real adapter because the Pi probe passed and its package declares Apache 2.0; production selection remains subject to remaining target evidence and closure review.
Unknowns: HDMI capture behavior and Ubuntu target behavior; exact unsupported controls on other devices.
Decision-blocking questions: None for host implementation under the user's direction.
Proposed behaviors: Discover physical cameras and typed controls; capture one bounded frame; render transforms and overlays; declare one digest-verified image; support deterministic simulation and control resets.
Path and reason: Full path, because this adds a package, dependencies and public operations. Existing branch retained under repository Git authority; no branch change was requested.

## Requirement coverage

| Frame outcome | Atomic behavior | Requirement | Positive case | Boundary or failure | Verification | Design decision |
|---|---|---|---|---|---|---|
| Camera operations | Discover and accept typed bounded operations | REQ-FUNC-0141b9d4 | two cameras and snapshot/control flow | invalid endpoint, control or dimensions | package tests and Pi probe | D1-D3 |
| Image evidence | Render and declare one validated image | REQ-DATA-8082c1dc | deterministic JPEG digest | oversized or mislabelled artifact | package and core tests | D4 |
| Configured behavior | Runner applies delivered plugin and device settings | REQ-FUNC-ac306a12 | maximum width and baseline control | invalid settings refused | package configure test | D5 |


## Design

D1 (REQ-FUNC-0141b9d4): Keep the protocol adapter in `src/nilo_camera` and the input/result contracts in package schemas. The core supplies an internal selected endpoint and staging directory; the public request supplies only a managed device id and bounded options.

D2 (REQ-FUNC-0141b9d4): Simulation models two physical cameras with stable capabilities, persistent per-device control state and seeded deterministic image frames. The provisional real backend uses the Pi-observed pyrav4l2 API and validates selected nodes against grouped sysfs and V4L2 capabilities before opening.

D3 (REQ-FUNC-0141b9d4): Control descriptors drive integer/Boolean/menu validation; automatic controls precede manual dependents. Re-read changed values and restore configured or factory baseline on explicit reset.

D4 (REQ-DATA-8082c1dc): Pillow renders in the specified order, strips source metadata by drawing into a new image, caps decoded pixels and output bytes, and declares one JPEG/PNG artifact with SHA-256 and effective parameters.

D5 (REQ-FUNC-ac306a12): The runner validates the versioned configure payload before hardware use, stores plugin limits and one targeted device baseline, then initializes that device and reports its configured revision in snapshot results.

Test seams: package simulation tests for deterministic bytes and boundaries, worker protocol tests, Pi probe for real backend behavior. Remaining target hardware proof stays in CAM-00/CAM-09.

## Scope update

The user directed on 2026-09-24 that the Camera plugin have no simulated code or data. Physical V4L2 devices are the only runtime camera source. Tests may isolate V4L2 calls with local doubles; no camera simulation backend, inventory, preview route or generated camera workspace remains.


## Log

- 2026-09-24 frame: Full path accepted in autopilot for CAM-03 package against provisional target envelopes. User directed implementation before unavailable HDMI and Ubuntu checks.
- 2026-09-24 specify/design: Reused existing camera capability and image evidence requirements and specs; D1-D4 assign protocol, backend, controls and artifact responsibilities. Production binding selection remains conditional on remaining hardware evidence.
- 2026-09-24 build red: REQ-FUNC-0141b9d4; command=`python -m pytest 05_Software/integrations/camera/tests/test_package.py -q`; exit=1; expected missing behavior=there is no camera package manifest declaring the five phase-1 operations and image artifact.
- 2026-09-24 build green: REQ-FUNC-0141b9d4; package manifest test exits 0. The camera package now declares five phase-1 operations, image evidence and bounded configuration/input schema paths; runner behavior remains to be built.
- 2026-09-24 build red: REQ-FUNC-0141b9d4; command=`uv run --no-sync --project 05_Software/integrations/camera python -m pytest 05_Software/integrations/camera/tests/test_plugin.py -q`; exit=1; expected missing behavior=the camera runner package does not exist, so two simulated physical cameras cannot be discovered.
- 2026-09-24 build green: REQ-FUNC-0141b9d4; focused simulation discovery test exits 0. The package now exposes two physical camera records with distinct capture and metadata endpoints through the SDK operation handler.
- 2026-09-24 build red: REQ-DATA-8082c1dc; command=`uv run --no-sync --project 05_Software/integrations/camera python -m pytest 05_Software/integrations/camera/tests/test_plugin.py::test_simulated_snapshot_is_bounded_deterministic_evidence -q`; exit=1; expected missing behavior=the camera runner rejects snapshot instead of retaining bounded deterministic image evidence.
- 2026-09-24 build green: REQ-DATA-8082c1dc; focused image test exits 0. Simulated snapshots now write bounded deterministic JPEG/PNG bytes, strip metadata by encoding a fresh image, and declare matching SHA-256, media type and size.
- 2026-09-24 build red: REQ-FUNC-0141b9d4; command=`uv run --no-sync --project 05_Software/integrations/camera python -m pytest 05_Software/integrations/camera/tests/test_plugin.py::test_controls_persist_through_snapshot_until_explicit_reset -q`; exit=1; expected missing behavior=typed control mutations and reset are rejected as unknown operations, and snapshot overrides do not persist.
- 2026-09-24 build green: REQ-FUNC-0141b9d4; focused control persistence and bounds test exits 0. Simulation now exposes typed descriptors, atomically validates changes, retains snapshot overrides as current state and resets to baseline or factory defaults.
- 2026-09-24 build red: REQ-FUNC-0141b9d4; command=`uv run --no-sync --project 05_Software/integrations/camera python -m pytest 05_Software/integrations/camera/tests/test_package.py::test_camera_runner_answers_protocol_hello_and_describe -q`; exit=1; expected missing behavior=the camera package has no executable protocol entry point for supervisor hello and describe.
- 2026-09-24 build green: REQ-FUNC-0141b9d4; focused process-level protocol test exits 0. The package now answers correlated hello and describe requests through the SDK and exposes only the five declared operations.
- 2026-09-24 build red: REQ-FUNC-0141b9d4; command=`uv run --no-sync --project 05_Software/integrations/camera python -m pytest 05_Software/integrations/camera/tests/test_v4l2.py -q`; exit=1; expected missing behavior=there is no real backend grouping physical USB nodes and excluding metadata-only endpoints.
- 2026-09-24 build green: REQ-FUNC-0141b9d4; focused grouping test exits 0. On the Pi, the new production sysfs/V4L2 scanner returned one FaceCam 1000X physical record with `/dev/video0` as capture and `/dev/video1` excluded as metadata. This is discovery only; real snapshot and controls remain open.
- 2026-09-24 build red: REQ-DATA-8082c1dc; command=`uv run --no-sync --project 05_Software/integrations/camera python -m pytest 05_Software/integrations/camera/tests/test_v4l2.py::test_decode_frame_rejects_oversize_and_strips_source_metadata -q`; exit=1; expected missing behavior=the real backend lacks bounded MJPEG/YUYV decoding that strips source metadata.
- 2026-09-24 build green: REQ-DATA-8082c1dc; focused decoder bounds/metadata test exits 0. MJPEG and YUYV now decode within input-byte and decoded-pixel limits into fresh RGB images without source EXIF.
- 2026-09-24 build red: REQ-FUNC-0141b9d4; command=`uv run --no-sync --project 05_Software/integrations/camera python -m pytest 05_Software/integrations/camera/tests/test_v4l2.py::test_capture_selects_advertised_format_and_discards_warmup -q`; exit=1; expected missing behavior=the backend cannot select an advertised format/size and discard warm-up frames before snapshot decode.
- 2026-09-24 build green: REQ-FUNC-0141b9d4; focused format/warm-up test exits 0. On the Pi the production backend selected FaceCam `/dev/video0` MJPEG 640x480, discarded two frames, decoded a clean image and re-encoded a 5,427-byte JPEG with SHA-256 `ae5a70715156c9032f796083ea6047903b9d6fcd5bb33c98b03e72a20c8edb3e`. This is backend capture proof, not yet a retained NILO job.
- 2026-09-24 build red: REQ-DATA-8082c1dc; command=`uv run --no-sync --project 05_Software/integrations/camera python -m pytest 05_Software/integrations/camera/tests/test_plugin.py::test_real_snapshot_uses_only_discovered_internal_endpoint -q`; exit=1; expected missing behavior=the runner refuses real snapshot despite a verified internally discovered capture endpoint.
- 2026-09-24 build green: REQ-DATA-8082c1dc; focused real snapshot handler test exits 0. The runner now re-discovers the USB camera, chooses only its internal capture endpoint, captures and declares one image artifact; requests cannot supply a node path.
- 2026-09-24 build Pi observation: With isolated Python dependencies, the package handler discovered the FaceCam as `cam-1` at capture endpoint `video0` and completed a real 640x480 snapshot. Its staged JPEG was 5,427 bytes and the declared digest exactly matched `ae5a70715156c9032f796083ea6047903b9d6fcd5bb33c98b03e72a20c8edb3e`; the temporary image was removed. This does not yet prove core job retention, control read-back, or packaged service permissions.
- 2026-09-24 build verification: All nine camera package tests and Ruff check exited 0 after the first real snapshot path.
- 2026-09-24 build red: REQ-FUNC-ac306a12; command=`uv run --no-sync --project 05_Software/integrations/camera python -m pytest 05_Software/integrations/camera/tests/test_plugin.py::test_configured_limit_and_control_baseline_apply_before_work -q`; exit=1; expected missing behavior=the runner acknowledges configure but ignores the camera limit and device control baseline.
- 2026-09-24 build green: REQ-FUNC-ac306a12; focused configure test exits 0. A versioned configure payload now constrains snapshot dimensions and initializes the target device's control baseline before work, with revisions retained in results.

- 2026-09-24 build deviation: Removed the camera simulation backend, generated frames and simulated inventory at the user's explicit direction. Retired REQ-FUNC-09c18b8e and REQ-UX-09873862; package tests now isolate real V4L2 calls and physical acceptance uses the Pi camera.
- 2026-09-24 build: Indexed the physical Camera bundle for Pi arm64 and installed it on the appliance. The attached serial-less USB camera completed a retained snapshot through the NILO job path. Camera and probe tests passed (27), Ruff passed, and the Pi camera remained ready after service restart.
- 2026-09-25 build: Added advertised V4L2 frame intervals to discovery and optional exact interval selection before streaming. A Pi probe enumerated 80 intervals on `/dev/video0`, excluded metadata node `/dev/video1`, and captured a real 640x480 MJPEG frame at 1/15 s. Indexed package upgrade then completed NILO transaction `01a0d571-84e6-726f-91e8-973af826cbd5` with the effective interval in its result. Camera/probe suite reached 31 tests and Ruff passed.
- 2026-09-25 build: Completed result metadata for capture time, selected endpoint, warm-up discard, requested/effective interval, controls, size and configuration revisions; Pi transaction `01a0d574-7cbb-78c6-8191-0c30fa36da86` returned it. Bundled licensed DejaVu Sans for text and scale labels; physical Pi transaction `01a0d57a-0891-7cb8-8b11-1cbeb7192689` retained a 15 fps JPEG showing `Șurub Ж`. Camera/probe suite reached 32 tests and Ruff passed. The Camera runtime and bundle contain no generated-frame, replay or Camera simulation source.

## Rulings

- RULING-001 | date=2026-09-24 | decision=The Camera plugin reads physical V4L2 hardware only; host checks use isolated V4L2 test doubles and live acceptance uses attached devices. | reason=Explicit user direction supersedes design D2's simulation clause and the camera simulation requirement pair. | source=user | supersedes=none


## Handoff


current: build
status: ready
next: Review the physical-only package and complete target closure checks when input archives and hardware are available.
must_read: .agentic/tasks/TASK-8e8a56dc.md; .agentic/steps/4_build.md; .agentic/specs/REQ-FUNC-0141b9d4.md; .agentic/specs/REQ-DATA-8082c1dc.md
blockers: None for host implementation; HDMI and Ubuntu acceptance remains deferred.

## Stage report

stage: build
result: in_progress
current: build
changed: Built the physical-only V4L2 Camera package, image processing, controls, runner and an indexed arm64 bundle.
evidence: 32 Camera/probe tests and Ruff passed; Pi camera discovery, requested 15 fps capture, reset, snapshot retention, authenticated preview, result metadata and bundled-font Unicode overlay completed through NILO.
issues: Interpreter-closed amd64/arm64 package execution and unavailable HDMI/mini-PC acceptance are deferred.
next: Review the package and retain unavailable target acceptance as open evidence.
needs_user: no
spec_compliance:
implementation_quality:
review_round:
