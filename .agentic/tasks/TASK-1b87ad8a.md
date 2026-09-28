<!--
Copyright (c) 2026 DDP Engineering
This software is licensed under the PolyForm Noncommercial License 1.0.0.
Commercial use requires a separate commercial license.
See the LICENSE.txt in the .agentic folder for full license details.
SPDX-License-Identifier: LicenseRef-PolyForm-Noncommercial-1.0.0
-->


# TASK-1b87ad8a - Simplify Web UI navigation to collapsed Instruments and System sections; start on About

stage: specify
type: feature
path: full
interaction: autopilot
requirements:
parent:
branch: adore/pi-26
base: a89db2fa89b0fdf8a77f24833555b617122cb344
opened: 2026-09-28

## Done checklist (current stage)

- [ ] every in-scope behavior has exactly one requirement
- [ ] behavior inventory covers positive, boundary, failure, and recovery cases
- [ ] required fields filled and prose is self-contained
- [ ] independently variable or verifiable responses are split; inseparable
      atomic outcomes remain together
- [ ] each final requirement normally uses one "shall", in an EARS shape
      where one fits
- [ ] verification names setup, trigger, action, observable oracle, and a
      relevant boundary or failure
- [ ] `affected_paths` set to the best current guess
- [ ] specs written for exact contracts that the requirement cannot carry alone
- [ ] Requirement coverage maps every Frame outcome without overlap or gaps
- [ ] semantic conflicts, assumptions, and unknowns resolved or recorded
- [ ] journal `requirements:` line lists every ID
- [ ] `check` reports no format errors
- [ ] `stage: design`

## Frame

Problem: The authenticated browser starts on Bench and the left navigation exposes Bench alongside expanded nested groups; its Instruments and System labels are not collapsible controls.
Desired outcome: Two top-level navigation sections, initially collapsed, with About as the only initial tab; existing authorized instrument and system work remains accessible.
Actors: Administrator, Operator and Viewer using keyboard, screen reader, or pointer on desktop, compact rail or drawer; a second user signing in on the same browser.
Primary scenario: Restore an authenticated session or sign in -> open About alone, show Instruments and System collapsed -> expand a section to discover permitted destinations; nested groups open deliberately or as needed to expose subsequently selected active content.
Examples: On an empty inventory, opening Instruments reveals an explicit understandable empty state; after opening a System destination, a resource refresh preserves tabs and expansion choices.
Counterexamples: Do not automatically open System because About is active, inherit a previous user's expanded groups, hide either section when inventory is empty/unavailable, or remove Bench Core execution, capture, ownership or APIs.
In scope: Shell page registry, initial workspace state, session-scoped navigation expansion, keyboard/ARIA, role/readiness filtering, three catalogues and layouts, shell/startup/browser tests, relevant UI docs and active requirement reconciliation. Retain access to useful Bench-only diagnostic/inventory information when removing its navigation destination.
Out of scope: Backend services, instrument execution semantics, package delivery, publication and unrelated working-tree modifications. No child task: navigation, startup and documentation are tightly coupled.
Constraints: Existing active requirements for the single-screen status/inventories and empty/failure states (REQ-UX-8fa0c15e, REQ-UX-afb79ae0, REQ-UX-8169ea1d) and the working/core ordering (REQ-UX-30821068) conflict with removing Bench; preserve established information contracts unless explicitly authorized to retire them. Session permission and hardware readiness filtering continue. Git branch already adore/pi-26 and matches journal; no Git mutation permitted.
Success signals: Exactly two top-level section controls in all layouts; About sole initial tab after restoration/new sign-in; both initially collapsed; authorized entries reachable; refresh preserves in-session state; new session resets it; accessible translated controls and empty state; role/layout/startup tests and documentation reflect behavior.
Evidence: STATED: Ticket #26 requested scope, boundaries and acceptance, About proposed as default. OBSERVED: pages.ts registers Bench and DEFAULT_PAGE=bench, filters devices/plugins and role-bound core pages; SideNav.tsx defaults nested groups open and hides empty Instruments; App.tsx seeds ['bench'] and resets tabs on sign-out; AppShell.tsx re-mounts navigation across refresh and routes Bench and About; BenchPage.tsx alone displays detailed inventories, failures and Refresh; gui-foundation.md and Development_and_Simulation_Flow.md document existing navigation and Bench information. INFERRED: Removing Bench without another presentation route would make detailed status/inventory and refresh inaccessible; expansion state owned solely by SideNav would be lost on inventory refresh or drawer remount. UNKNOWN: exact destination/placement for Bench-only inventory and failure information, and whether failed inventory reads should use a distinct Instruments empty/unavailable message; specify/design must settle without removing established contracts.
Assumptions: Adopt the ticket's proposed About landing as the scope decision in this explicitly noninteractive autopilot run; keep Bench Core status and inventories accessible via a System child or equivalent focused view, not necessarily a Bench-named tab. Confirm exact placement in specify/design against existing contracts.
Decision-blocking questions: None requiring an external answer: About is explicitly proposed, and the information-preservation conflict has an in-scope solution; do not silently retire the existing inventory requirements.
Proposed behaviors: Remove Bench as a navigation and opening-tab destination; use About as the single startup/reset tab; keep both collapsible section controls present; default groups collapsed and expand only when explicit navigation within an opened section needs visibility; keep expansion across refresh/drawer, reset on session change; retain permission/readiness filtering and a differentiated zero-instrument display; provide three-language labels and keyboard/ARIA semantics; preserve Bench-only inventory/status/refresh behavior in an authorized System presentation route to be specified.
Path and reason: full. Criterion | result | evidence: no new externally observable behavior | no | two new controls, collapsed entry and changed landing; no new interface/data format | yes | frontend presentation only; no new dependency | yes | existing React/i18n; existing requirement covers behavior | no | REQ-UX-30821068 specifies different layout; no security/data handling change | yes | preserve existing role/authorization and resource handling. Any no selects full.

## Requirement coverage


## Design


## Log

- 2026-09-28 frame: Autopilot scope accepted as requested, including proposed About landing and retention of Bench-only information; full path because observable navigation and startup change. Existing inventory/navigation requirement conflicts require reconciliation in specify. No children; unrelated dirty toolchain files excluded.

## Rulings


## Handoff

current: specify
status: ready
next: Reconcile the existing Bench inventory/status requirements with the two-section navigation and specify testable behaviors.
must_read: .agentic/steps/2_specify.md; .agentic/tasks/TASK-1b87ad8a.md; .agentic/requirements/ux/REQ-UX-8fa0c15e.toml; .agentic/requirements/ux/REQ-UX-afb79ae0.toml; .agentic/requirements/ux/REQ-UX-30821068.toml
blockers: none

## Stage report

stage: frame
result: completed
current: specify
changed: Framed two-section navigation and About landing; selected full path and autopilot; recorded information-preservation boundary.
evidence: Branch adore/pi-26 matches journal; pages.ts, SideNav.tsx, App.tsx, AppShell.tsx, BenchPage.tsx and active UX requirements inspected; no tests run in frame.
issues: Exact replacement presentation for Bench-only status/inventory/refresh and inventory failure messaging to resolve at specify/design; unrelated existing dirty toolchain files excluded.
next: Specify independently verifiable navigation, startup, preservation and recovery requirements; reconcile conflicting active requirements.
needs_user: no
spec_compliance:
implementation_quality:
review_round:
