<!--
Copyright (c) 2026 DDP Engineering
This software is licensed under the PolyForm Noncommercial License 1.0.0.
Commercial use requires a separate commercial license.
See the LICENSE.txt in the .agentic folder for full license details.
SPDX-License-Identifier: LicenseRef-PolyForm-Noncommercial-1.0.0
-->


# TASK-d8c3f7bf - Plan Go to Python backend migration with FastAPI uv and Ruff

stage: frame
type: change
path: full
interaction: checkpoint
requirements:
parent:
branch: feature/38-add_ci
base: 52580d439c3dfc082aa529a5ce657ab837cd25c2
opened: 2026-09-28

## Done checklist (current stage)

- [x] Frame records scope, actors, scenario, examples, constraints and evidence.
- [x] Claims classified as stated, observed, proposed/inferred or unknown.
- [x] Unresolved implementation decisions retained in the proposed plan.
- [x] Full future implementation path selected with its reason.
- [x] Existing branch retained; header matches checkout; Git authority is read-only.
- [x] Journal created with complete header.
- [x] Plan maps migration outcomes to work packages and acceptance gates.
- [ ] Implementation scope accepted; current instruction authorizes planning only.
- [ ] Advance to specify only after an implementation instruction.

## Frame

Problem: Three language stacks and exposed tooling motivate a proposed Go-to-Python backend migration.
Desired outcome: A reviewable implementation plan saved in 00_Planning using Python/FastAPI, uv and Ruff.
Actors: Developer, Administrator, Operator, Viewer, GUI/CLI/MCP clients, Core, helper, Camera/Serial runners and release engineer.
Primary scenario: Developer implements bounded migration packages -> contract and recovery gates prove Python behavior -> accepted candidate replaces Go with a reproducible offline target runtime.
Examples: Existing React logs into the Python simulation core; Camera/Serial sessions retain policy and bounded supervision; a network change still rolls back after Core failure.
Counterexamples: FastAPI defaults silently alter errors; multiple ASGI workers duplicate schedulers; copying an amd64 virtualenv to Pi; claiming excluded Sigrok acceptance.
In scope: Proposed plan, planning-index link, frame record and document verification. Target proposal includes Core/CLI/helper, FastAPI, uv, Ruff, Camera/Serial parity and tooling/packaging adaptation.
Out of scope: Implementation, formal requirement creation, code/environment changes, release/version assignment, Gitea issue creation, Git mutations and deployment. Sigrok-specific work and GUI replacement excluded from migration proposal.
Constraints: Preserve covered product contracts; repo-local pinned inputs; offline exact-target packaging; Linux appliance and WSL simulation topology; no unauthorized Git mutation.
Success signals: Saved plan has explicit boundaries, uv/Ruff adoption, dependency/ASGI feasibility, bounded work packages, parity coverage, target gates and cutover/rollback decisions.
Evidence: STATED current user request and Sigrok exclusion; OBSERVED sources and existing Python pins listed in plan section 12; INFERRED architecture/work packages labeled proposed; UNKNOWN selections and thresholds listed in section 11.
Assumptions: FastAPI follows the accepted discussion; React retained; full Go execution surface includes helper; existing integration process isolation retained. Sigrok removal interpreted as removal from planning scope, not deletion of current files.
Unknowns: Exact new dependencies, TLS/admission implementation, workspace details, resource thresholds, hardware availability, real-data preservation and excluded catalogue disposition.
Decision-blocking questions: None for a proposed plan; PY-01/PY-02 must settle implementation scope and feasibility before migration.
Proposed behaviors: 00_Planning/Go_to_Python_Migration_Plan.md; no claim that Python Core currently exists.
Path and reason: Full future implementation: new framework/runtime dependencies and security/concurrency/persistence composition require requirements, design and complete validation. Planning ends at frame.

| Quick-path criterion | Yes | No | Evidence |
|---|---|---|---|
| No new externally observable behavior | | x | Parity intended, but migration/resource and exclusion behavior require proof. |
| No new interface or data format | x | | Preserve covered public contracts; internal Python/helper composition changes. |
| No new dependency | | x | FastAPI, ASGI and Python platform dependencies require selection. |
| Existing requirement covers the complete change | | x | No complete Python migration requirement identified. |
| No security or data-handling change | | x | Authentication, canonical approval digests, process/privilege ownership and persistence are ported. |

## Requirement coverage

Deferred to implementation specify stages. The plan's parity matrix and work packages establish discovery/verification obligations rather than new approved requirements.

## Design

Proposed architecture is in the plan; no implementation design approved by this planning request.

## Log

- 2026-09-28 frame: Opened planning-only task with full future implementation path; repository evidence and primary framework references inform the migration plan. Existing branch retained under Git authority rules.
- 2026-09-28 frame: User excluded Sigrok; plan scopes integration acceptance to Camera/Serial and records excluded catalogue disposition for implementation framing.
- 2026-09-28 frame deviation: Saved proposal and index under explicitly requested 00_Planning, overriding the ordinary frame write zone; no production implementation performed.
- 2026-09-28 frame verification: Completed plan review and document checks: all local links resolve, fences paired, eleven unique ordered work packages and index/exclusion checks pass. Planning-index whitespace check passes. Advisory task check reports zero errors, five pre-existing warnings and five notes (four existing manual checks and this frame task's missing anchor). No software suite needed for this planning-only output; unrelated Modbus changes preserved and Git state unchanged.

## Rulings

- RULING-001 | date=2026-09-28 | decision=Planning only, save output in 00_Planning | reason=Current explicit user request | source=user | supersedes=none
- RULING-002 | date=2026-09-28 | decision=Sigrok integration excluded from migration scope | reason=Current steering instruction during planning | source=user | supersedes=none

## Handoff


current: frame
status: waiting
next: Review the saved plan; accept PY-01 scope, revise the proposal or leave implementation paused.
must_read: 00_Planning/Go_to_Python_Migration_Plan.md; AGENTS.md; .agentic/steps/1_frame.md
blockers: None for plan delivery; implementation decisions remain proposed.

## Stage report

stage: frame
result: paused
current: frame
changed: Migration plan, planning index and frame journal.
evidence: Document links, fences, eleven work packages, index/exclusion and whitespace checks pass; advisory check has zero errors.
issues: Five pre-existing advisory warnings; implementation dependencies, thresholds and catalogue disposition remain proposed.
next: Review proposal; accept PY-01 scope, revise it or leave implementation paused.
needs_user: yes - only for future implementation; requested planning output is complete.
spec_compliance:
implementation_quality:
review_round:
