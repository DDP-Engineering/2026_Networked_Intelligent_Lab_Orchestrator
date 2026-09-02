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
Keep each log line to one sentence.

Stage report is one replaceable latest snapshot, not an evidence store.
Use plain one-line fields with no fences or bullets inside that section.
stage is the reported stage; current must match the journal header after any
transition. result is in_progress | completed | paused | blocked | skipped.
Keep all observations and decisions in the append-only Log as well.
Leave unknown fields blank; summary prints not recorded, never an assumed pass.
Refresh after work or decisions even if current has not changed.
-->

# TASK-a1b5a675 — Apply the saved GUI design direction

stage: deliver
type: change
path: full
requirements: REQ-UX-5503457a, REQ-UX-f300b346, REQ-UX-30821068, REQ-UX-6da44516, REQ-UX-0697c0ab, REQ-UX-2d00dbdb, REQ-UX-adbe5465, REQ-UX-811bdde9, REQ-DATA-1a14a1dc, REQ-NFR-b3f90f07, REQ-TEST-842f1943, REQ-UX-0018c647, REQ-UX-188a4860, REQ-SEC-ed66f6ed, REQ-SEC-66b84b2c, REQ-UX-66636ac9, REQ-UX-c4a0c9e7, REQ-UX-c7a277c5
parent:
branch: master
base: c39bf1ea4ebf4ac5f34072cbad30657aea7af553
opened: 2026-09-02

## Done checklist (current stage)

- [x] diff reviewed against scope, requirements, design, error paths, portability, security, and unrequested work
- [x] knowledge pages updated for every materially changed component
- [x] requirements re-read: status, prose, verification, paths, and links true
- [x] manual or inspection verification completed and recorded where applicable
- [ ] final evolution report refreshed after the last candidate edit — not run; no anchor exists (DEVIATION recorded under build)
- [x] final `check --strict --run` result recorded truthfully after all edits
- [x] remaining warnings and pre-existing failures explained
- [x] `stage: deliver`

## Log

- 2026-09-02 frame: scope is the saved GUI design direction in `00_Planning/GUI_Design/` applied to the existing interface - one compact 44-48px top row carrying the wordmark, the open page tabs and the network, simulation, theme, language and user controls; a labelled left sidebar whose lower cluster is Network, Plugins, Authentication, Settings, About; a central workspace freed of the decorative grid, the oversized connection panel and the card nesting; Light/Dark/System themes over shared semantic tokens; a centralized string catalogue with a compact language selector; and an honest Network summary and page.
- 2026-09-02 frame: explicitly out of scope - VPN provisioning, network configuration changes, new telemetry collectors, host networking changes, building a logic analyzer or camera integration, a plugin embedding or loading system, a replacement frontend framework, development-script redesign, and a version bump.
- 2026-09-02 frame: full path chosen because the work adds externally observable behavior (themes, languages, tabs, a Network page), a new persisted browser preference format, and changes to screens whose authentication, authorization and safety behavior must survive unchanged - all of which the quick path forbids.
- 2026-09-02 frame: no child tasks - the shell, the tokens and the string catalogue are one substitution; a theme without the shell, or a shell without the tokens, cannot be validated on its own.
- 2026-09-02 frame: components touched are the whole of `05_Software/nilo_gui/src` and its tests, the browser suite under `e2e/`, and the planning, knowledge and delivery documents that record the interface. Bench Core is deliberately untouched: this task adds no route and changes no authorization.
- 2026-09-02 frame: two facts settled by reading the repository rather than assumed. Nothing anywhere documents a supported-language set, so the languages were asked for rather than chosen; the answer selected English with German and Romanian, and all three ship as complete catalogues. Bench Core exposes no network interface, address, link-speed or traffic data on any route, so every address and every RX/TX figure in the reference images is demo data that this task must not reproduce as if measured.
- 2026-09-02 frame: this work stacks on the uncommitted TASK-a86b32ec candidate in the same working tree. That task remains at stage deliver awaiting a human decision; nothing here commits, stages or reverts it, and its authentication, authorization, CSRF and token behavior is preserved rather than rewritten.
- 2026-09-02 frame: expected requirements - the single compact top row; tab activation, closing, keyboard operation and active-tab fallback; the two-cluster sidebar with Network in the lower cluster; permission-based navigation visibility that never substitutes for server-side authorization; a workspace that uses the available width and height without page-level scrolling; Light/Dark/System over one set of semantic tokens with no second component tree; a System theme that follows and reacts to the device preference; persisted theme and language selections that carry no credential; a centralized catalogue translated into the three agreed languages with technical identifiers left alone; an honest compact network summary and Network page; and a test requirement covering the new shell behaviors.
- 2026-09-02 specify: wrote 11 requirements - one row, tab operation, the two navigation clusters, the full-bleed workspace, three themes over one token set, the System theme following the device, the language selector, honest network reporting, persisted preferences carrying no credential, one message catalogue per language, and the browser proof.
- 2026-09-02 specify: seven existing requirements are carried on this task's line because it changes the files they own or the screens they describe - REQ-UX-0018c647 and REQ-UX-c4a0c9e7 (single viewport), REQ-UX-188a4860 (simulation identity and the boundary statement), REQ-SEC-ed66f6ed and REQ-SEC-66b84b2c (credentials never in durable storage, which persisted preferences sit beside), REQ-UX-66636ac9 (the five distinguishable screens) and REQ-UX-c7a277c5 (the administration navigation entry).
- 2026-09-02 specify: no spec written. The design note and the two reference images are the buildable description of the layout, and a spec restating them would be a third source to keep in step.
- 2026-09-02 frame: the prompt approves this scope in advance, so the frame checkpoint is logged rather than asked; the one genuinely missing decision, the language set, was asked for separately as the prompt directs.

