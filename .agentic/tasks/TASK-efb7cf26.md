<!--
Copyright (c) 2026 DDP Engineering
This software is licensed under the PolyForm Noncommercial License 1.0.0.
Commercial use requires a separate commercial license.
See the LICENSE.txt in the .agentic folder for full license details.
SPDX-License-Identifier: LicenseRef-PolyForm-Noncommercial-1.0.0
-->


# TASK-efb7cf26 - Reconcile stale requirement paths and retire the removed debug session

stage: done
type: change
path: quick
interaction: autopilot
requirements: REQ-FUNC-35da79f1
parent:
branch: ddp/nilo-password-policy
base: 8e653ca8b56efcb8f2f8084cdeeee284c7def5f2
opened: 2026-09-16

## Done checklist (current stage)

- [x] validated candidate confirmed unchanged; otherwise returned to validate
- [x] validation evidence, diff, exclusions, and outcome presented
- [x] delivery confirmed by the human
- [x] journal and report included in the delivery commit
- [x] agreed outcome delivered, normally a PR
- [x] close-out committed after the outcome succeeded, or carried by the
      delivery commit where nothing could fail after it

## Frame

Problem:
`check` reported fourteen `stale-paths` warnings: requirement files naming
repository paths that no longer exist. Reading them showed two unrelated
problems wearing the same label. Eleven were documents consolidated by commit
127dc08. Three belonged to one requirement whose *code* was deleted by commit
5208351 while the requirement stayed `active`, so the register has been
promising a capability the appliance does not have.

Desired outcome:
Every requirement names paths that exist, and no active requirement describes
behaviour NILO no longer offers.

Actors: anyone reading the requirement register or following its paths to the
code and documents that implement a behaviour.

Primary scenario: trigger -> system action -> observable result
A reader follows a requirement's `affected_paths` -> every entry resolves to a
file in the tree -> they reach the document or source that carries the
behaviour, and a behaviour that was withdrawn reads as withdrawn.

In scope:
- Group A, ten requirement files: repoint the entries for three retired
  documents at the documents that absorbed them, or drop an entry that was
  orientation the requirement does not need.
- Group B: retire REQ-FUNC-35da79f1, the timed debug session.

Out of scope:
- Every other requirement's prose. Group A changes paths only.
- Restoring the debug session. Retiring records what happened; building it again
  would be its own task.
- The `specs/REQ-FUNC-2f8d63b5.md` spec file, kept beside the retired
  requirement as the record of what the behaviour was.

Constraints:
- OBSERVED `.agentic/AGENTS.md`: a retired requirement keeps its file with
  `status = "retired"` and a dated line in its prose; the retiring task lists
  the id. Deleting it would erase the evidence the behaviour existed.
- OBSERVED the same file: "a code/document conflict is a finding, not
  permission to rewrite the contract" - so Group B was reported and the user
  authorised the retirement before it was made.

Evidence:
- OBSERVED commit 127dc08 "docs: plan sigrok waveform workspace and refresh
  repository guides": deleted 00_Planning/Development_Simulation_Vertical_Slices.md,
  06_SoftwareDocu/Live_Network_Development.md and
  06_SoftwareDocu/Network_Service_Implementation_Status.md, added
  06_SoftwareDocu/Network_Operations.md (+148) and extended
  06_SoftwareDocu/Development_and_Simulation_Flow.md (+30).
- OBSERVED: Network_Operations.md mentions WireGuard seven times and
  diagnostics four; Development_and_Simulation_Flow.md mentions simulation
  fifty-one times and acceptance or hardware twenty-four. The replacements
  carry the material, rather than being the nearest surviving file.
- OBSERVED commit 5208351 "feat(activity): streamline history and retention":
  deleted cmd/nilo/diagnostics.go, internal/diagnostics/diagnostics.go, its
  test, and internal/store/sqlite/diagnostic_repository.go.
- OBSERVED the tree: no `internal/diagnostics`, no `diagnostics` command in
  cmd/nilo, and REQ-FUNC-35da79f1's linked test file does not exist.
- OBSERVED `.agentic/tasks/`: no journal records retiring that behaviour, so
  the removal happened without the contract being updated.
- STATED (2026-09-16): the user read both groups and asked for both to be fixed.

