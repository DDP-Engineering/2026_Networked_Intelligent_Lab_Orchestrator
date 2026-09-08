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
The Log is a progress-and-evidence ledger, not an activity transcript. Append
only durable events that change the task's stage, scope, engineering outcome,
verification status, authority, or ability to proceed: stage transitions;
material scope or implementation decisions not already captured elsewhere;
consolidated automated or manual verification results; material discoveries;
blockers; approvals; deviations; and delivery outcomes.

Do not log routine searches, reads, edits, internal tool mechanics or corrected
command retries, and do not copy requirements, designs, specifications,
knowledge pages, Rulings, Handoff, or Stage report content into the Log. A
failed command belongs only when it exposes a candidate problem, changes the
implementation or tests, or remains an accepted verification limitation. Keep
each entry concise and self-contained; reference durable artifacts instead of
repeating them. Existing entries remain append-only under later contracts.

Stage report is one replaceable latest snapshot, not an evidence store.
Use plain one-line fields with no fences or bullets inside that section.
stage is the reported stage; current must match the journal header after any
transition. result is in_progress | completed | paused | blocked | skipped.
Leave unknown fields blank; summary prints not recorded, never an assumed pass.
Refresh at a stage transition, pause or session end, blocker, or completed
validation or delivery outcome. Do not refresh after routine work.
-->

# TASK-df6a2395 — Harden structured Sigrok worker validation

stage: build
type: fix
path: quick
requirements: REQ-REL-c2695283, REQ-FUNC-cf21e27d
parent:
branch: master
base: 8d326898aec25de98052c59b5b2d67981f014d26
opened: 2026-09-05

## Done checklist (current stage)

- [ ] each automated behavior increment has a pre-implementation red record with requirement IDs, exact selector or command, non-zero exit, and expected failure reason, plus one final focused green result; each manual increment has a consolidated post-implementation observation
- [ ] repository anchor recorded after red and evolution reviewed after implementation
- [ ] implementation makes those tests pass
- [ ] `test --impacted` green
- [ ] `affected_paths` and `linked_tests` updated on every requirement
- [ ] `status = "active"` on every requirement this task delivers — or `"retired"`, with the reason in its prose, where this task deliberately removed the behavior
- [ ] `check` clean, or each remaining warning explained in the log
- [ ] `stage: validate`

## Frame

Problem: The feasibility adapter accepts records with only a valid schema/kind and the decoder verification claims two native failure cases that are not exercised.
Desired outcome: Every `nilo.sigrok.worker/1` kind has enforced fields, types, enums and ranges, while fixed native fixtures prove unknown-decoder and invalid-channel-mapping failures.
Actors: The isolated native Sigrok worker, its Python containment adapter, maintainers validating SW-01, and later SW-02 callers.
Primary scenario: A child emits a complete worker stream -> the adapter validates every record against its kind contract -> a malformed field or terminal structure is rejected before data can be trusted.
Examples: A logic packet requires positive unit size, consistent decimal counts, valid lowercase hex and a truncation flag; fixed diagnostic commands return structured `decoder_not_found` and `decoder_channel_mapping` errors from the deployed decoder runtime.
Counterexamples: Accepting `{schema, kind: packet}` or an empty summary, adding arbitrary decoder/config passthrough, or beginning SW-02 does not satisfy this correction.
In scope: Kind-specific Python validation; terminal success/error stream rules; focused negative tests; two fixed native decoder error fixtures and retained evidence; requirement/spec/document/knowledge reconciliation.
Out of scope: Public manifest/API/GUI changes; generic decoder selection or channel mapping; SW-02; physical/electrical qualification; version assignment, packaging, release or push.
Constraints: Stay on `master`; create exactly one additional local commit after validation; preserve unrelated files; no push; keep version 0.2.0; retain bounded fixed-command feasibility grammar.
Success signals: All retained success evidence validates; malformed required fields/types/enums/ranges/unknown keys and bad terminal structures fail deterministically; target-native fixed failure fixtures emit the expected structured errors and nonzero exits.
Evidence: STATED — independent review supplied both gaps and bounded delivery constraints. OBSERVED — `structured_worker.py` currently validates only schema/kind/aggregate/terminal-summary shape; `test_worker.py` lacks kind-field negatives; REQ-FUNC-cf21e27d names unknown-decoder and invalid-mapping verification; matching libsigrokdecode source rejects unknown IDs and invalid mapping keys. INFERRED — fixed diagnostic fixture commands preserve the feasibility-only grammar without exposing arbitrary decoder or mapping input. UNKNOWN — exact deployed stderr wording is irrelevant because stable worker error codes will be asserted.
Assumptions: Fixed diagnostic commands are internal feasibility fixtures, not public operations; error-only streams may be validated structurally while `StructuredWorker.run` still raises on the nonzero child exit.
Unknowns: None that changes scope or observable public behavior.
Decision-blocking questions: None; the correction request explicitly prefers native failure coverage and fixes delivery authority.
Proposed behaviors: Define closed per-kind schemas and semantic validators; require exactly one terminal summary for success or one terminal error for direct failure evidence; add fixed `missing-decoder-fixture` and `invalid-uart-mapping-fixture` commands; retain and test their machine-readable error records.
Path and reason: Quick path. No public observable behavior, public interface/data format, dependency, security policy or data handling changes; existing REQ-REL-c2695283 and REQ-FUNC-cf21e27d already require strict containment and decoder failure verification. The exact internal protocol becomes correctly enforced. Work is tightly coupled and needs no child task.

## Requirement coverage

| Frame outcome | Atomic behavior | Requirement | Positive case | Boundary or failure | Verification | Design decision or no impact |
|---|---|---|---|---|---|---|
| Strict record admission | Adapter enforces the complete closed schema and semantic constraints for every kind and terminal stream | REQ-REL-c2695283 | Retained Demo/UART streams | Missing/wrong/unknown fields, invalid enum/range/count/hex and terminal structure | Parameterized Python negatives and retained evidence | Existing feasibility protocol, no public contract |
| Decoder failure proof | Fixed native commands emit structured errors for missing decoder and invalid UART channel mapping | REQ-FUNC-cf21e27d | Existing UART success fixture | Unknown decoder and invalid mapping | Target run, retained error evidence and tests | Fixed diagnostics only; no arbitrary passthrough |

## Design

Quick path: the existing worker-boundary spec already defines the kinds, fixed grammar, error records and decoder failure acceptance; this correction makes implementation and proof match it without a new design.

## Log

- 2026-09-05 frame: Scope accepted from the explicit correction request; quick path selected because two existing SW-01 requirements already cover the internal validation and decoder-failure behavior, with public interfaces and SW-02 excluded.
- 2026-09-05 build paused: No product test or implementation edit was started; the user made an accepted, repository-managed build environment a prerequisite to further Sigrok work, so this correction waits for that separate environment plan and implementation.

## Rulings

<!-- Append decisions here; never edit history. Supersede with a new ruling.
- RULING-001 | date=YYYY-MM-DD | decision=<decision> | reason=<reason> | source=user | supersedes=none
source: user | repository | requirement | technical
-->
- RULING-001 | date=2026-09-05 | decision=Remain on master, preserve unrelated files, create exactly one additional validated local commit, and never push | reason=Explicit delegated delivery authority | source=user | supersedes=none

## Handoff

<!-- Replace these fields with the latest minimal resume snapshot.
status: ready | waiting | blocked
-->

current: build
status: waiting
next: Resume only after the development/build environment plan has been accepted and its required environment gates implemented.
must_read: 00_Planning/Development_Build_Environment_Plan.md; .agentic/steps/4_build.md; .agentic/tasks/TASK-df6a2395.md
blockers: Build environment definition, WSL2 OCI workflow and archive policy are prerequisite work by user decision.

## Stage report

stage: build
result: paused
current: build
changed: The correction remains framed but unstarted; environment planning is now the prerequisite activity.
evidence: Current SW-01 build script relies on ambient `cc`; repository policy and system plan require repository-managed, recoverable tooling.
issues: Correction intentionally waits for implementation and acceptance of the development/build environment gates.
next: Complete and approve the environment plan, then implement ENV-01 through ENV-05 before resuming this task.
needs_user: yes — ENV-01 must settle the five choices listed in the environment plan before implementation.
spec_compliance:
implementation_quality:
review_round:
