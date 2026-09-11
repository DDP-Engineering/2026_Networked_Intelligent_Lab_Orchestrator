<!--
Copyright (c) 2026 DDP Engineering
This software is licensed under the PolyForm Noncommercial License 1.0.0.
Commercial use requires a separate commercial license.
See the LICENSE.txt in the .agentic folder for full license details.
SPDX-License-Identifier: LicenseRef-PolyForm-Noncommercial-1.0.0
-->


# TASK-e9236fc5 — Refine final login interface

stage: done
type: change
path: full
requirements: REQ-UX-40473e43, REQ-FUNC-5d80ae6c, REQ-UX-8a1ddb96, REQ-UX-10182ba0, REQ-UX-fdb7c574, REQ-FUNC-a6c14c23, REQ-SEC-66b84b2c, REQ-UX-188a4860, REQ-UX-66636ac9
parent:
branch: master
base: c996736a0b570d43ea9c9ffed27ec67492a052d5
opened: 2026-09-10

## Done checklist (current stage)

- [x] validated candidate confirmed unchanged; otherwise returned to validate
- [x] validation evidence, diff, exclusions, and outcome presented
- [x] delivery confirmed by the human
- [x] journal and report included in the delivery commit
- [x] agreed outcome delivered, normally a PR
- [x] close-out committed after the outcome succeeded, or carried by the
      delivery commit where nothing could fail after it

## Frame

Problem: The signed-out GUI exposes development-only labels, instructions, bearer-token controls, endpoint/runtime notes, and no way to inspect a typed password or identify the NILO version before authentication.
Desired outcome: A delivery-quality login surface that shows the NILO version before sign-in, offers an accessible password visibility control, and remains usable from a seven-inch low-resolution display through a high-resolution desktop.
Actors: A NILO user signing in; a developer exercising the same production-facing login surface.
Primary scenario: Open the signed-out interface -> see NILO and its version without authenticating -> enter username and password -> optionally reveal or hide the password -> sign in.
Examples: At 1024x600 and 800x480 the complete login remains readable and operable; at 1920x1080 it remains proportionate rather than stretching into a sparse full-width form.
Counterexamples: The signed-out page must not show "Development interface", development bearer-token guidance or controls, the configured endpoint, mock/live notes, or simulation/appliance boundary prose. Necessary credential validation, refusal, throttling, ended-session, and signed-out feedback is not decorative commentary and remains.
In scope: Signed-out login markup and responsive styling; localized visible login strings; password reveal/hide behavior and accessibility; a pre-authentication project-version value derived from the canonical GUI version mirror; component and browser regression coverage; positive, current-state requirements plus reconciliation of affected knowledge, development-flow documentation, and changelog.
Out of scope: Authenticated workspace presentation; backend authentication semantics; CLI or MCP schema changes; project version assignment; removal of bearer-token support from the API client or non-GUI tooling; redesign of identity-administration password fields.
Constraints: Work directly on `master` as explicitly requested; do not push; preserve the unrelated untracked `09_Others/Preview.mp4`; follow the 44x44-pixel control and >=14-pixel text rules; target 1024x600 primary, 800x480 constrained, and normal/high-resolution desktop layouts; do not fetch protected status before login merely to learn a version.
Success signals: The signed-out surface contains only product identity/version, credential controls, actions, and state/error feedback; version is visible before any request succeeds; password starts masked and toggles both ways without losing its value; keyboard and assistive-technology users receive an unambiguous toggle; no clipping, horizontal document overflow, undersized text, or undersized control at the field viewports; existing human authentication still succeeds.
Evidence: STATED - the user requested the password-display button, removal of development commentary/artifacts, pre-login NILO version, work on `master`, and seven-inch-to-high-resolution suitability. OBSERVED - `SignInScreen.tsx` currently renders the development eyebrow, SIM badge, explanatory intro, token form, endpoint, boundary, and mode note; `package.json` mirrors project version 0.2.0; `Development_and_Simulation_Flow.md` defines 1024x600, 800x480, >=14px text, and >=44x44px controls; `gui-foundation.md` currently requires development boundary/token presentation. INFERRED - the build-time GUI version mirror is the only trustworthy pre-auth value without weakening authentication. UNKNOWN - no separate visual mock for the final login was supplied beyond the current-state screenshot.
Assumptions: "Clean final login" means the signed-out card presents only the final product identity/version, human credential path, and messages needed to complete or recover sign-in. The label will be concise (`NILO {version}` or an equivalent localized presentation), not a claim that protected backend status has been read. Durable requirements describe only this intended current state and do not name removed elements or historical transitions.
Unknowns: Whether a later appliance-branding pass will supply a logo or exact typography; this task preserves the existing design system and improves only the stated login surface.
Decision-blocking questions: None; the repository already defines the minimum field viewports and the user's "similar artefacts" direction supports the stated removal boundary. The ordinary AEGIS frame checkpoint still requires scope acceptance before implementation.
Proposed behaviors: 1) Present NILO and the project version before login. 2) Present username/password and sign-in as the credential path on the page. 3) Provide a reveal/hide control adjacent to the password with localized accessible naming, masked by default and value-preserving. 4) Present actionable validation/session/refusal feedback. 5) Use a bounded responsive card that fits 800x480 and 1024x600 and remains balanced on high-resolution displays. 6) Express durable requirements and product documentation as the intended final state; keep removal history only in task evidence and the changelog.
Path and reason: Full path. Criterion analysis: no new externally observable behavior = no (password toggle and version are new); no new interface or data format = no (signed-out interface changes); no new dependency = yes; existing requirement already covers behavior = no; no security/data-handling change = no (password visibility and removal of a credential path require explicit security treatment). No child tasks: the markup, styling, localization, and tests are tightly coupled and small enough for one task.

## Requirement coverage

| Frame outcome | Atomic behavior | Requirement | Positive case | Boundary or failure | Verification | Design decision or no impact |
|---|---|---|---|---|---|---|
| Final product login | Present the complete human sign-in composition and actionable state feedback | REQ-UX-40473e43 | Signed-out screen has product identity and usable fields/action | Incomplete, refused, throttled, ended and signed-out states remain actionable | Component states | SignInScreen owns the bounded composition |
| Human credential focus | Offer one username/password submission path | REQ-FUNC-5d80ae6c | One form invokes password sign-in | No parallel signed-out credential form | Component role/form count and client call | SignInScreen exposes only onSignIn |
| Pre-auth identity | Show canonical project version without authentication | REQ-UX-8a1ddb96 | Package version is visible immediately | Protected status has not answered | Recording client plus production build | Import the package mirror at build time and pass it as display data |
| Password inspection | Toggle the same password between masked and visible | REQ-UX-10182ba0 | Show then Hide preserves and submits the value | Initial state is masked; action remains keyboard/AT operable | Component interaction test | Local boolean state controls input type and action label |
| Display range | Keep the login complete from 800x480 through 1920x1080 | REQ-UX-fdb7c574 | All required content and targets fit at each viewport | No horizontal overflow, undersized targets/text, or unbounded desktop card | Playwright geometry checks | Dedicated login card and password-field layout rules |
| Existing authentication | Preserve username/password session behavior | REQ-FUNC-a6c14c23 | Valid credentials enter the workspace | Refused credentials remain on login | Existing component and browser sign-in tests | No client/API change |
| Credential handling | Keep the entered password out of durable/logging channels | REQ-SEC-66b84b2c | Password remains component state until submission | Toggling visibility does not write storage, URLs, cookies, or console | Existing storage/console spies plus toggle regression | Visibility is input presentation only |
| Authenticated runtime identity | Keep runtime status in the authenticated top bar | REQ-UX-188a4860 | SIM/LIVE badge remains after sign-in | No duplicated authenticated runtime notice | Existing shell tests | No authenticated-shell design impact |
| Authenticated resource states | Keep connection outcomes distinct and recoverable | REQ-UX-66636ac9 | Healthy status enters the workspace | Pending, ended-session, unreachable and unsupported states remain distinct | Existing component state tests | Retry keeps the current human session when it remains valid |

## Design

Requirements coverage: `REQ-UX-40473e43` owns the final signed-out composition; `REQ-FUNC-5d80ae6c` owns its single human credential path; `REQ-UX-8a1ddb96` owns the pre-auth version; `REQ-UX-10182ba0` owns password visibility; `REQ-UX-fdb7c574` owns viewport geometry; `REQ-FUNC-a6c14c23` and `REQ-SEC-66b84b2c` are preserved through regression tests; `REQ-UX-188a4860` has no authenticated-shell design impact; `REQ-UX-66636ac9` keeps the authenticated resource outcomes distinct and recoverable.