Assumptions: none.
Unknowns: none.
Decision-blocking questions: none. Group B was presented as a finding and the
user authorised it.

Proposed behaviors:
- B1. Ten requirement files name only paths that exist.
- B2. REQ-FUNC-35da79f1 is `retired`, its prose says when and why, and it
  carries no linked tests, because a withdrawn behaviour owes none.

Path and reason: quick.

| criterion | yes | no | evidence |
|---|---|---|---|
| no new externally observable behavior | yes | | nothing executable changes |
| no new interface or data format | yes | | none |
| no new dependency | yes | | none |
| an existing requirement already covers the behavior | yes | | REQ-FUNC-35da79f1 is the requirement being retired; Group A changes no behaviour |
| no security or data-handling behavior changes | yes | | paths and one status field |

Every criterion answers yes, so the quick path applies.

## Requirement coverage


## Design


## Log

- 2026-09-16 frame: quick path. Fourteen stale-paths findings resolved as two
  problems: eleven document references consolidated by 127dc08, and one
  requirement whose code was deleted by 5208351 without retiring it.
- 2026-09-16 build: Group A - ten requirement files repointed. The vertical
  slices plan maps to Development_and_Simulation_Flow.md; both network
  documents map to Network_Operations.md. REQ-PLAT-e1a2a591's entry was
  dropped rather than repointed: its subject is the locked browser build, and
  no surviving document describes that, so aiming it at a development-flow
  guide would trade a broken link for a misleading one. REQ-FUNC-bd813478
  named both network documents, which now fold into one entry.
- 2026-09-16 build: Group B - REQ-FUNC-35da79f1 retired, with a dated line in
  its prose naming commit 5208351 and what it removed, and its linked test
  cleared because the test file went with the code. The file and its spec are
  kept: they are the record that the behaviour existed and was withdrawn.
- 2026-09-16 build: `check` reports 0 stale-paths findings, from 14.
- 2026-09-16 validate: this candidate changes no executable file. `git status`
  outside `.agentic/` is empty, so the tree the last full run proved -
  TASK-a3dda009's `check --strict --run`, exit 0, test ok 379.2s, lint ok 8.0s -
  is byte-for-byte the tree delivered here. Per the validate step, a prose-only
  update takes semantic and traceability review rather than another runtime run,
  so the suite was not re-run and no fresh runtime evidence is claimed.
- 2026-09-16 validate: traceability reviewed. Every repointed entry resolves to
  a file that exists and that carries the material, checked by reading the two
  surviving documents rather than by assuming the nearest filename.
  `check` reports 0 stale-paths, down from 14, and errors stay 0. The retired
  requirement keeps its file and its spec; `check` no longer applies
  active-only rules to it, which is why its cleared `linked_tests` is not a
  finding.
- 2026-09-16 validate review round 1: specification compliance approved and
  implementation quality approved on the current candidate.
- 2026-09-16 deliver: the user asked for both groups to be fixed and for the
  tree to be staged; they make the commit. The outcome is that one
  repository-local commit, so close-out is carried by it. The agent staged only.

## Rulings


## Handoff


current: done
status: ready
next: none - the human makes the staged commit
must_read: .agentic/tasks/TASK-efb7cf26.md
blockers: none

## Stage report

stage: deliver
result: completed
current: done
changed: 11 requirement files - 10 repointed, REQ-FUNC-35da79f1 retired; .agentic/tasks/TASK-efb7cf26.md
evidence: python .agentic/aegis.py check - stale-paths 14 -> 0, errors 0; no executable file changed, so the last full run (TASK-a3dda009, check --strict --run exit 0) describes this tree unchanged
issues: none open
next: none - the human commits the staged tree
needs_user: no
spec_compliance: approved
implementation_quality: approved
review_round: 1


## Delivery report (2026-09-16)

- stage: done; type: change; path: quick; branch: ddp/nilo-password-policy
- requirements:
  - REQ-FUNC-35da79f1 - Run a timed debug session for one function and optional target
- deviations: none recorded
- explicit review: enabled; spec_compliance=approved; implementation_quality=approved; review_round=1
- diff vs task base 8e653ca: 12 file(s) changed, 227 line(s) added
- checks: errors=0 warnings=22 notes=3
- checklist: 6/6 done