- 2026-09-02 design: recorded the decisions and test seams in `## Design`; the knowledge diagram is revised at validate rather than design because this task replaces the component set wholesale and an intended-shape picture would have been rewritten twice.
- 2026-09-02 build: DEVIATION build - the interface was implemented before its tests, as in the preceding slice, rather than test-first. The cost is that the component tests were written against the built behavior; they are honest about what they observe, but they are not a red-then-green record. The Bench Core half of this task is empty, so no red was available there either.
- 2026-09-02 build: implemented the token layer, the preference store, the theme and language providers, the shell grid with its top row, tab strip and navigation, the Bench, Network, About, integration and not-yet-available pages, and the three message catalogues; `App.css`, `StatusPanel.tsx` and `InventoryPanel.tsx` were removed, and `Resource<T>` moved to `api/resource.ts`.
- 2026-09-02 build: `npm run test:ci` 254 tests green in 13 files, `npm run lint` clean at `--max-warnings 0`, `npm run format:check` clean, `npm run build` exit 0.
- 2026-09-02 build: `npm run test:e2e` exit 0 - 19 browser tests passed in 27.1s against real Bench Core instances under WSL2, including the seven new shell tests and the eleven pre-existing ones.
- 2026-09-02 build: four real defects were found by running the thing rather than by reading it. The mock seeded no catalogue problem, but: `e2e/shell.spec.ts` used `document` and `window` inside `page.evaluate`, which `test:e2e` does not type-check and the production build does, so it passed a browser run while breaking `npm run build`; the navigation buttons lost their accessible name entirely on a narrow viewport once the label span was hidden, which the screenshot pass caught and an `aria-label` now fixes; the Disable and Enable button kept the `secondary` class the new stylesheet no longer defines, so it rendered unstyled in both themes; and `components/WorkspaceHeader.tsx` was left orphaned when `TopRow` replaced it, and is now deleted.
- 2026-09-02 build: the two lint rules this repository enforces shaped two designs rather than being suppressed - `react-refresh/only-export-components` moved the contexts, hooks and constant lists out of both provider files into `theme/themes.ts` and `i18n/languages.ts`, and `react-hooks/refs` moved the menu's focus restoration out of the render path into an effect that runs after the panel closes.
- 2026-09-02 build: DEVIATION build - no repository anchor was recorded before implementation, so `aegis evolution` cannot describe this task's starting point and was not run. Taking one now would report an empty change set and would be worse than none. The change record for this task is `git diff --stat` against base c39bf1e, listed in the stage report; the cost is that there is no timestamped fingerprint of the tree before the work began.
- 2026-09-02 validate: reviewed the diff against the frame scope - no VPN provisioning, no network configuration, no telemetry collector, no host networking change, no logic analyzer or camera integration, no plugin loading, no framework replacement, no development-script change and no version bump. Bench Core is byte-for-byte untouched: `git diff --stat c39bf1e -- 05_Software/nilo_assistant` reports nothing beyond the preceding task's own changes.
- 2026-09-02 validate: the preceding task's behavior is preserved rather than rewritten. `AuthenticationPage`, `AdministrationForms` and `Modal` kept their logic and gained only class names and catalogue lookups; `App` kept its five-phase shell, its credential handling and its restoration; and the whole of TASK-a86b32ec's Go candidate is unchanged. Its browser suite still passes with the two assertions that named the removed screen updated.
- 2026-09-02 validate: Inspection of the built interface, captured with a real Chromium against the built mock bundle at 1440x810 and 820x780. Nine screenshots cover the sign-in screen, Bench and Authentication in both themes, the Network page, a dialog, the network summary popover and a narrow viewport. Observed - one 46px row carrying the wordmark, the tabs and all five control groups; the lower navigation cluster in the agreed order with Network in it; the workspace filling the rest with no decorative background; the boundary strip and the SIM badge on every screen; the Network page showing fifteen "Not reported" cells and no illustrative value.
- 2026-09-02 validate: three of the four defects listed under build were found by that capture pass and by the production build rather than by any test, which is the honest reason this task ran the screenshots before the gate rather than after.
- 2026-09-02 validate: knowledge refreshed - `.agentic/knowledge/gui-foundation.md` redrawn around the shell, the token layer, the catalogue and the page set; `.agentic/knowledge/bench-core-foundation.md` amended only where it described what the interface shows. Requirements re-read: the eleven new ones and the seven carried ones each state a behavior this candidate actually has, and their verification lines name observations that were made.
- 2026-09-02 validate: first full gate, `check --task TASK-a1b5a675 --strict --run`, exit 0 - `test ok (81.0s)`, `lint ok (5.8s)`, `summary: errors=0 warnings=60 notes=2`, `result: WARN`, `strict: ok`. Seventeen of those warnings were `stale-paths`: ten requirements still named `App.css`, `StatusPanel.tsx`, `InventoryPanel.tsx` or `WorkspaceHeader.tsx`, which this task deleted.
- 2026-09-02 validate: repointed those ten requirements onto the real successors - `StatusPanel.tsx` and `InventoryPanel.tsx` to `components/pages/BenchPage.tsx`, `WorkspaceHeader.tsx` to `components/shell/TopRow.tsx`, and `App.css` to `styles/tokens.css` and `styles/shell.css` - and claimed seven files this task introduced that no requirement owned: `Menu.tsx` and `Icon.tsx` on REQ-UX-5503457a, `SimplePages.tsx` and the GUI README on REQ-UX-30821068, `main.tsx` and `test/render.tsx` on REQ-UX-0697c0ab, and `api/resource.ts` on REQ-UX-66636ac9. No requirement anywhere now names a file that does not exist.
- 2026-09-02 validate: final gate after those edits, `check --task TASK-a1b5a675 --strict`, `summary: errors=0 warnings=35 notes=2`, `result: WARN`, `strict: ok`. The full `--run` gate was not repeated for them because they change only requirement metadata; no source, test or configuration file was touched after the run that produced `test ok` and `lint ok`.
- 2026-09-02 validate: DEVIATION validate - thirty-five warnings accepted, in four groups. Seventeen `secret-in-diff`: fifteen are the word "password" in field names, labels and throwaway test fixtures on screens whose whole subject is a password, and two are the design PNGs, which are binary and cannot be scanned. Fourteen `uncovered-source`: eight are TASK-a86b32ec's own files, which sit uncommitted in this same working tree and are covered by that task's requirements rather than this one's, and the remaining six are the development mock and the earlier browser suites, owned by requirements from earlier slices. Three `unticked-deliver`: the deliver checklist is unticked because this repository's convention is that the human commits. One `outside-layout`: `CHANGELOG.md` sits outside the source, test and docs globs, and the prompt requires an entry in it. The cost is that a genuinely new warning in any of these categories would have to be found by reading the list rather than by the count changing.
- 2026-09-02 validate: the human committed cc24621 (`03_Hardware/NILO_case_v1.skp` and `.stl`) onto master while this task was in validate. It touches no file this task touches, and the recorded base c39bf1e is left alone as the contract requires, so this candidate's diff is measured across that unrelated commit.
- 2026-09-02 validate: no unrequested work found in the diff. The one thing built beyond the literal prompt is the Instruments page, and it is built only from integrations Bench Core actually reports; the logic analyzer, USB camera and flash tools the reference image names were deliberately not created.
- 2026-09-02 validate: an independent read-only review arrived at `09_Others/nilo-review-20260902` after this journal reached deliver, raising three P2 findings and one P3. All four were reproduced against the code rather than accepted on the reviewer's word, and all four were real; the journal is reopened at validate rather than delivered over them.
- 2026-09-02 validate: P2 open tabs lost on refresh. `App.loadBench` sets the connecting phase, which unmounts `AppShell`, and `useWorkspace(['bench'])` re-initialised on remount, so Refresh closed every tab but Bench. The workspace now lives in `App`, which stays mounted across the phase change, and is passed in; it resets on sign-out so the next person does not inherit the previous one's tabs.
- 2026-09-02 validate: P2 a failed device inventory read as an empty one. `AppShell` collapsed the device `Resource` to `[]` before handing it to `IntegrationPage`, which then said the integration reports no devices - a claim about the bench nobody had established. `IntegrationPage` now takes the `Resource` itself and distinguishes an unreadable inventory from a successfully empty one, with a new `integration.devicesUnavailable` string in all three catalogues.
- 2026-09-02 validate: P2 dialogs did not contain keyboard focus. `Modal` set initial focus and handled Escape but never confined Tab, so a keyboard user tabbed onto the page behind a dialog that was supposed to be covering it - `aria-modal` names the condition for a screen reader but moves no focus. Tab and Shift+Tab now wrap within the panel, and the initial-focus query skips disabled controls instead of landing on one.
- 2026-09-02 validate: P3 the instrument page carried a page-title row repeating its own tab, which the agreed layout in `00_Planning/GUI_Design/README.md` forbids above an instrument and which cost a row of workspace. The head is gone; the identifier it carried is a fact in the grid, so no reported information was lost.
- 2026-09-02 validate: four regression tests added and one rewritten - tab survival across a refresh and the unreadable-inventory wording drive the whole `App`, as the review asked; the instrument page is asserted to have no `.page-head` and no heading repeating its tab; and the dialog focus boundary is checked in both directions. The focus test was confirmed to fail against the unfixed `Modal` before the fix was restored, so it discriminates rather than decorates.
- 2026-09-02 validate: the focus boundary is also proved in a real browser, `e2e/authentication.spec.ts`, because jsdom cannot reproduce the wrap from the end of the document back to the top that carried focus onto Close Bench. Component tests are now 258 in 13 files and the browser suite 20.
- 2026-09-02 validate: inspection of the two visible fixes at 1440x810 on the `malformed-devices` mock - the instrument page states that the device inventory could not be read and which fact that leaves unknown, carries no title row, and reads correctly without one.
- 2026-09-02 validate: gate after the review fixes, `check --task TASK-a1b5a675 --strict --run`, exit 0 - `test ok (94.9s)`, `lint ok (5.1s)`, `summary: errors=0 warnings=40 notes=2`, `result: WARN`, `strict: ok`. Also `npm run test:ci` 258 green in 13 files, `lint` clean at `--max-warnings 0`, `format:check` clean, `build` exit 0, and `test:e2e` 20 browser tests green in 28.4s against real Bench Core under WSL2.
- 2026-09-02 validate: the count rose from 35 to 40 and every one of the five is traceable and none is from the changed code. Four are `secret-in-diff` on the review's own unscannable binaries - `09_Others/Preview.mp4` and the review PNGs, which arrived in the working tree with the review - and one is this journal's own deliberately unticked evolution line, which the earlier DEVIATION already accounts for. A line-by-line diff of the two warning lists shows nothing dropped and nothing new from the fixes.
- 2026-09-02 validate: `npm run test:e2e` must be run through `02_Tools/env/with-tools.ps1`, which sets `PLAYWRIGHT_BROWSERS_PATH` to the repository-local runtime; run bare it looks in a per-user cache that this repository deliberately never writes, and fails to launch. On a Constrained Language Mode host the wrapper must be given `npm.cmd`, because `npm.ps1` cannot run there.

## Design

**One grid, four cells (REQ-UX-5503457a, REQ-UX-6da44516).** The shell is a
single CSS grid: `grid-template-columns: var(--nav-width) 1fr` and
`grid-template-rows: var(--row-height) 1fr auto`. The wordmark sits in the
navigation column of the header row, the controls in the workspace column of the
same row, the navigation below the wordmark, the workspace below the controls,
and a 24px boundary strip under the workspace only. That is exactly the shape
both reference images show, and it is why there is nowhere for a second tab
strip or a page-title row to appear. Rejected alternative: a full-width header
above a two-column body, which pushes the wordmark out of the navigation column
and makes the sidebar start below the header - neither reference does that.

**Tokens are the only place a colour exists (REQ-UX-0697c0ab).**
`styles/tokens.css` defines one semantic set - surface, text, border, accent,
state, focus - three times: as light on `:root`, as dark on
`:root[data-theme='dark']`, and as dark again on `:root[data-theme='system']`
inside a `prefers-color-scheme: dark` query. Components style through the tokens
and declare no literal colour, which is what makes a second component tree
impossible rather than merely discouraged. The System branch is CSS, so it
follows the device without script (REQ-UX-2d00dbdb); `useSystemColorScheme`
exists only to report the resolved value to code that needs it, and it is the
seam the reactivity test drives.

**Preferences are a two-key store (REQ-DATA-1a14a1dc).** `preferences/store.ts`
owns the only `localStorage` access in the interface and exposes exactly
`nilo.theme` and `nilo.language`. Every credential rule stays as it was: the
existing tests that asserted `setItem` was never called are narrowed to assert
the written keys are exactly these two and that no written value contains a
password, a request-forgery token or a token secret. Rejected alternative:
leaving those tests as "never writes" and keeping preferences in memory, which
would have failed the persistence this task is asked for.

**The catalogue is typed, not looked up (REQ-NFR-b3f90f07, REQ-UX-adbe5465).**
`i18n/en.ts` is the key set; `de.ts` and `ro.ts` are declared as
`Record<MessageKey, string>`, so a missing key is a compile error and the build
is the test. No dependency is added: the repository pins its dependency graph
exactly, and a flat typed dictionary with one `useTranslate()` hook is the whole
requirement. Identifiers, permission names, protocol names and inventory values
are rendered from data and never routed through the catalogue.

**Navigation is data, authorization is not (REQ-UX-30821068).** `pages.ts` is one
list of page descriptors, each with its cluster, its icon, and the permission it
needs; the sidebar renders the upper cluster from that list plus the
integrations the backend actually reported, and the lower cluster from the five
fixed entries. `mayAdminister` and its siblings decide visibility only - every
request is still authorized by Bench Core, and the Authentication page is
unchanged in that respect.

**Network reports nothing it was not told (REQ-UX-811bdde9).** Bench Core serves
no network data, so `NetworkSummary` and `NetworkPage` render the real structure
around an explicit unavailable state, and the only fact either states is that
Bench Core answered over HTTP - labelled as that and nothing else. The
illustrative addresses and traffic figures from the references appear nowhere in
the source, which a test asserts by searching for them.

**Test seams.** The stylesheets are read from disk for the grid, token-parity and
no-literal-colour assertions, as `layout.test.ts` already does. `AppShell` is
rendered over the existing `BenchCoreClient` stub for the tab, navigation,
theme, language and network tests. `preferences/store.ts` is the single seam for
the storage assertions. The browser suite gains `e2e/shell.spec.ts` for the
reload-survival proof (REQ-TEST-842f1943).

**Knowledge architecture.** `gui-foundation.md` gains the shell, the token
layer, the catalogue and the preference store; `bench-core-foundation.md` is
untouched, because this task changes no backend behavior.

## Rulings

- RULING-001 | date=2026-09-02 | decision=Work on the current `master` checkout, and leave every change uncommitted and unstaged | reason=The prompt forbids staging, committing, pushing, branching and worktrees, and requires unrelated existing changes to be preserved | source=user | supersedes=none
- RULING-002 | date=2026-09-02 | decision=Ship English, German and Romanian as complete catalogues | reason=No supported-language set was documented, so the prompt required asking; the answer selected both "English + Romanian" and "English + German + Romanian", and the superset is the only reading that satisfies both | source=user | supersedes=none
- RULING-003 | date=2026-09-02 | decision=The Network summary and Network page report an explicit unavailable state instead of the addresses, link speeds and RX/TX figures shown in the reference images, and the reachability of Bench Core over HTTP is never presented as VPN or appliance network health | reason=Bench Core exposes no network telemetry on any route, the design note records every illustrated value as demo data, and the prompt forbids inventing live telemetry or inferring VPN health from an HTTP connection | source=user | supersedes=none
- RULING-004 | date=2026-09-02 | decision=Keep the physical-equipment boundary statement on every screen, as a slim strip at the foot of the workspace column, even though neither reference image shows one | reason=REQ-UX-188a4860 requires that statement on every screen and this task may not weaken a safety behavior to match an illustration; a 24px strip in the workspace column costs half the removed 47px footer and leaves the single-row header rule intact | source=requirement | supersedes=none

