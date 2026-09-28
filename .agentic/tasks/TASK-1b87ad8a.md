<!--
Copyright (c) 2026 DDP Engineering
This software is licensed under the PolyForm Noncommercial License 1.0.0.
Commercial use requires a separate commercial license.
See the LICENSE.txt in the .agentic folder for full license details.
SPDX-License-Identifier: LicenseRef-PolyForm-Noncommercial-1.0.0
-->


# TASK-1b87ad8a - Simplify Web UI navigation to collapsed Instruments and System sections; start on About

stage: design
type: feature
path: full
interaction: autopilot
requirements: REQ-UX-30821068, REQ-UX-8fa0c15e, REQ-UX-b718864e, REQ-UX-2bc3a464, REQ-UX-eb84f037, REQ-UX-71cbafec, REQ-UX-57d10c15, REQ-UX-74d4256e, REQ-UX-afb79ae0, REQ-UX-8169ea1d, REQ-UX-f300b346, REQ-UX-adbe5465, REQ-UX-2687f591
parent:
branch: adore/pi-26
base: a89db2fa89b0fdf8a77f24833555b617122cb344
opened: 2026-09-28

## Done checklist (current stage)

- [ ] design note written, or its absence justified in one line
- [ ] applicable component, interface, state, failure, security, concurrency,
      compatibility, migration, observability, and operational effects covered
- [ ] each decision cites a requirement ID or named technical constraint
- [ ] every requirement maps to a decision and test seam, or no design impact
- [ ] meaningful alternatives, risks, and open decisions recorded
- [ ] `affected_paths` match the expected file list
- [ ] proposed architecture is in the task when structure changes; current
      component knowledge remains true until validate
- [ ] `stage: build`

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

Behavior inventory (STATED unless noted):

| Frame outcome | Actor/system | Trigger or state | Response | Positive case | Boundary or failure | Observable result |
| --- | --- | --- | --- | --- | --- | --- |
| Two-section navigation | Authenticated shell | Any role and inventory | Two section controls; System destinations permission-filtered | Authorized destinations reachable | No Bench entry; Viewer/Operator restrictions | Only Instruments and System headings |
| About landing/reset | App/workspace | Restore or new user's sign-in | About alone; sections closed | About active, System closed | Previous user's tabs/groups do not leak | One tab, two collapsed controls |
| Controlled hierarchy | Nav in wide/compact/drawer | Toggle/open/tab select | Disclose children without changing tab; expose needed nested ancestors | Keyboard/pointer access to ready device | No implicit top-section expansion for active About or tab switch | Accessible state and visible hierarchy |
| Refresh preservation | App/nav | In-session inventory refresh | Retain tabs and disclosure choices | New eligible entries visible | Failed read does not advertise unverified devices | Tabs/sections remain stable |
| Empty instruments | Nav | Expanded Instruments with no eligible destinations | Translated empty message and section controls | Valid zero inventory | Failed inventory is unknown, not zero | Distinct empty/unavailable copy |
| Information preservation | Authenticated user | Opens System Inventory | Status, integration states, device owners/availability and Refresh together | All reported entries | Valid empty and unreadable resource retain current empty/error handling | Single accessible screen |
| Filtering | Shell | Role/readiness changes | Existing eligible destinations only | Admin/Operator/Viewer routes | Ineligible or unreadable inventory has no instrument entries | Permitted paths only |
| Layout/language | Nav | Viewport/language change | Same disclosure model and translated controls | Wide, compact, drawer; en/de/ro | No unlabeled or non-keyboard controls | Accessible names and expanded state |
| Service boundary | Bench Core | Navigation changes | No service change (OBSERVED pages.ts and BenchPage.tsx are presentation) | Existing execution APIs | No backend removal | Existing contracts unaffected |

| Frame outcome | Atomic behavior | Requirement | Positive case | Boundary or failure | Verification | Design decision or no impact |
| --- | --- | --- | --- | --- | --- | --- |
| Two sections, roles | Section taxonomy/order and permission filtering | REQ-UX-30821068 (revised) | Authorized System routes in order | No Bench entry or unauthorized routes | Role-specific shell and browser assertions | Inventory follows About in System; retain former core relative order |
| About landing/reset | Session initial tab and disclosure reset (inseparable session-entry presentation) | REQ-UX-2bc3a464 (new) | Restored and new sign-in show About only | Previous person's expansion not inherited; About does not open System | Restore, sign-out/sign-in and reload tests | About confirmed per proposed scope |
| Hierarchy | Section and nested toggle interactions across layouts, keyboard and language (one disclosure control contract) | REQ-UX-eb84f037 (new; spec) | Open authorized nested page | Closed section not forced open by active tab | Keyboard, aria, language and layout tests | State transitions in spec |
| Refresh | Preserve in-session tabs and expansion | REQ-UX-71cbafec (new) | Valid inventory changes eligible children | Failed refresh does not erase tab/choices | Shell refresh tests | State must outlive shell remount |
| Empty instruments | Honest zero vs unknown section state | REQ-UX-57d10c15 (new) | Zero result explicitly empty | Unreadable inventory distinct; sections stay | Empty/failure language/layout tests | Unknown resource may coexist with zero eligible hardware |
| Inventory status | Single-screen reported facts and refresh | REQ-UX-8fa0c15e (revised) | System Inventory screen | Missing resource | Stub resource and Refresh test | Keep BenchPage presentation via renamed System route; do not retire information |
| Inventory empty | Explicit valid empty inventory and usable refresh | REQ-UX-afb79ae0 (reused) | Valid zero | No false failure | Existing empty component test plus System route | No semantic change |
| Inventory failure | Resource-specific partial failure | REQ-UX-8169ea1d (reused) | Good resources remain | Malformed integration/device read | Existing resource failure test plus System route | No semantic change |
| Instrument filtering | Readiness filtering and empty child groups | REQ-UX-74d4256e (reused) | Ready integrations/devices | Failed read hides children, not section | Route/shell tests | Section presence distinct from eligible child visibility |
| Network nesting | Four focused Network destinations start closed | REQ-UX-b718864e (revised) | Expand Network, open child | Collapsing keeps tab | Existing shell/drawer tests updated | Other nested groups follow shared disclosure contract |
| Tabs | Normal tab operation | REQ-UX-f300b346 (reused) | Activate/close | Last tab closed | Existing tab tests | No semantic change |
| Viewports | Compact/drawer navigation | REQ-UX-2687f591 (reused) | Same routes across layouts | Drawer Escape/focus | Existing shell/browser tests updated | No semantic change |
| Translations | Full catalogues | REQ-UX-adbe5465 (reused) | en/de/ro text | Identifiers unchanged | Catalogue/language tests | No semantic change |
| Service boundary | Core APIs and execution continue | No frontend requirement: existing service contracts unaffected | Calls unchanged | No service modification | Diff review and existing service tests | No impact |

UNKNOWN: placement within System and exact display label of the inventory route are design-level choices; Inventory is the proposed label in the revised draft requirements. OBSERVED: BenchPage.tsx owns status/inventories/Refresh and pages.ts owns permission/readiness filtering; existing active UX requirements preserve empty and resource-specific failure displays. INFERRED: a dedicated System Inventory route preserves these contracts without opening it initially. No backend contract or new persistence is proposed.

## Design


## Log

- 2026-09-28 frame: Autopilot scope accepted as requested, including proposed About landing and retention of Bench-only information; full path because observable navigation and startup change. Existing inventory/navigation requirement conflicts require reconciliation in specify. No children; unrelated dirty toolchain files excluded.
- 2026-09-28 specify: Four new draft UX requirements, three revised active UX requirements, six reused active requirements, none retired; coverage and disclosure spec recorded. Check reports zero format errors; unrelated tracked sidecar/stage-zone and other-task warnings remain.

## Rulings


## Handoff

current: design
status: ready
next: Design session-owned disclosure state and the System Inventory presentation route against the drafted requirements.
must_read: .agentic/steps/3_design.md; .agentic/tasks/TASK-1b87ad8a.md; .agentic/specs/REQ-UX-eb84f037.md; .agentic/knowledge/gui-foundation.md
blockers: none

## Stage report

stage: specify
result: completed
current: design
changed: Drafted four UX requirements and a disclosure-state spec; revised three active requirements and mapped six reused requirements to the Frame outcomes.
evidence: Behavior inventory and coverage table audited against shell source and GUI/field knowledge; aegis check: errors=0, warnings=21, notes=4; no tests run in specify.
issues: Inventory route placement/label is draft System Inventory proposal for design; check warnings concern unrelated working-tree toolchain edits, tracked sidecars and other tasks.
next: Design expansion state ownership across remounts, System Inventory route and test seams.
needs_user: no
spec_compliance:
implementation_quality:
review_round:
