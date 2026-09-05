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

# TASK-17d71acd — Reorder the core navigation cluster and drop the Settings entry

stage: deliver
type: change
path: full
requirements: REQ-UX-30821068
parent:
branch: nav-core-order
base: e78080ec4e59ac42fd3171ea10c3b2b9653df1f9
opened: 2026-09-05

## Done checklist (current stage)

- [x] validated candidate confirmed unchanged; otherwise returned to validate
- [x] validation evidence, diff, exclusions, and outcome presented
- [x] delivery confirmed by the human
- [ ] journal and report included in the delivery commit
- [ ] agreed outcome delivered, normally a PR
- [ ] close-out committed after the outcome succeeded, or carried by the
      delivery commit where nothing could fail after it

## Frame

Problem: The bottom-anchored core navigation cluster carries a Settings entry
that opens a "not available yet" placeholder, and its remaining entries are not
in the order the operator wants to find them in. (STATED by the user; OBSERVED:
`05_Software/nilo_gui/src/components/shell/pages.ts` CORE_PAGES lists Network,
Activity, Plugins, Authentication, Settings, About, and
`05_Software/nilo_gui/src/components/shell/AppShell.tsx:400` renders the
Settings page as `NotAvailablePage`.)
Desired outcome: The core cluster shows, bottom to top, About, Activity,
Authentication, Network, Plugins - and no Settings entry anywhere. (STATED)
Actors: An operator using the browser interface at any viewport; the same list
is rendered by the compact rail and the constrained drawer, which reuse
`SideNav` (OBSERVED: `src/components/shell/NavDrawer.tsx` renders its children).
Primary scenario: The operator signs in -> the shell builds the navigation from
`buildPages` -> the lower cluster reads, top to bottom, Plugins, Network,
Authentication, Activity, About, with no Settings entry and no Settings page
reachable.
Examples: An administrator at 1920x1080 sees the five core entries in that
order; Network stays a collapsible group with its four child pages, and
Authentication stays a group with API tokens.
Counterexamples: A viewer without the users or tokens permission still sees no
Authentication entry, and the remaining four keep the same relative order
(OBSERVED: `buildPages` permission filter in `pages.ts`).
In scope: `CORE_PAGES` order and the removal of the `settings` descriptor; the
`settings` branch in `AppShell.renderPage`; the now-unused `nav.settings` and
`page.settings.body` catalogue entries in en/de/ro; the shell component test
and the Playwright shell test that assert the order and the placeholder; the
navigation-order statement in REQ-UX-30821068; the navigation sentences in
`.agentic/knowledge/gui-foundation.md` and
`06_SoftwareDocu/Development_and_Simulation_Flow.md`.
Out of scope: The upper working/instrument area, the Network and Authentication
child ordering, the `settings` icon (still used by Network / Adapters), the
`src/api/settings.ts` network-settings client and every Go-side `settings`
identifier, which are unrelated to this navigation entry (OBSERVED). No new
requirement is written for the removal, at the user's instruction.
Constraints: Repository Git authority - never stage, commit or push
(repository CLAUDE.md). Work happens on branch `nav-core-order` in the worktree
`F:/NILO_nav-core-order` (STATED). This is work in progress, so no
compatibility or migration path is owed for the removed entry (STATED).
Success signals: The shell test and the Playwright shell test assert the new
five-entry order; no test or catalogue still names a Settings navigation entry;
`aegis check` reports no orphaned message key or stale requirement path.
Evidence: STATED - the target order, the Settings removal, the branch and
worktree, no compatibility, no requirement for the removal, no commit.
OBSERVED - `src/components/shell/pages.ts`, `src/components/shell/SideNav.tsx`,
`src/components/shell/AppShell.tsx:398-402`,
`src/components/shell/shell.test.tsx:502-523,755-766`,
`e2e/shell.spec.ts:80-98`, `src/i18n/en.ts:33,282`, `src/i18n/de.ts:34,282`,
`src/i18n/ro.ts:34,275`, `.agentic/requirements/ux/REQ-UX-30821068.toml`,
`.agentic/knowledge/gui-foundation.md:185`,
`06_SoftwareDocu/Development_and_Simulation_Flow.md:504,544`.
INFERRED - removing the descriptor is enough to make the page unreachable,
because every route into a page goes through `buildPages` and `findPage`.
Assumptions: "Remove the settings window" means the navigation entry and the
placeholder page it opens, not the appliance's network-settings API.
Unknowns: None material.
Decision-blocking questions: None - the requested order and removal are
unambiguous, and the user stated the scope limits explicitly.
Proposed behaviors: (1) The core cluster is ordered Plugins, Network,
Authentication, Activity, About from the top. (2) No Settings navigation entry
or page exists.
Path and reason: full.

| criterion | yes | no | evidence |
|---|---|---|---|
| no new externally observable behavior | | no | the visible order changes and an entry disappears |
| no new interface or data format | yes | | only an existing descriptor list is reordered |
| no new dependency | yes | | none added |
| an existing requirement already covers the behavior | | no | REQ-UX-30821068 states the old order and names Settings, so it must be rewritten |
| no security or data-handling change | yes | | the permission filter is untouched |

Two `no` rows select the full path; `specify` updates REQ-UX-30821068 rather
than adding a requirement, and `design` is recorded as skipped for a
descriptor-order change that adds no new seam. No child tasks: the work is one
small, tightly coupled change.

## Requirement coverage

Behavior inventory:

| Frame outcome | Actor/system | Trigger or state | Response | Positive case | Boundary or failure | Observable result |
|---|---|---|---|---|---|---|
| Core cluster ordered Plugins, Network, Authentication, Activity, About | Signed-in operator / browser shell | The shell renders the navigation for a session | Emit the lower core group in that top-to-bottom order | Administrator sees the five entries in order | Viewer without users, tokens or activity permission | The lower group's entry labels read in the stated order |
| No Settings entry or page | Signed-in operator / browser shell | Same render | Emit no Settings entry, and no Settings page to open | No Settings label anywhere in the navigation | A stale link or restored tab naming `settings` finds no page | Settings is absent from the navigation and unreachable |

| Frame outcome | Atomic behavior | Requirement | Positive case | Boundary or failure | Verification | Design decision or no impact |
|---|---|---|---|---|---|---|
| Reordered core cluster without Settings | The lower navigation group contains exactly Plugins, Network, Authentication, Activity and About, in that top-to-bottom order, subject to permission | REQ-UX-30821068 (revised) | Administrator render asserts the five labels in order | Viewer render asserts Authentication and Activity absent and the rest in relative order | Shell component test and Playwright shell test on `.nav-lower` entry text | No design impact - one descriptor list is reordered and one entry deleted |

Deliberate many-to-one mapping: the two Frame behaviors are one inseparable
observable outcome. "Contains exactly those five entries in that order" already
states the absence of a Settings entry, and a separate requirement for the
removal would both duplicate it and contradict the user's instruction not to
record removed features as requirements (RULING-002).

## Design

Skipped. The change reorders one descriptor array and deletes one entry from
it, with the matching render branch and catalogue keys; there is no new
interface, data format, dependency, state or trust boundary, and `SideNav`
already renders `CORE_PAGES` in array order. The single coverage row is marked
`no design impact`. Component knowledge `.agentic/knowledge/gui-foundation.md`
names the old order in one sentence and is corrected in build.

## Log

- 2026-09-05 frame: full path - the visible navigation order changes and an
  entry is removed, and REQ-UX-30821068 states the superseded order, so it is
  revised in specify. Scope accepted from the user's written instruction; no
  child tasks. Excluded: the network-settings API and every unrelated
  `settings` identifier, and any new requirement for the removal.
- 2026-09-05 frame -> specify.
- 2026-09-05 specify: created 0, revised 1 (REQ-UX-30821068), reused 0,
  retired 0. The revision replaces the superseded order sentence, which also
  omitted the Activity entry the shell already builds, and restates
  verification over the administrator, browser and viewer cases. No spec: the
  requirement carries the whole contract.
- 2026-09-05 specify -> design -> build. Design skipped, reason in ## Design.
- 2026-09-05 build red: REQ-UX-30821068; command=`npm exec -- vitest run
  src/components/shell/shell.test.tsx` (in 05_Software/nilo_gui); exit=1;
  2 failed of 34; expected missing behavior=the lower cluster still renders
  Network, Activity, Plugins, Authentication, Settings, About instead of
  Plugins, Network, Authentication, Activity, About without Settings.
- 2026-09-05 build: reordered CORE_PAGES, deleted the `settings` descriptor,
  its AppShell NotAvailablePage branch and the `nav.settings` /
  `page.settings.body` entries in en, de and ro; corrected the navigation
  sentences in the GUI knowledge page and Development_and_Simulation_Flow.md.
  A Playwright rail assertion counting six core entries was updated to five -
  found by the impacted run, not a candidate defect.
- 2026-09-05 build green: REQ-UX-30821068; command=`aegis test --impacted
  --base HEAD`; exit=0; 38 Playwright tests passed. Full component suite
  `npm exec -- vitest run` exit=0, 426 tests in 29 files.
- 2026-09-05 build -> validate. `evolution` listed exactly the seven expected
  candidate files; `check --task` exit=0 errors=0.
- 2026-09-05 validate: diff reviewed hunk by hunk against the Frame scope -
  eight candidate files, no unrequested work, no error path, secret,
  portability or permission behavior touched. Component knowledge
  `.agentic/knowledge/gui-foundation.md` reconciled: the requirement line, the
  Activity placement sentence and a new dated section for this change.
  REQ-UX-30821068 re-read: status active, prose and verification describe the
  built behavior, paths and linked tests current. No manual or inspection
  verification applies.
- 2026-09-05 validate final: command=`python .agentic/aegis.py check --task
  TASK-17d71acd --strict --run`; exit=0; strict ok; errors=0 warnings=10
  notes=1. Ran the configured `test` (full Go, component and Playwright suite,
  322.3s, ok) and `lint` (11.5s, ok). Every remaining warning is pre-existing
  on master and outside this task: REQ-FUNC-35da79f1 stale paths and missing
  linked tests, the REQ-TEST-add2dfbd manual-verification note, TASK-943403e2
  invalid Handoff status, and unticked deliver items on five other open tasks.
  No warning names this task or its files.
- 2026-09-05 validate -> deliver. Candidate unchanged since the proof.
- 2026-09-05 correction: the `2026-09-05 build -> validate` entry above says
  `evolution` listed seven candidate files. The saved report lists eight; the
  seventh-file count was taken from the pre-anchor run, which could not include
  `shell.test.tsx` because the anchor is taken after the red test is written.
  The entry stands as written and this line corrects it.
- 2026-09-05 deliver -> validate. Review of the presented candidate found two
  documentation records still stating the superseded navigation: the
  [Unreleased] shell entry in `CHANGELOG.md` and the "Agreed layout" bullet in
  `00_Planning/GUI_Design/README.md`. Both are corrected, so the recorded proof
  no longer describes the candidate and the repository is proved again.
  `CHANGELOG.md` needed no Removed entry: the Settings navigation entry has
  never been released, so the Unreleased Added bullet is corrected in place.
  The design reference keeps the 2026-09-02 agreement visible and names what
  superseded it rather than rewriting the record.
- 2026-09-05 validate: swept the repository for every other record of the
  superseded order. The remaining hits are deliberately left: other tasks'
  journals and the `Development_Simulation_Vertical_Slices.md` slice record are
  append-only history; the image-generation prompt in
  `00_Planning/GUI_Design/README.md` records what was asked of the mockup tool;
  and `00_Planning/TODO.md` describes a Settings subsystem that is still planned
  and never claimed a navigation entry. Cancelling that backlog item would be a
  product decision, not a documentation correction, so it is raised with the
  human instead.
- 2026-09-05 validate final (re-proof): command=`python .agentic/aegis.py check
  --task TASK-17d71acd --strict --run`; exit=0; strict ok; errors=0 warnings=11
  notes=1. Ran the configured `test` (316.8s, ok) and `lint` (8.9s, ok). Ten
  warnings are the same pre-existing ones as the previous proof. The eleventh,
  `outside-layout CHANGELOG.md`, is a known repository configuration gap already
  recorded by TASK-a86b32ec: `CHANGELOG.md` sits outside the `[layout]` globs in
  `aegis.toml`, so any change to it is reported. It is accepted, not a candidate
  defect, and repository configuration is out of this task's scope.
- 2026-09-05 validate -> deliver. Candidate unchanged since this proof.
- 2026-09-05 deliver: the human approved integrating this candidate into the
  local `master` checkout. This supersedes the earlier no-commit limit only for
  the local commits needed to complete that integration; no push or other
  remote publication is authorized.

## Rulings

- RULING-001 | date=2026-09-05 | decision=The user's written instruction is the frame scope acceptance; no separate checkpoint stop was taken. | reason=The order, the removal and the scope limits were stated completely and unambiguously in the request, and the pre-delivery checkpoint remains. | source=user | supersedes=none
- RULING-002 | date=2026-09-05 | decision=The removed Settings entry gets no requirement of its own; REQ-UX-30821068 is revised to state the new order without it. | reason=The user instructed that removed features are not recorded as requirements. | source=user | supersedes=none
- RULING-003 | date=2026-09-05 | decision=No compatibility or migration path is provided for the removed navigation entry. | reason=The user stated this is development work in progress. | source=user | supersedes=none
- RULING-004 | date=2026-09-05 | decision=Create the local commits needed to integrate the validated candidate into local master, without pushing. | reason=The human explicitly requested the reviewed changes be merged into the main local development branch. | source=user | supersedes=none

## Handoff

current: deliver
status: active
next: Create the reviewed feature commit, merge it into the local master checkout while preserving its existing development commit, and validate the integrated tree.
must_read: .agentic/tasks/TASK-17d71acd.md
blockers: none

## Stage report

stage: deliver
result: in_progress
current: deliver
changed: 05_Software/nilo_gui/src/components/shell/pages.ts, 05_Software/nilo_gui/src/components/shell/AppShell.tsx, 05_Software/nilo_gui/src/components/shell/shell.test.tsx, 05_Software/nilo_gui/e2e/shell.spec.ts, 05_Software/nilo_gui/src/i18n/en.ts, 05_Software/nilo_gui/src/i18n/de.ts, 05_Software/nilo_gui/src/i18n/ro.ts, 06_SoftwareDocu/Development_and_Simulation_Flow.md, CHANGELOG.md, 00_Planning/GUI_Design/README.md, .agentic/requirements/ux/REQ-UX-30821068.toml, .agentic/knowledge/gui-foundation.md, .agentic/tasks/TASK-17d71acd.md
evidence: re-proof after the documentation corrections - `check --task TASK-17d71acd --strict --run` exit=0, strict ok, errors=0 warnings=11 notes=1, running the configured full test command (316.8s ok) and lint (8.9s ok)
issues: eleven accepted warnings, none a candidate defect - ten pre-existing on other tasks and requirements, plus outside-layout on CHANGELOG.md, a known aegis.toml layout-glob gap already recorded by TASK-a86b32ec; 00_Planning/TODO.md still lists a planned Settings subsystem, which is a product decision for the human, not a stale record
next: create the reviewed feature commit, merge it into local master, and validate the integrated tree; do not push
needs_user: no - the human explicitly approved local integration, and the unrelated planned Settings subsystem remains outside this navigation task
spec_compliance: disabled
implementation_quality: disabled
review_round:

## Delivery report (2026-09-05)

- stage: deliver; type: change; path: full; branch: nav-core-order
- requirements:
  - REQ-UX-30821068 - Group the navigation into working entries and a core cluster
- deviations: none recorded
- explicit review: disabled; existing validation remains required
- diff vs task base e78080e: 15 file(s) changed, 3035 line(s) added
- checks: errors=0 warnings=13 notes=1
- checklist: 3/6 done; open: journal and report included in the delivery commit; agreed outcome delivered, normally a PR; close-out committed after the outcome succeeded, or carried by the delivery commit where nothing could fail after it