Component responsibilities: `SignInScreen` owns all signed-out presentation and ephemeral field state. A small `version.ts` module reads the checked-in GUI package mirror and exports the displayed project version. `App` owns session restoration/sign-in and no longer exposes an alternate signed-out credential callback. The API client retains bearer credentials for non-login consumers and tooling.

Interfaces and contracts: `SignInScreenProps` retains notice, working, refusal and `onSignIn`; endpoint, API mode, runtime profile and token callback are removed because they have no place in the final signed-out contract (`REQ-UX-40473e43`, `REQ-FUNC-5d80ae6c`). The version is imported from the package mirror rather than fetched from the protected status API (`REQ-UX-8a1ddb96`). No wire contract changes.

State and execution: username, password and one `passwordVisible` boolean remain local and non-persistent. The toggle changes only the input `type`, accessible action text and pressed state; the same password string is submitted (`REQ-UX-10182ba0`, `REQ-SEC-66b84b2c`). Sign-in and restoration continue through the existing session flow (`REQ-FUNC-a6c14c23`).

Failure recovery: incomplete, refused, throttled, ended-session and signed-out messages remain beside the form. Backend-unreachable or malformed authenticated loads retain their retry/sign-in recovery card, but its header uses product identity rather than environment diagnostics (`REQ-UX-40473e43`).

Security and trust boundary: rendering a password as text is an explicit user-controlled, reversible local presentation state. It creates no storage, URL, cookie, console or network behavior. The field remasks on component remount because state defaults to false (`REQ-UX-10182ba0`, `REQ-SEC-66b84b2c`).

Responsive layout: a dedicated sign-in card class stays capped at the existing readable width. The password input and 44-pixel-minimum toggle share a shrinking grid row; the card uses the existing 14-pixel minimum type tokens and scroll-safe centred container. Browser geometry is checked at 800x480, 1024x600, 1366x768 and 1920x1080 (`REQ-UX-fdb7c574`).

Compatibility and migration: the delivered login deliberately replaces the development-only browser entry path; bearer-token issuance and API credential support remain available outside this surface. Existing browser suites that entered through the prior UI will bootstrap a development administrator and use the supported human sign-in path. No persisted state migration is required.

Observability and tests: component tests go red first for product/version composition, single form, and the full reveal/hide/submit sequence. Browser authentication tests assert all viewports and real sign-in; the bench-core recovery suite uses a real session and proves retry without re-entering credentials. Formatting, lint, component, production build, browser E2E and strict AEGIS validation provide final evidence.

Alternatives and tradeoffs: an unauthenticated status endpoint was rejected because build identity does not justify weakening the protected API. A second hard-coded version constant was rejected because it could drift. A hover-only or icon-only control was rejected because the action must remain explicit and accessible. A full-page desktop form was rejected because it wastes large displays and weakens scanability.

Expected files: `src/version.ts`, `src/components/SignInScreen.tsx`, `src/App.tsx`, `src/styles/shell.css`, three locale catalogues, `src/App.test.tsx`, `e2e/authentication.spec.ts`, `e2e/bench-core.spec.ts`, affected requirements, GUI knowledge, development-flow documentation and `CHANGELOG.md`.

Risks: importing package metadata must remain compatible with the configured TypeScript bundler; browser suites must not leak fixture passwords when changing their sign-in setup; responsive assertions must measure the signed-out card before the shared serial session becomes authenticated.

Open decisions: none. Exact future logo artwork and typography remain outside this task. No architecture drawing is owed because component boundaries remain the same; one tiny read-only version module is a value source, not a new subsystem.


## Log

- 2026-09-10 frame: proposed the full path for a production-facing, responsive login with pre-auth version and password visibility; paused at the checkpoint before scope acceptance.
- 2026-09-10 frame: user accepted the scope, including seven-inch through high-resolution behavior and positive current-state requirement wording.
- 2026-09-10 specify: created three, revised three and reused two requirements; no separate spec is needed because the component interaction and viewport oracles fully constrain the behavior.
- 2026-09-10 design: constrained the implementation to SignInScreen-local presentation state, a package-derived version value, the existing session boundary and real-browser viewport proof; see `## Design`.
- 2026-09-10 build red: command=`.\\02_Tools\\env\\with-tools.ps1 npm.cmd --prefix .\\05_Software\\nilo_gui run test:ci -- src/App.test.tsx`; exit=1; requirements=REQ-UX-40473e43, REQ-FUNC-5d80ae6c, REQ-UX-8a1ddb96, REQ-UX-10182ba0; expected missing behavior=the current screen still lacked the product/version composition and password visibility control and still exposed multiple credential forms.
- 2026-09-10 build green: command=`.\\02_Tools\\env\\with-tools.ps1 npm.cmd --prefix .\\05_Software\\nilo_gui run test:ci -- src/App.test.tsx`; exit=0; 33 component tests cover the final product login, canonical version, password visibility, human session flow and credential handling.
- 2026-09-10 deviation build: responsive login CSS was implemented before the dedicated browser viewport test was written, so REQ-UX-fdb7c574 can receive real-browser proof but not clean pre-implementation red evidence.
- 2026-09-10 deviation build: added DOM library types to `tsconfig.node.json` after the new viewport test exposed that existing Playwright DOM callbacks also fail a clean type-check without them; this is compile-time-only but adds an expected file not named in design.
- 2026-09-10 build green: `npm run test:e2e -- e2e/authentication.spec.ts e2e/bench-core.spec.ts`; exit=0; 15 real-browser tests cover human sign-in, project version, the 800x480/1024x600/1366x768/1920x1080 viewport matrix and same-session backend recovery.
- 2026-09-10 build green: `npm run test:ci`; exit=0; 32 files and 433 tests passed. `npm run lint`; exit=0. `npm run build`; exit=0 with the existing non-failing Vite large-chunk advisory.
- 2026-09-10 build: reviewed the evolution report's 15 candidate files against the accepted scope; `e2e/commands.test.ts` is a security-test adaptation for password redaction and `tsconfig.node.json` is the already-recorded compile-only deviation. AEGIS check has no errors; its remaining warnings are repository-wide stale records/open tasks, the unrelated `09_Others/Preview.mp4`, the test password heuristic, and the pending final evolution state.
- 2026-09-10 validate: reconciled `REQ-UX-66636ac9` to the current authenticated resource states after semantic diff review found its legacy rejected-token wording no longer described the built behavior; existing component tests cover each stated outcome.
- 2026-09-10 validate correction: the focused `npm run test:ci -- src/App.test.tsx` check exited 1 after simplifying the refusal type because refused and throttled messages still tested the removed discriminator; changed the single-form screen to render every non-null refusal and reran focused proof.
- 2026-09-10 validate green: focused `npm run test:ci -- src/App.test.tsx`; exit=0; all 33 tests passed after the refusal-state correction. Full `npm run test:e2e`; exit=0; all 38 serial Chromium tests passed in 5.4 minutes, including the signed-out viewport matrix and real WSL2 sign-in/recovery.
- 2026-09-10 validate gate: pinned-environment `check --strict --run`; exit=1; lint passed across Go, Python and GUI, but the configured test command stopped in its first repository-tooling suite because committed `reconstruction-inputs.json` hashes to `66b1e14f0a02eef6be7b8c0437b259fae576373ae8959226f71d732ca60193be` while the committed lock declares `784a1d020ef72bd4c567a3f85287e2b72e456f2f7bc28db7c470d0e82579da3f`. Neither file differs from HEAD, so this is a pre-existing base-branch failure; it prevents a green repository-wide gate but does not touch the GUI candidate. Candidate digest `4230d76289404a773ca0ea893e1d8276f00a50c00a861558cd816367e17e1bfb` was unchanged by the gate. Remaining advisory warnings are repository-wide stale paths/open delivery tasks, the known fixture-password heuristic, and unrelated untracked `09_Others/Preview.mp4`.
- 2026-09-10 deviation validate: accepted the pre-existing reconstruction-manifest gate failure for this scoped GUI task; full GUI component/build/lint and all 38 real-browser tests provide the scoped substitute, but repository-wide CI remains red until the separate build-environment task repairs its committed lock/manifest pair.
- 2026-09-10 deployment smoke: the mini PC served the revised login but its browser tab still identified the page as a development interface, so delivery returned to build for a final product document title and description.
- 2026-09-10 build red: command=`.\02_Tools\env\with-tools.ps1 npm.cmd --prefix .\05_Software\nilo_gui run test:ci -- src/layout.test.ts`; exit=1; requirements=REQ-UX-40473e43; expected missing behavior=`index.html` still carries the development browser title and description instead of final NILO product metadata.
- 2026-09-10 build green: focused `npm run test:ci -- src/layout.test.ts`; exit=0; 23 tests passed. Full component suite then passed 434 tests, lint passed and the production build passed after unused signed-out development catalogue text was removed.
- 2026-09-10 deployment: promoted the rebuilt GUI to `/opt/nilo/gui` on `ddp-remote-linux`, retained the prior deployment at `/opt/nilo/gui.backup-e9236fc5-20260910T1425`, restarted the active `nilo.service`, and observed the live HTTPS page title `NILO`, version `0.2.0`, human credential fields, Show password and Sign in. The user independently confirmed the deployed login works.
- 2026-09-10 validate gate after final metadata correction: pinned-environment `check --strict --run`; exit=1; lint passed and candidate digest `f3c6c5ed1b38f2a1048b1cd54ac637382609dbe5e37683dbd37959ec81b0f62d` remained unchanged, while the configured test command stopped at the same pre-existing committed reconstruction-manifest SHA mismatch. Full GUI component tests, production build, the prior complete 38-test Chromium run, live mini-PC browser observation and the user's independent test are green.
- 2026-09-10 deliver approval: the user confirmed the mini-PC deployment works and explicitly instructed closing the task and committing the files; outcome is one local commit on `master`, excluding `09_Others/Preview.mp4`, with no push.
- 2026-09-10 deliver close-out: AEGIS report generated; the approved outcome is the local `master` commit carrying this closed journal, requirements, implementation, tests and documentation. Remote publication remains with the human.

## Rulings

- RULING-001 | date=2026-09-10 | decision=Write requirements as positive, timeless descriptions of the intended current interface; do not name or enumerate removed elements in requirement text. | reason=Requirements preserve the product contract, not migration history. | source=user | supersedes=none

## Handoff


current: done
status: ready
next: no further action; the human may inspect or publish the local master commit
must_read: .agentic/tasks/TASK-e9236fc5.md; .agentic/steps/6_deliver.md
blockers: none

## Stage report

stage: deliver
result: completed
current: done
changed: Deployed and live-verified the final product login, generated the AEGIS delivery report and prepared the approved local master commit.
evidence: 434 component tests, lint and production build passed; 38 Chromium E2E tests passed; live mini-PC page title and login composition verified; user confirmed the deployment works.
issues: The pre-existing reconstruction lock/manifest hash mismatch keeps the repository-wide strict test gate red; scoped GUI proof is green. Responsive red timing, DOM type addition and the strict-gate substitution remain recorded deviations.
next: no further action; inspect or publish the local master commit if desired.
needs_user: no - local commit authorized; push remains prohibited.
spec_compliance: disabled
implementation_quality: disabled
review_round:

## Delivery report (2026-09-10)

- stage: deliver; type: change; path: full; branch: master
- requirements:
  - REQ-UX-40473e43 - Present the product sign-in surface
  - REQ-FUNC-5d80ae6c - Offer one human credential submission before sign-in
  - REQ-UX-8a1ddb96 - Show the project version before sign-in
  - REQ-UX-10182ba0 - Reveal and hide the sign-in password
  - REQ-UX-fdb7c574 - Scale the sign-in surface across supported displays
  - REQ-FUNC-a6c14c23 - Sign in to the browser interface with a username and password
  - REQ-SEC-66b84b2c - Keep browser credentials out of durable storage and URLs
  - REQ-UX-188a4860 - Identify the GUI as simulation in every state
  - REQ-UX-66636ac9 - Render distinct authenticated resource states
- deviations: none recorded
- explicit review: disabled; existing validation remains required
- diff vs task base c996736: 31 file(s) changed, 4113 line(s) added; 1 file(s) not content-scanned
- checks: errors=0 warnings=24 notes=1
- checklist: 3/6 done; open: journal and report included in the delivery commit; agreed outcome delivered, normally a PR; close-out committed after the outcome succeeded, or carried by the delivery commit where nothing could fail after it
