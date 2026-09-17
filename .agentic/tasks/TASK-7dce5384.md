<!--
Copyright (c) 2026 DDP Engineering
This software is licensed under the PolyForm Noncommercial License 1.0.0.
Commercial use requires a separate commercial license.
See the LICENSE.txt in the .agentic folder for full license details.
SPDX-License-Identifier: LicenseRef-PolyForm-Noncommercial-1.0.0
-->


# TASK-7dce5384 - Unify contextual activity actions beside page titles

stage: done
type: change
path: full
interaction: autopilot
requirements: REQ-UX-b75fbbc3, REQ-UX-d697e8ec
parent:
branch: master
base: 0c94a5692cc3850f38af6b417a5346ba082504bb
opened: 2026-09-12

## Done checklist (current stage)

- [x] Reviewed working candidate confirmed unchanged at delivery.
- [x] Validation evidence, scoped diff, exclusions and local-only outcome presented.
- [x] User explicitly authorized task closure and local commit.
- [x] Journal and report included in delivery commit.
- [x] Local commit is the sole remaining delivery outcome; no push or PR.
- [x] Close-out carried by delivery commit under the local-only close-out rule.

## Frame

Problem: STATED Authentication places View activity below its title while Network places it beside the title.
Desired outcome: STATED one consistent title-adjacent page-context action and a durable rule for future pages.
Actors: Authorized human browser users; future human and agent implementers.
Primary scenario: Open a page with Activity access -> title and shortcut appear together -> shortcut opens existing contextual Activity.
Examples: Authentication / Users and Authentication / API tokens match Network's title/action association.
Counterexamples: Create/Refresh remain list actions; record-specific Activity stays attached to its record.
In scope: Title/action layout for existing Network and Authentication pages; reusable CSS pattern, requirements, regression tests, GUI knowledge and changelog; GUI-only Pi deployment.
Out of scope: Activity filter changes (including the previously recorded Network exact-filter bug), backend, permissions, new shortcuts on pages without them, unrelated dirty changes.
Constraints: Preserve master per the user's existing branch instruction; no commit/push; 44px targets, supported 800x480 through desktop and all locales. Existing staged Preview.mp4 deletion is unrelated and untouched.
Success signals: Authorized shortcut is beside its page title, no duplicate list shortcut, narrow layouts wrap within the header without overflow, existing identity filtering and permission hiding pass.
Evidence: OBSERVED UsersPage.tsx and ApiTokensPage.tsx place the action in AdministrationPanel; NetworkPage.tsx places it in page-head; shell.css pushes .page-head .button right with margin-left:auto. STATED user grants uninterrupted validation and deployment.
Assumptions: Preserve all existing Activity destinations and visibility rules; no need to add new page capabilities.
Unknowns: None decision-blocking; actual viewport geometry requires browser proof.
Decision-blocking questions: None.
Proposed behaviors: One reusable title/action grouping, with descriptive copy below and context action immediately following title.
Path and reason: Full, because the universal observable layout rule needs a new requirement. No independent child implementation tasks.

| Quick-path criterion | yes/no | Evidence |
| --- | --- | --- |
| No new observable behavior | no | Title/action positioning changes |
| No new public interface/data | yes | Presentation only |
| No new dependency | yes | Existing React/CSS |
| Existing requirement covers all behavior | no | Identity rule governs filtering, not universal placement |
| No security/data change | yes | Existing callbacks and permissions preserved |

## Requirement coverage

| Frame outcome | Atomic behavior | Requirement | Positive case | Boundary/failure | Verification | Design |
| --- | --- | --- | --- | --- | --- | --- |
| Uniform contextual shortcut | Title/action grouping | REQ-UX-b75fbbc3 | Network and both auth titles followed by one action | No callback; narrow/translated content; record actions unaffected | Component DOM/callback tests and browser geometry | Reuse flex title group |
| Preserve contextual operation | Identity Activity navigation | REQ-UX-d697e8ec | Existing identity filter | Missing activity permission | Existing shell integration test | No navigation change |

Behavior inventory: authorized page render -> group shortcut with title -> visible association; narrow render -> wrap within header -> no clipping; missing callback -> omit shortcut; activation -> existing callback -> unchanged Activity context. One new layout requirement, one reused navigation requirement, zero revisions/retirements. No spec needed for this simple presentation contract.

## Design

- REQ-UX-b75fbbc3: Introduce shared `.page-title-actions` wrapping flex group inside `.page-head`; h2 followed immediately by the existing Activity button. Network's other header actions remain adjacent outside that group. Authentication descriptions remain below via a full-width header row.
- Keep existing `.button` styling but override its auto margin only inside the title group; preserve unrelated page headers. Use existing spacing/44px tokens, wrapping and min-width:0 rather than fixed widths.
- REQ-UX-d697e8ec: Callbacks, permission gating, Activity filters and resource state ownership unchanged. No new component abstraction, API, persistence, asynchronous work or architecture diagram is needed for this markup/CSS change.
- Test seams: AuthenticationPage component tests for group ownership, uniqueness, callbacks and absence; existing NetworkPage header test; extend field-display browser helpers to assert title/action geometry on all visited Network/Auth pages in both themes at four sizes. Existing shell test verifies identity filter and permissions.
- Expected files: UsersPage.tsx, ApiTokensPage.tsx, NetworkPage.tsx, shell.css, AuthenticationPage.test.tsx, NetworkPage.test.tsx, e2e/field-display.spec.ts, project.md, gui-foundation.md, CHANGELOG.md and linked requirement/journal.
- Risks: translated titles and small screens may wrap; only the title group overrides auto margins. Preserve all prior dirty content. Alternatives: copying per-page inline styles drifts; a new React component is unnecessary for this small shared pattern.
- Deployment: GUI assets only, checksum readback, backup, restart web service to reload cached index; keep helper/VPN and PLC forwarding running. No unresolved product decisions.

## Log

- 2026-09-12 build green: Focused AuthenticationPage, NetworkPage and shell component suites passed 114/114; field-display browser suite passed 9/9, including title/action geometry across all four sizes and both themes. Requirement activated, project/GUI authoring guidance updated; no navigation/filter changes.
- 2026-09-12 build red: REQ-UX-b75fbbc3; command=`02_Tools/env/with-tools.ps1 npm.cmd --prefix 05_Software/nilo_gui run test:ci -- src/components/AuthenticationPage.test.tsx`; exit=1; expected missing behavior=both auth titles lack the title/action group (2 failures, 36 passing).
- 2026-09-12 frame: Scope accepted under explicit autopilot; full path selected; existing dirty master preserved. New bounded layout task, separate from existing identity/network feature journals.
- 2026-09-12 specify: Created one universal UX layout requirement and reused identity navigation unchanged; positive and boundary checks mapped. No new data/interface or separate spec needed.
- 2026-09-12 design: Minimal shared CSS/markup plan recorded above; all requirement/test seams mapped. Static check exposes inherited catalog/dirty-tree warnings; this task's Handoff/ruling syntax corrected before build.
- 2026-09-12 validate review round 1: Independent spec and quality verdicts approved digest 735f6b2085455214837a9168e9a227875ea1deb8b6e9f7ec17ddc02fb2c1f56e, unchanged before/after. Spec reviewer title_activity_spec and quality reviewer aegis_quality_review_r7 inspected actual files; quality reviewer reused because fresh-agent capacity was exhausted, with previous task findings explicitly excluded. No current-scope findings. Reviewers ran no tests. Focused outputs are tool evidence, not persisted logs; final bridge proof will persist logs.
- 2026-09-12 validate coverage clarification: Field suite includes both themes, but new Network geometry runs in light traversal; Authentication geometry runs in both. Automated geometry is English only, not all locales. Existing identity shell test directly exercises Users and shared callback; no claim of exhaustive old-requirement acceptance. These are unchanged-behavior evidence limits, not new feature claims.
- 2026-09-12 validate final configured proof: Bridge job 3d1e9a1b-9b12-48f3-9d68-50a19eba5e7f ran `check --task TASK-7dce5384 --strict --run`; exit=1, candidate_unchanged=true. Test command failed in 92 script tests on test_reconstruction_lock_is_complete_and_consumed_offline (manifest checksum); lint passed including GUI format/eslint. Full logs: C:/Users/dpasare/.cache/aegis/integrations/3d1e9a1b-9b12-48f3-9d68-50a19eba5e7f.stdout.log and .stderr.log. Manifest, lock and failing test have no Git changes and no task evolution; expected SHA 784a1d02..., actual 66b1e14f.... This task does not alter reconstruction tooling. GUI and downstream tests were not reached by that command. 66 catalog warnings are inherited stale paths/open tasks/whole-dirty-tree coverage; scoped test files are linked_tests rather than source affected_paths. No task journal format error remained.
- 2026-09-12 DEVIATION validate: Repository-wide proof is not green due to unchanged reconstruction-manifest checksum mismatch. Proceed with separately verified GUI-only deployment under existing user authority; leave tooling unchanged and disclose limited full-suite confidence. Separate complete GUI component suite passed 473/473 and production build passed; full browser suite running.
- 2026-09-12 validate GUI proof: Complete browser suite passed 39/39 in 3.7 minutes, including real simulation-backed identity Activity navigation and field geometry. All GUI checks passed on the reviewed unchanged candidate. No full-repository success claimed.
- 2026-09-12 deployment: GUI-only installed at /opt/nilo/gui on 10.205.148.11; prior GUI retained at /home/dpasare/nilo-gui-before-activity-header-20260912. Index SHA256 5ac7684d0dec2b574a5487bfaea5676096ae245f5a112ed063bc0fc853dff526; JS index-yO2Gw6mU.js SHA256 7372ceedfe7b0be1bc54954a7fb8f0ddd5a3c23b946eb00fc499f97711992042; CSS index-BMaftH3e.css SHA256 b579e0d8d82f111757437e85271516896ce9f72a01c7305163b08fbe00b031e6. Local/staged/installed checksums matched; HTTPS index references new assets. Restarted only nilo.service for cached index; both services active, backend/helper binaries and network settings untouched.
- 2026-09-12 live Chrome: Reloaded deployed GUI with existing session; visually inspected Users, API tokens and Network Overview headers. Activity is beside each title, absent from auth list toolbar. API tokens shortcut opened Identity-filtered Activity with records. Remote TCP 10.71.1.130:504 connected in 58ms with no application bytes sent. No commit/push. Validation remains recorded with repository gate limitation, while requested GUI deployment is complete.

- 2026-09-12 deliver: User authorized closure and local commit after the validation limitation was disclosed. Reviewed working candidate fingerprint remains 735f6b2085455214837a9168e9a227875ea1deb8b6e9f7ec17ddc02fb2c1f56e. Commit isolates 13 task files/hunks, including requirement and evidence, from earlier network/backend work and the staged Preview.mp4 deletion. Test and deployment evidence describes the integrated working tree, not an isolated checkout of this commit. The report's 66-file base diff and check counts include earlier dirty work and are advisory, not the commit scope. Source, tests, requirements and knowledge remain unchanged. Local-only close-out is carried by this delivery commit; no push or PR is authorized or performed. The unchanged reconstruction checksum failure remains outside this closed GUI task.

## Rulings

- RULING-001 | date=2026-09-12 | decision=AEGIS autopilot through validation and GUI Pi deployment on master; no commit or push | reason=Explicit current and earlier user instructions | source=user | supersedes=none
- RULING-002 | date=2026-09-12 | decision=Close this task and create its local commit; no push. This supersedes RULING-001 only in its local-commit restriction; the no-push part of RULING-001 stands | reason=User explicitly requested "ok, close task. Commit." after validation limitation and deployment were reported | source=user | supersedes=RULING-001

## Handoff


current: done
status: ready
next: None for this task; earlier uncommitted work and the repository checksum limitation remain separate.
must_read: .agentic/tasks/TASK-7dce5384.md; .agentic/steps/6_deliver.md; .agentic/requirements/ux/REQ-UX-b75fbbc3.toml
blockers: None for authorized local delivery; repository-wide checksum failure remains a recorded validation limitation.

## Stage report

stage: deliver
result: completed
current: done
changed: Uniform Activity headers and future-page requirement implemented and deployed to Pi.
evidence: 473 GUI tests; 39 browser tests; build/lint passed; two reviews approved; live Chrome and PLC TCP checked.
issues: Full strict gate exit 1 on unchanged reconstruction-manifest checksum mismatch; GUI scope passed.
next: None for this task; human controls any later publication.
needs_user: no
spec_compliance: approved
implementation_quality: approved
review_round: 1

## Delivery report (2026-09-12)

- stage: deliver; type: change; path: full; branch: master
- requirements:
  - REQ-UX-b75fbbc3 - Keep contextual Activity actions beside page titles
  - REQ-UX-d697e8ec - Open identity Activity from Authentication pages
- deviations: 1 recorded
  - validate: Repository-wide proof is not green due to unchanged reconstruction-manifest checksum mismatch. Proceed with separately verified GUI-only deployment under existing user authority; leave tooling unchanged and disclose limited full-suite confidence. Separate complete GUI component suite passed 473/473 and production build passed; full browser suite running.
- explicit review: enabled; spec_compliance=approved; implementation_quality=approved; review_round=1
- diff vs task base 0c94a56: 66 file(s) changed, 7287 line(s) added
- checks: errors=0 warnings=69 notes=1
- checklist: 3/6 done; open: Journal and report included in delivery commit.; Local commit outcome delivered; no push or PR.; Close-out carried by delivery commit.
