# TASK-1ef49b01 - Correct serial disconnect polling feedback and saved-session alignment

stage: done
type: fix
path: quick
interaction: autopilot
requirements: REQ-UX-7338451f
parent:
branch: master
base: d2bb8be8803edb07b2f471937c12b5a6119f2320
opened: 2026-09-27

## Done checklist (current stage)

- [x] Implementation and integrated validation complete
- [x] Requested Pi deployment verified
- [x] User authorized task closure and local commit
- [x] Close-out included in this delivery commit

## Frame

Problem: Late serial read failures appear as HTTP 503 after a successful disconnect; populated session rows crowd checkboxes against dates.
Desired outcome: Show authoritative disconnected state and clearly separated log selection labels.
Actors: Serial operator and administrator.
Primary scenario: Read races with Disconnect; refresh confirms disconnected and the page shows normal status.
Examples: Closing a port while a read is outstanding; selecting a saved log by its date label.
Counterexamples: Failed reads on an active connection and authorization failures remain visible.
In scope: GUI read-error reconciliation, normal disconnect presentation, saved-log label spacing and regression proof.
Out of scope: Core/runner changes, physical payload transmission, dependency changes, Git publication.
Constraints: User requested autonomous live corrections and explicitly authorized restart/deployment. Existing branch retained under Git authority.
Success signals: Regression reproduces false error before fix and passes afterward; existing error paths remain tested; browser layout confirmed.
Evidence: OBSERVED live disconnect warning/error in Chrome, Core rejects reads for a stopped connection, populated real saved-log labels crowded; STATED user wants Camera-consistent interface corrections.
Assumptions: A fresh successful status reporting disconnected supersedes the failed old read.
Unknowns: Physical RX/TX wiring remains outside this UI correction.
Decision-blocking questions: none.
Proposed behaviors: Reconcile only obsolete read failures against fresh status, preserve active errors and unauthorized handling; hide normal disconnected warning and obsolete owner/expiry; inline spaced log checkbox/date labels.
Path and reason: quick; existing UX requirement, no new interface/data/dependency/authority behavior.

## Requirement coverage

REQ-UX-7338451f covers terminal status feedback and responsive retained-session controls. Positive: closed-port read reconciles. Boundary: active failures stay visible. Tests: SerialWorkspacePage.test.tsx and serial-layout.check.ts.

## Design

Catch only read failure, preserve unauthorized/forbidden errors, re-read workspace once; suppress the old read error only when disconnected or replaced by a different connection. Poll loop and abort cleanup remain unchanged. CSS spaces existing semantic label/input without changing selection behavior.

## Log

- 2026-09-27 frame: Follow-up findings after the earlier GUI candidate was fully validated and deployed. Quick path; existing user autonomy and restart authorization apply. No Git operation authorized. Scope is the two new observed findings, separate from delivered layout review.
- 2026-09-27 build red: REQ-UX-7338451f; command=`npm run test:ci -- src/components/pages/SerialWorkspacePage.test.tsx` through with-tools; Vitest exit=1, 7 passed/1 failed: obsolete read leaves Error HTTP 503 after disconnected status.

- 2026-09-27 build green: Same focused component command passed 11 tests, including closed-read race and active 401/403/503 errors; dedicated serial layout suite passed all four viewports. Parent specification review approves narrow existing-UX correction; no authority or wire changes. Knowledge and changelog reconciled; live checkbox spacing will be rechecked after delivery.

- 2026-09-27 review round 1 -> build: Independent quality review found serial API classifies 403 as rejected, bypassing the authorization guard. Added disconnected-status/403 regression; same focused command exited 1 (11 pass/1 fail, expected missing alert). Corrected 403 to forbidden in existing client classification. Focused component/API run now passes 14 tests. Requirement already covers serial API path/test. Scope remains preservation of authorization errors; second review requested.

- 2026-09-27 final reviews: Independent quality review round 2 approved; parent specification re-review approved for the same final candidate. Obsolete reads reconcile, active/auth failures stay visible, UI selection behavior unchanged. Final full proof next.

- 2026-09-27 validate/deliver completed: Full strict check for TASK-1ef49b01 exited 0; configured full tests 493.7s and lint 10.9s passed. Catalog errors=0, warnings=63, notes=4: inherited stale path/sidecars/unpublished checklists plus existing broad serial candidate paths outside this narrow task, covered by TASK-5344100e. No required verification omitted. Same reviewed code deployed GUI-only with approved restart; served JS/CSS hashes verified over pinned HTTPS. Backup /var/backups/nilo-serial-gui-feedback-20260927-102037. Chrome loaded index-nS8Dr3mb.js; direct DOM confirmed five retained logs, all label gaps 8px, all targets 44px, vertical center difference 0 and no error banners. RealTime View left open; temporary viewport reset. No serial payload sent by agent. Deployment receipt in 07_Delivery is package-operation evidence; source/tests unchanged after proof. Git untouched.

- 2026-09-27 close-out authorized: User requested "close tasks, commit". Complete this task with the repository-local delivery commit, incorporating integrated full proof and Pi deployment in `07_Delivery/NILO_0.11.0_RPi_Live_Test.md`. Physical RX/TX and unplug/replug acceptance remain outside the completed implementation scope. No push requested.

- 2026-09-27 closed: Implementation, regression verification and requested Pi delivery are complete. Closing with the authorized local commit; latest integrated full tests/lint passed (414.0s/9.5s). No executable changes since that proof. Live physical acceptance limitations remain documented in the deployment record.

## Rulings

- RULING-001 | date=2026-09-27 | decision=autopilot and authorized Pi restart/deploy | reason=continuing user-requested live corrections and explicit restart reply | source=user | supersedes=none

## Handoff

current: done
status: ready
next: No further implementation work; user live testing may continue
must_read: .agentic/tasks/TASK-1ef49b01.md, 07_Delivery/NILO_0.11.0_RPi_Live_Test.md
blockers: none

## Stage report

stage: deliver
result: completed
current: done
changed: Completed task closed in authorized local delivery commit; version 0.11.0 deployed
evidence: Latest integrated full tests 414.0s and lint 9.5s passed; Pi deployment verified
issues: Existing repository advisories; physical RX/TX and unplug/replug acceptance remain user testing
next: No further implementation work; user live testing
needs_user: no
spec_compliance: approved
implementation_quality: approved
review_round: 2

## Delivery report (2026-09-27)

- stage: deliver; type: fix; path: quick; branch: master
- requirements:
  - REQ-UX-7338451f - Provide responsive serial terminal and sessions
- deviations: none recorded
- explicit review: enabled; spec_compliance=approved; implementation_quality=approved; review_round=2
- diff vs task base d2bb8be: 161 file(s) changed, 11380 line(s) added
- checks: errors=0 warnings=74 notes=4
- checklist: 6/7 done; open: Git delivery requested by user (not requested).