## Handoff

current: deliver
status: ready
next: Present the candidate for a human delivery decision. Nothing is staged, committed or pushed, and this working tree also carries TASK-a86b32ec's undelivered candidate, so the two are decided together or the newer one is separated first.
must_read: .agentic/tasks/TASK-a1b5a675.md, .agentic/steps/6_deliver.md, 00_Planning/GUI_Design/README.md
blockers: none

## Stage report

stage: validate
result: completed
current: deliver
changed: 66 files against base c39bf1e - the GUI source replaced around a shell, token, catalogue and page structure; `App.css`, `StatusPanel.tsx`, `InventoryPanel.tsx` and `WorkspaceHeader.tsx` deleted; 18 requirements written or repointed; and CHANGELOG.md, 00_Planning/TODO.md, 00_Planning/GUI_Design/README.md, 06_SoftwareDocu/Development_and_Simulation_Flow.md, 07_Delivery/NILO_CLI_and_Authentication.md, both knowledge pages and the GUI README updated. Bench Core unchanged.
evidence: `npm run test:ci` 258 tests green in 13 files; `npm run lint` clean at --max-warnings 0; `npm run format:check` clean; `npm run build` and `npm run build:mock` exit 0; `npm run test:e2e` 20 browser tests green in 28.4s against real Bench Core instances under WSL2, 7 of them new; nine screenshots of the built bundle at 1440x810 and 820x780 in both themes; `check --strict --run` errors=0 warnings=60 notes=2 strict ok, then errors=0 warnings=35 notes=2 after the requirement paths were repointed, and errors=0 warnings=40 notes=2 strict ok after the four independent-review findings were fixed.
issues: 40 accepted warnings in four groups, five of them the review artifacts' own binaries and this journal's unticked evolution line - password literals on password screens and two binary PNGs, files belonging to the undelivered TASK-a86b32ec candidate in the same tree, an unticked deliver checklist, and CHANGELOG.md sitting outside the layout globs. No anchor was taken before implementation, so no evolution report exists for this task.
next: A human decides delivery. Bench Core still reports no network telemetry, so the Network page shows nothing measured; there is no plugin loading system, no Settings page and no instrument control surface, and each says so rather than showing an empty screen.
needs_user: delivery decision only
spec_compliance: disabled
implementation_quality: disabled
review_round:
