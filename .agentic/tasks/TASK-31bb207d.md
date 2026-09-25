<!--
Copyright (c) 2026 DDP Engineering
This software is licensed under the PolyForm Noncommercial License 1.0.0.
Commercial use requires a separate commercial license.
See the LICENSE.txt in the .agentic folder for full license details.
SPDX-License-Identifier: LicenseRef-PolyForm-Noncommercial-1.0.0
-->


# TASK-31bb207d - CAM-04 camera adapter and submission

stage: build
type: feature
path: full
interaction: autopilot
requirements: REQ-FUNC-3d711f4f, REQ-FUNC-a9657141, REQ-SEC-09df2645
parent:
branch: NILO_camera_plugin
base: b83555684d11c359c41676a962dc0f0860e1335d
opened: 2026-09-24

## Done checklist (current stage)

- [ ] Write failing adapter, submission and interface tests before changes.
- [ ] Register camera package and route discovery/submission/artifacts through managed records.
- [ ] Add CLI and MCP camera operations with bounded image results.
- [ ] Validate Sigrok regression and camera simulation and Pi workflows.

## Frame

Problem: Bench Core lacks a camera family adapter, so the camera package cannot be installed or used through jobs.
Desired outcome: Installed camera package uses the same queue, leases, approval, artifact and CLI/MCP interfaces as Sigrok.
Actors: Operator, administrator and agent.
Primary scenario: Discover a managed camera, submit snapshot, retain one image, then return a bounded result or pending id.
Examples: Pi FaceCam real snapshot completed in the package; simulated camera has deterministic image bytes.
Counterexamples: Public node paths, unknown device ids, unauthorized artifact reads and undeclared media are refused.
In scope: CAM-04 adapter, generic submission, core reset orchestration, CLI and MCP camera operations.
Out of scope: Core record storage, camera package implementation and browser UI, owned by CAM-02/03/06.
Constraints: Manifest authority, existing approval model, 8 MiB artifact ceiling, owned image reads and typed terminal errors.
Success signals: Package passes manifest agreement; simulated and real snapshots survive through jobs and authorized artifact reads; agent receives bounded image or durable pending id.
Evidence: STATED full camera-plan request; OBSERVED adapter registry and camera package manifest; OBSERVED Pi package snapshot.
Assumptions: CAM-02 records are ready before user submission is enabled; provisional hardware identity test is available on Pi.
Unknowns: HDMI/Ubuntu behavior and phase-2 preview arbitration.
Decision-blocking questions: None for host implementation.
Proposed behaviors: Register camera adapter, validate typed requests, inject stored device endpoint, expose images through CLI/MCP.
Path and reason: Full path for new public operations and security boundary; current branch retained per Git authority.

## Requirement coverage

| Frame outcome | Atomic behavior | Requirement | Positive | Boundary | Verification | Decision |
|---|---|---|---|---|---|---|
| Package contract | Adapter agrees with camera manifest | REQ-FUNC-3d711f4f | matching package | mismatch refused | adapter test | D1 |
| Safe endpoint | Resolve stored managed endpoint | REQ-SEC-09df2645 | camera id | path rejected | job tests | D2 |
| Agent image | Return bounded completed or pending result | REQ-FUNC-a9657141 | image | Ask pending | MCP/CLI tests | D3 |


## Design

D1: Register `camera` with operations, image artifact, canonicalizer, resource, runner arguments and staged filename. Manifest declarations remain authoritative and are checked at package load.

D2: Camera submissions carry a managed record id and typed options. The adapter verifies configured/present/ready state, takes its device resource lease and injects the internally retained endpoint into the runner request after admission.

D3: CLI and MCP wait only a bounded interval. An Ask or busy job returns the transaction id; completed jobs read a bounded preview of the owner-authorized image artifact.

Test seams: adapter agreement, bench job tests, CLI tests, MCP tests and Pi live job.


## Log

- 2026-09-24 frame: Full CAM-04 path accepted in autopilot under user's implementation direction; missing HDMI/Ubuntu acceptance remains later work.
- 2026-09-24 specify/design: Reused three existing adapter, image-result and endpoint-safety requirements; D1-D3 assign the package contract, trust boundary and wait behavior.

- 2026-09-24 build deviation: Removed camera runner simulation arguments and catalog data at the user's explicit direction; the Camera adapter now launches a physical-only runner.
- 2026-09-24 build: Added Camera adapter, startup and reconnect baseline resets, typed control and snapshot jobs, bounded previews, generic CLI submission, Camera CLI commands, MCP Camera image/pending tools and browser navigation. Full Go suite, 511 GUI tests, GUI lint and 27 Camera/probe tests passed; Pi package produced retained physical snapshots.

## Rulings


## Handoff


current: build
status: ready
next: Complete remaining CLI/MCP edge cases and browser workspace controls, then validate integrated phase-one contracts.
must_read: .agentic/tasks/TASK-31bb207d.md; .agentic/steps/4_build.md; .agentic/specs/REQ-FUNC-3d711f4f.md; .agentic/specs/REQ-FUNC-a9657141.md
blockers: None for host work; CAM-02 and CAM-03 are in progress.

## Stage report

stage: build
result: in_progress
current: build
changed: Added Camera adapter, physical discovery, ready-state admission, startup reset, image artifact/preview, CLI/MCP tools and browser entry points.
evidence: Full Go suite and 519 GUI tests passed; Pi Camera plugin installed, ready after restart, and completed retained snapshots through the browser with controls, composition, interval selection and result metadata.
issues: CLI/MCP administrative edge cases and unavailable target hardware acceptance remain open; browser overview, comparison and composition are implemented.
next: Review CLI/MCP edge cases, then validate against the plan.
needs_user: no
spec_compliance:
implementation_quality:
review_round:
