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

# TASK-a03aef5b — Separate plugin administration and named bench work

stage: frame
type: change
path: full
requirements:
parent:
branch: master
base: 09a411552bf2f3bd284608c554a1bdd9fe074950
opened: 2026-09-05

## Done checklist (current stage)

- [x] Frame card states problem, outcome, actors, scenario, scope, examples, constraints, success signals, evidence and unknowns.
- [x] Material claims classified as stated, observed, inferred or unknown.
- [x] Contradictions and decision-blocking questions resolved or presented.
- [x] Full path chosen with reason logged.
- [x] Existing master retained under the user's branch instruction.
- [x] Task journal created with a complete header.
- [x] Proposed behaviors map to the desired outcome and success signals.
- [ ] Scope confirmed for implementation; this request is planning only.
- [ ] Stage advanced after scope acceptance and instruction to implement.

## Frame

Problem: Plugin administration, capture configuration and technical transaction history occupy one long page; saved work lacks names and deletion, and the navigation does not express the user's virtual bench.
Desired outcome: A reviewable plan for separate installed-plugin administration and enabled-plugin working pages, meaningful saved results, compact consistent presentation and server-enforced RBAC.
Actors: Administrator, operator, viewer, GUI session, scoped CLI/MCP client and shared appliance service.
Primary scenario: Administrator enables installed Sigrok -> it appears under Bench -> operator captures and names a memory-read test -> later finds/downloads/deletes owned results -> administrator uninstalls Sigrok while saved history remains accessible.
Examples: Capture name "SPI memory-read test" with optional project label; operator restarts idle Sigrok without enable/uninstall permission.
Counterexamples: Restart granting plugins.manage to an operator; uninstall silently deleting captures; hiding a plugin on failed refresh; a capture deadline treated as a deletion date; adding SPI decoder functionality just because an example name mentions SPI.
In scope: Planning only for dynamic navigation, plugin metadata/version, separate administration/working views, lifecycle confirmations, narrow restart permission, contextual audit/help, named/paginated/deletable saved results, storage bounds, asynchronous state and accessible compact EN/DE/RO themed UI.
Out of scope: Implementation, deployment, commit, push, driver/signal qualification, PWM, additional device adapters, per-project workspaces/ACLs, marketplace and unrelated page redesign.
Constraints: Keep master under persistent user instruction; never push; preserve unrelated files. Product requirements only, not repository-tool requirements. Follow existing compact help/theme/localization patterns and development compatibility policy.
Success signals: Plan accounts for each requested page/action, RBAC, current transaction/deadline semantics, retained-data access after uninstall, concurrency, failure states and desktop/field layouts; defines implementation sequence and verification.
Evidence: STATED user brief, screenshot and explicit clarification that plugin configuration is per NILO appliance, shared by all users. OBSERVED source evidence is catalogued in .agentic/specs/plugin-workflow-plan.md section 1. INFERRED proposed page structure and capture model in that plan. UNKNOWN storage budget default and whether old Pi captures need preserving across a future feature-data reset.
Assumptions: Owner-scoped result access with admin override; saved captures retained until explicit deletion; no new general audit access for operators. These remain recommendations; appliance-wide admin enablement is confirmed by the user.
Unknowns: Storage default should reflect the edition; automated retention is not silently selected. If future implementation resets feature-owned development data, resolve preservation of existing Pi captures before doing so.
Decision-blocking questions: None to deliver a plan. Appliance-wide enablement is settled; saved-data policy remains proposed. No request for implementation has been made this turn.
Proposed behaviors: Detailed plan in .agentic/specs/plugin-workflow-plan.md; requirements remain uncreated until specify. Update existing navigation, lifecycle/restart, evidence and GUI/CLI/MCP behaviors and add atomic naming/deletion/storage and ownership requirements during specify.
Path and reason: Full; the proposed work changes externally visible UI/API/data handling and authorization. One coordinated task; optional explicit review disabled in current config, no delegated work.

| Quick-path criterion | Yes | No | Evidence |
|---|---|---|---|
| No new observable behavior | | X | Two navigation groups, saved-result lifecycle |
| No new interface/data format | | X | Metadata, paging and deletion contracts |
| No new dependency | X | | Reuse existing components and service stack |
| Existing requirements fully cover behavior | | X | Operator restart and named deletion need specification |
| No security/data-handling changes | | X | New permission and ownership checks; storage/deletion |

## Requirement coverage

<!-- A broad Frame outcome may produce several atomic behavior rows. Each
atomic behavior maps to exactly one requirement. Keep this table current
through design.
| Frame outcome | Atomic behavior | Requirement | Positive case | Boundary or failure | Verification | Design decision or no impact |
|---|---|---|---|---|---|---|
-->

## Design

The requested planning artifact is .agentic/specs/plugin-workflow-plan.md.
It is a proposal for scope review, not a completed design-stage claim.

## Log

- 2026-09-05 frame: Inspected current source and supplied screenshot; drafted the complete workflow/interface/RBAC plan. Full path selected. Planning request fulfilled; remain in frame pending future implementation instruction. No production files or Pi state changed and no tests claimed.
- 2026-09-05 clarification: User confirmed one plugin configuration per NILO appliance. Updated the plan to make shared installed/active plugin state a stated constraint, not an assumption or future per-user/project choice.

## Rulings

- RULING-001 | date=2026-09-05 | decision=This turn is planning only; retain master and never push | reason=Current plan request and persistent Git instructions | source=user | supersedes=none
- RULING-002 | date=2026-09-05 | decision=Enable/disable/uninstall admin-only; allow operator restart; PWM excluded | reason=Explicit user product scope | source=user | supersedes=none
- RULING-003 | date=2026-09-05 | decision=Plugin configuration and active selection are per NILO appliance and shared by all users; no independent user/project benches | reason=Explicit user clarification | source=user | supersedes=none

<!-- Append decisions here; never edit history. Supersede with a new ruling.
- RULING-001 | date=YYYY-MM-DD | decision=<decision> | reason=<reason> | source=user | supersedes=none
source: user | repository | requirement | technical
-->

## Handoff

<!-- Replace these fields with the latest minimal resume snapshot.
status: ready | waiting | blocked
-->

current: frame
status: waiting
next: Review proposed workflow scope before any implementation.
must_read: .agentic/tasks/TASK-a03aef5b.md; .agentic/specs/plugin-workflow-plan.md
blockers: None for planning; implementation not requested this turn.

## Stage report

stage: frame
result: paused
current: frame
changed: Plan records one shared plugin configuration per NILO appliance.
evidence: Explicit user clarification; documentation diff checked.
issues: Storage policy remains proposed; no implementation tested.
next: Review the plan before implementation.
needs_user: no - requested planning is complete; implementation requires a later instruction.
spec_compliance: disabled
implementation_quality: disabled
review_round:
