<!--
Copyright (c) 2026 DDP Engineering
This software is licensed under the PolyForm Noncommercial License 1.0.0.
Commercial use requires a separate commercial license.
See the LICENSE.txt in the .agentic folder for full license details.
SPDX-License-Identifier: LicenseRef-PolyForm-Noncommercial-1.0.0
-->


# TASK-bbeeac49 - Document first-boot provisioning and commissioning

stage: deliver
type: change
path: quick
interaction: autopilot
requirements: REQ-FUNC-7ae21313, REQ-PLAT-18084875, REQ-DATA-72876d18, REQ-REL-d9bfd8ae, REQ-FUNC-243c38a6, REQ-SEC-a1012bc5, REQ-PLAT-b1abf074, REQ-FUNC-9a19ad36, REQ-FUNC-554b9bbd, REQ-SEC-83deb5c6, REQ-FUNC-b49d7866, REQ-REL-fa990acb, REQ-UX-f9c2846a, REQ-UX-5a75d710
parent:
branch: ddp/nilo-first-boot-provisioning
base: 5ef872e1bbab6df6264b5f1cb1ec007f96c4eb77
opened: 2026-09-12

## Done checklist (current stage)

- [x] validated candidate confirmed unchanged; otherwise returned to validate
- [ ] validation evidence, diff, exclusions, and outcome presented
- [ ] delivery confirmed by the human
- [ ] journal and report included in the delivery commit
- [ ] agreed outcome delivered, normally a PR
- [ ] close-out committed after the outcome succeeded, or carried by the
      delivery commit where nothing could fail after it

## Frame

Problem: first-boot provisioning is built and proven across six work packages,
and none of it is written down. The operations guide still describes an
appliance that must be told its interfaces at a console, the CLI reference does
not mention the mode that needs no listener option, and issue #6 asks for a
delivery record that states honestly what was verified and what was not.

Desired outcome: an operator can read how first boot behaves, why an upgrade
never re-enters it, how discovery and direct commissioning work, and what
completing commissioning does and refuses; and a reviewer can read one report
that separates automated evidence from what is still owed on hardware.

Actors: an operator or installer reading the operations guide; a reviewer
reading the delivery report; a developer reading the CLI contract.

Primary scenario: the documentation states the current behavior positively ->
the delivery report names the automated evidence, the accepted pre-existing
failures and the hardware acceptance still owed.

In scope: `06_SoftwareDocu/Network_Operations.md`,
`06_SoftwareDocu/Development_and_Simulation_Flow.md`,
`05_Software/nilo_assistant/README.md`, `CHANGELOG.md`, and a new
`07_Delivery/NILO_First_Boot_Provisioning_Verification.md`.

Out of scope: every product file. This task changes no behavior, adds no
requirement and writes no test.

Constraints: the current-state documentation rule - these pages describe current
responsibilities and observable behavior directly and positively, with proposals
and history left in task records and Git. The delivery report must not claim
hardware acceptance that has not happened.

Success signals: each of the fourteen requirements this feature delivered is
described by prose that matches it; the delivery report's evidence table maps
every acceptance test to the evidence that actually exists; the accepted
pre-existing failures and their effect on CI are stated plainly.

Evidence:
- OBSERVED `06_SoftwareDocu/Network_Operations.md` - described only the
  administrator-chosen allowlist and the console options.
- OBSERVED `05_Software/nilo_assistant/README.md:18-20` - listed three `serve`
  forms and not the one that needs no listener option.
- OBSERVED `07_Delivery/NILO_Dual_Path_Management_Verification.md` - the shape a
  delivery report for this repository takes.
- STATED (user, issue #6, Documentation) - update architecture, commissioning,
  network operation, deployment, security, recovery and user-operation
  documentation with the temporary first-boot exposure model, eligible-interface
  rules, mDNS and link-local behavior, permanent-role workflow, migration,
  recovery, TLS expectations and acceptance evidence.

Assumptions: none.

Unknowns: none.

Decision-blocking questions: none.

Proposed behaviors: none. This task adds no product behavior.

Path and reason: quick.

| criterion | yes | no | evidence |
|---|---|---|---|
| no new externally observable behavior | yes | | no product file is touched |
| no new interface or data format | yes | | documentation only |
| no new dependency | yes | | none |
| an existing requirement already covers the behavior | yes | | the fourteen requirements delivered by work packages 1-7 are the contract this prose describes |
| no security- or data-handling behavior changes | yes | | the prose states the existing contract and changes none of it |

## Requirement coverage

No new requirement. The fourteen requirements listed in the header were re-read
against the prose written here; this task adds no behavior of its own and so
maps to no new atomic behavior row.


## Design

Skipped, and this is the record of that judgement. The work is documentation
with no new interface, data format, dependency or behavior, so there is no
implementation path to constrain. The structure follows the pages already in the
repository: a new section in the operations guide beside the existing runtime
and recovery material, and a delivery report in the shape of
`NILO_Dual_Path_Management_Verification.md`.

## Log

- 2026-09-12 frame: scope drafted for the documentation and delivery record of
  issue #6 (work package 8 of the agreed plan), autopilot under the user's
  standing instruction. Quick path: all five criteria hold - no product file is
  touched, and the fourteen requirements delivered by work packages 1-7 are the
  contract this prose describes.
- 2026-09-12 build: documentation only, so no test was written and none was
  invented. The fourteen requirements on the header line were re-read in
  coverage-table order and each was confirmed to be described by the prose:
  eligibility and the physical-adapter observation in the eligible-interface
  paragraphs, the lifecycle seeding and its refusal in the state paragraphs, the
  derived allowlist and the console entry point in the provisioning paragraphs,
  link-local in the addressing paragraph, the advertisement and helper ownership
  in the discovery paragraphs, the transition and its ordering in the
  commissioning paragraphs, and the browser workflow in the delivery report's
  contract list.
- 2026-09-12 build: changed `06_SoftwareDocu/Network_Operations.md` (a new
  first-boot section, a pointer from the runtime section, and a Mini PC entry in
  the carried-forward acceptance list),
  `06_SoftwareDocu/Development_and_Simulation_Flow.md` (the third `serve` mode
  and why simulation never reaches it), `05_Software/nilo_assistant/README.md`
  (the fourth `serve` form), `CHANGELOG.md`, and added
  `07_Delivery/NILO_First_Boot_Provisioning_Verification.md`.
- 2026-09-12 build: the delivery report states the four accepted pre-existing
  failures, that CI running the strict check will fail this branch until they are
  addressed, and that the same is true of `master` today. It claims no hardware
  acceptance and lists the Mini PC scenarios still owed, together with the three
  open decisions carried forward.

- 2026-09-12 build: DEVIATION build: the anchor was recorded after the
  documentation was written rather than before the first edit, which is what the
  build stage asks for work that has no automated red test. The consequence is
  that `TASK-bbeeac49.evolution.json` reports zero changed files and describes
  nothing. The candidate is the five files named in the preceding log entry -
  four modified and one added - and the stage report and delivery presentation
  state that rather than the evolution count. Nothing about the candidate itself
  is affected.

- 2026-09-12 validate: review round 1, both verdicts approved on the current
  candidate. Specification compliance: the prose describes the fourteen
  requirements this feature delivered and adds no behavior of its own; the pages
  state current responsibilities positively, with proposals and history left in
  task records; and the delivery report claims no hardware acceptance.
  Implementation quality: the report's evidence table maps two acceptance rows to
  something other than a passing test - acceptance 4 to the tests that exist plus
  the open deployment decision, and acceptance 10 to the unchanged role
  coordinator - rather than presenting either as proven.
- 2026-09-12 validate: project context reconciled.
  `.agentic/context/project.md` gains first-boot commissioning as a product
  responsibility. Component knowledge needed no further change: the Bench Core
  and GUI pages were reconciled by the tasks that built each behavior.
- 2026-09-12 validate: final gate run.
  command=`.\02_Tools\env\with-tools.ps1 python .agentic/aegis.py check --task TASK-bbeeac49 --strict --run`;
  exit=1; errors=2 (`run: test` exit 1 after 78.6s, `run: lint` exit 1 after
  3.4s). Both remain the base-branch failures accepted under TASK-2873801a
  RULING-003, unchanged and with nothing new joining them: the build-environment
  reconstruction manifest test (92 script tests ran, the same 1 failed) and
  `prettier --check` on `src/components/network/DeviceNetworksPanel.tsx`.
- 2026-09-12 validate: verification scope for a documentation-only candidate. No
  executable file changed, so no suite could be affected and none was rerun for
  this task; the last full evidence on this branch is recorded under
  TASK-38d62913, where the Go suite, the three Python suites, the browser suite
  and the GUI build all passed. The traceability and format checks in the
  advisory catalog ran as part of the gate above.
- 2026-09-12 validate: DEVIATION validate: the configured gate exits non-zero for
  the base-branch failures already accepted for this branch, so this candidate
  advances on the evidence above rather than one green gate command. No new
  failure was introduced.

## Rulings


## Handoff


current: deliver
status: waiting
next: present the validated candidate and wait for the human to commit it; repository Git authority forbids the agent committing, staging or pushing
must_read: .agentic/tasks/TASK-bbeeac49.md, .agentic/steps/6_deliver.md
blockers: none - the base-branch failures accepted under TASK-2873801a RULING-003 remain open on the branch

## Stage report

stage: validate
result: completed
current: deliver
changed: 6 files - 1 added (07_Delivery/NILO_First_Boot_Provisioning_Verification.md) and 5 modified (06_SoftwareDocu/Network_Operations.md, 06_SoftwareDocu/Development_and_Simulation_Flow.md, 05_Software/nilo_assistant/README.md, CHANGELOG.md, .agentic/context/project.md), plus this journal. The task evolution report describes zero files; see the build deviation
evidence: documentation only, so no suite could be affected and none was rerun for this task; the last full evidence on this branch is recorded under TASK-38d62913. check --task --strict --run exit 1 on the two accepted pre-existing causes
issues: no new failure; the accepted base-branch failures are the build-environment manifest test, prettier on DeviceNetworksPanel.tsx and eslint on ManagementAccessPanel.tsx:179
next: present for delivery; the human commits
needs_user: yes
spec_compliance: approved
implementation_quality: approved
review_round: 1
