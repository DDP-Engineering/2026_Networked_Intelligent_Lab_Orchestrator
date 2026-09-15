<!--
Copyright (c) 2026 DDP Engineering
This software is licensed under the PolyForm Noncommercial License 1.0.0.
Commercial use requires a separate commercial license.
See the LICENSE.txt in the .agentic folder for full license details.
SPDX-License-Identifier: LicenseRef-PolyForm-Noncommercial-1.0.0
-->


# TASK-29673ddb - Give the Activity page its screen back

stage: done
type: change
path: full
interaction: autopilot
requirements: REQ-UX-dd4b2c16, REQ-UX-3e01a5b6, REQ-UX-c5976f3b, REQ-UX-f8c58c4c, REQ-UX-ba46f6d3, REQ-UX-0291ba55, REQ-UX-7781cc4e
parent:
branch: ddp/nilo-activity-page-layout
base: 0dbdbeceb7804b93e70b17667cd7e73ce42164e5
opened: 2026-09-15
source: Gitea issue dpasare/2026_Networked_Intelligent_Lab_Orchestrator#20

## Done checklist (current stage)

- [x] validated candidate confirmed unchanged; otherwise returned to validate
- [x] validation evidence, diff, exclusions, and outcome presented
- [x] delivery confirmed by the human
- [x] journal and report included in the delivery commit
- [x] agreed outcome delivered, normally a PR
- [x] close-out committed after the outcome succeeded, or carried by the
      delivery commit where nothing could fail after it

## Frame

Problem: The Activity page spends its first screen on text instead of on
activity. Above the table stand a title, a subtitle, a health sentence, one
retained-entry count per category, a scope chip row, a filter row and a
full-width More filters disclosure, so the table starts low and, on a bench
panel, near or past the fold. Two controls are also placed against their
frequency of use: Export sits among the filters, and the retention setting sits
below the table, so an administrator scrolls past every row to reach the one
control that decides how long those rows are kept. (STATED)

Desired outcome: The records occupy the first screen. The page keeps one header
line, one filter line, an administrator retention disclosure above the filters,
and a smaller default page, while every existing Activity behavior - filtering,
export, pagination, sorting, the details modal - is untouched. (STATED)

Actors: Viewer or Operator holding `activity.read` reading and filtering the
history; Administrator additionally holding the retention control; both on a
1024x600 bench panel and on a desktop display. (OBSERVED
`05_Software/nilo_gui/src/components/shell/pages.ts`, `ActivityPage.tsx` props
`canExport` and `canManageRetention`)

Primary scenario: an operator opens the Activity tab on a 1024x600 panel ->
the page renders header line, retention disclosure (administrator only),
filter line and table -> the first table row is visible without scrolling.

Examples: on 1024x600 the page currently renders title, subtitle, chips,
health sentence, three retained counts and the filter drawer summary before the
table (OBSERVED `ActivityPage.tsx` render body, `ActivityHealthBanner.tsx`).
After the change the same viewport shows the title with Export, the collapsed
retention disclosure for an administrator, the collapsed filter drawer and the
first record.

Counterexamples: an appliance that cannot write its audit log, or a category
whose retained window has a gap, still reports that on the page - the removal
covers the everything-is-fine sentence and the per-category counts only.
(STATED; OBSERVED the degraded notice and gap lines in
`ActivityHealthBanner.tsx`)

In scope:
- Header: page title and Export on one line, Export top right; the subtitle
  removed.
- Standing labels: the `activity.health.ok` sentence and the
  `activity.retained` counts removed from the standing layout; the degraded
  notice and the retained-window gap lines stay visible.
- Filters: the three scope chips (All, Audit, Events), the six filter fields
  and the More filters disclosure share one wrapping line; More filters no
  longer occupies a full-width row.
- Retention: the administrator-only retention disclosure moves above the
  filters and below the header, still collapsed, still administrator-only.
- Page size: 10 added as the smallest choice and used as the default in place
  of 25; 25, 50, 100 and all remain.
- The three message catalogues (`en`, `de`, `ro`) lose the removed keys
  together, and the component and browser tests that assert the old layout and
  the old default are updated.

Out of scope (STATED by the issue):
- What the table shows: columns, sorting, the details modal, export formats.
- The Activity API, its filters, its pagination contract, and the retention
  service behind the control.
- Any other page's header or filter layout.

Constraints:
- One layout serves 800x480 up to a desktop; no operator text below 14px and
  every interactive target at least 44x44 (OBSERVED
  `.agentic/context/project.md`, `e2e/field-display.spec.ts`).
- `REQ-UX-f30891b4` keeps Activity usable at 1024x600 without document-level
  horizontal scrolling and keeps time, outcome, action, subject and actor in
  the row. Preserved, not changed. (OBSERVED
  `.agentic/requirements/ux/REQ-UX-f30891b4.toml`)
- `REQ-UX-b75fbbc3` governs page-scoped View activity buttons in a
  left-aligned title group on other pages. Export on the Activity page is a
  different action class, so a top-right Export does not restate or contradict
  it. (OBSERVED `.agentic/requirements/ux/REQ-UX-b75fbbc3.toml`; INFERRED that
  the two patterns coexist)
- A removed catalogue key must go from `en`, `de` and `ro` together;
  `messages.test.ts` and the `de` and `ro` key-set types enforce it. (OBSERVED
  `05_Software/nilo_gui/src/i18n/messages.test.ts`)
- Appliance browser evidence comes from `02_Tools/tests/appliance/check.sh`
  under WSL2; direct host browser runs are not acceptance evidence. (OBSERVED
  `.agentic/context/environment.md` section Verification)

Success signals:
- Title and Export on one line, Export top right.
- Subtitle, health sentence and retained counts gone from the standing layout.
- A degraded appliance or a retained-window gap is still reported on the page.
- Scope chips, filter fields and More filters occupy one wrapping line.
- Retention appears above the filters, below the header, administrator only.
- The table defaults to 10 records and offers 10, 25, 50, 100 and all.
- At 1024x600 the first table row is visible without scrolling.
- Filtering, export, pagination, sorting and the details modal behave as today.

Evidence:
- STATED: the issue text, including its scope and exclusions.
- OBSERVED `05_Software/nilo_gui/src/components/pages/ActivityPage.tsx`: the
  header renders an `h2` plus `p.page-sub`; the chips are an `.activity-chips`
  group inside `header.page-head`; the filter form holds the six primary
  fields, a `details.activity-filters-more` and a `.form-actions` row that also
  carries the Export menu; `DEFAULT_ACTIVITY_QUERY.limit` is 25; the page-size
  select offers 25, 50, 100 plus all; the retention `details` is rendered last,
  after the pagination.
- OBSERVED `05_Software/nilo_gui/src/components/activity/ActivityHealthBanner.tsx`:
  `activity.health.ok` with a state dot when writable, a `notice-danger` with
  `activity.health.degraded` when not, one `activity.retained` span per
  category, and one `activity.gap` line per truncated window.
- OBSERVED `05_Software/nilo_gui/src/components/shell/useViewportClass.ts`:
  1024x600 classifies as `compact`, so `collapsible` is true and the whole
  filter form already sits inside a closed Filters drawer at the usual field
  size.
- OBSERVED `05_Software/nilo_gui/e2e/field-display.spec.ts`: at both field
  viewports the test reads the chips with the filter drawer closed, asserts the
  first record starts inside the viewport, and opens retention by its
  Activity retention summary text.
- OBSERVED `05_Software/nilo_gui/src/components/pages/ActivityPage.test.tsx:112`
  asserts `page: 2, limit: 25` on the second query.
- OBSERVED `05_Software/nilo_assistant/internal/httpapi/activity.go:157` and
  `internal/activity/activity.go:201`: the appliance parses any integer limit
  and refuses only a negative one or one above `MaximumPageSize`.
- INFERRED: a default of 10 needs no appliance change, so the issue's
  "Activity API out of scope" is achievable as written.
- UNKNOWN: whether the 1024x600 acceptance is already met today - the existing
  browser assertion is the weaker "row top is above the viewport bottom", which
  a row whose body is cut off still satisfies.

Assumptions:
- The scope buttons are the existing category chips (All, Audit, Events) and
  keep their current re-query-on-click behavior; only their placement changes.
- The filters-in-effect count, Apply filters and Clear filters stay in the
  filter form, which the issue confirms by leaving their behavior unchanged.
- Export stays a menu offering CSV and JSONL over the applied filter; only its
  position moves.

Unknowns:
- Whether the retained-entry counts are wanted anywhere else, for example
  inside the retention disclosure. Treated as removed, per the issue.

Decision-blocking questions:
1. At 1024x600 and 800x480 the filter form is already inside a closed drawer.
   If the scope chips join that one filter line, scope selection moves behind
   the drawer on exactly the bench panel this change is for, and the existing
   browser test that reads the chips with the drawer closed would have to be
   retired. Proposed: the chips stay outside the drawer at field viewports and
   share the one filter line wherever the filters are shown inline, so the wide
   viewport gets the single line the issue describes. Confirm, or ask for the
   chips inside the drawer at every viewport.

Proposed behaviors:
- B1 One header line: the `h2` and the Export menu in a single `.page-head`
  row, Export aligned to the top right; `activity.subtitle` and its three
  catalogue entries removed.
- B2 The standing health strip carries only conditions: the degraded notice and
  the per-category gap lines. `activity.health.ok` and `activity.retained` and
  their three catalogue entries are removed.
- B3 One filter line: chips, the six fields and the More filters disclosure lay
  out inline and wrap; `.activity-filters-more` stops being a full-width row.
- B4 The administrator retention disclosure renders above the filters and below
  the header, unchanged in content and permission.
- B5 The page-size control offers 10, 25, 50, 100 and all, and the page opens at
  10; `ActivityPageSize` gains 10 and `DEFAULT_ACTIVITY_QUERY.limit` becomes 10.

Path and reason: full.

| criterion | quick? | evidence |
|---|---|---|
| no new externally observable behavior | no | header, filter line, retention position and default page size all change what the operator sees |
| no new interface or data format | yes | no API, contract or export change; the appliance already accepts limit 10 |
| no new dependency | yes | React, the existing stylesheet and the catalogues only |
| an existing requirement already covers the behavior | no | REQ-UX-f30891b4 covers field readability, not the header line, the one-line filters, the retention position or the default page size |
| no security or data-handling change | yes | retention stays administrator-only; export scope unchanged |

Two criteria fail, so the full path applies and `specify` writes the new
requirement or requirements.

Children: none. The work is one page, its banner, three catalogues and their
tests - too tightly coupled to split.

## Requirement coverage

| Frame outcome | Atomic behavior | Requirement | Positive case | Boundary or failure | Verification | Design decision or no impact |
|---|---|---|---|---|---|---|
| Records occupy the first screen | the first record is rendered entirely inside the initial viewport at 1024x600 | REQ-UX-dd4b2c16 | administrator, disclosures closed, several records listed | the administrator case is the boundary: it is the only one that also renders the retention disclosure | browser test measures the bounding box of the first table row against the viewport bottom | design decides the standing vertical budget: header, conditional banner, retention summary, filter summary |
| One header line | the header carries the title and, for an exporter, Export at its right, and no other text | REQ-UX-3e01a5b6 | identity that may export sees title and Export on one line | identity that may not export sees the title alone | component test on both permissions; browser test at 1920 and 1024 for alignment, target size and overflow | design places Export in the page head and removes `activity.subtitle` from three catalogues |
| Fewer standing labels | only qualifying recording conditions are reported above the listing | REQ-UX-c5976f3b | audit writes failing, or a truncated retained window | healthy appliance with no truncated window reports nothing | component test over the three states | design reduces the banner to conditions and removes `activity.health.ok` and `activity.retained` |
| One filter line | scope controls, filter fields and More filters lay out as one wrapping line | REQ-UX-f8c58c4c | wide viewport puts them on one row | a narrow viewport wraps them, with More filters never alone on a row | browser test compares vertical offsets and then narrows; component test proves the queries are unchanged | design changes the filter form layout and drops the full-width More filters row |
| One filter line | scope selection stays outside the collapsed filter disclosure | REQ-UX-ba46f6d3 | 1024x600 with the filter drawer closed | 800x480, where targets and overflow are tightest | browser test reads and activates a scope control with the form hidden | design renders the chips outside the drawer (RULING-001) |
| Retention above the filters | the retention disclosure sits between header and filters, administrator only | REQ-UX-0291ba55 | administrator sees a collapsed disclosure before the filters | an identity that may not manage retention sees none | component test on both permissions, including that saving still reaches the callback | design moves the existing disclosure; its content and permission are untouched |
| A smaller default page | the listing offers 10, 25, 50, 100 and all, and opens at 10 | REQ-UX-7781cc4e | the first query carries a limit of 10 | selecting all carries the whole selection | component test over the first query and two size selections | design widens `ActivityPageSize` and the default query; the appliance already accepts a limit of 10 |

No spec is written. Each behavior is a placement or a default that its
requirement states completely; there is no schema, wire format, protocol, state
machine or migration for a spec to carry.

## Design

**Requirements coverage.** REQ-UX-3e01a5b6 header shape; REQ-UX-c5976f3b
banner reduction; REQ-UX-f8c58c4c and REQ-UX-ba46f6d3 the filter line and where
the scope controls render; REQ-UX-0291ba55 render order; REQ-UX-7781cc4e the
page-size type and default; REQ-UX-dd4b2c16 is the emergent outcome of the
other five and adds no separate implementation, only its own browser proof.

**Component responsibilities.** Everything stays inside the browser GUI
foundation. `ActivityPage.tsx` owns page composition and order,
`ActivityHealthBanner.tsx` owns what is said above the listing, `shell.css`
owns the layout, the three catalogues own the text, and `api/activity.ts` owns
the page-size type. Bench Core is untouched: it already accepts any limit up to
`MaximumPageSize`, so a default of 10 is a client decision
(`internal/activity/activity.go:201`).

**Page order (REQ-UX-0291ba55, REQ-UX-dd4b2c16).** header -> conditions banner
(often empty) -> retention disclosure (administrator, collapsed) -> scope and
filters -> table -> pagination. The standing vertical budget above the first
row therefore becomes: one header line, one scope-and-filter line (or the
scope line plus a closed drawer summary at field widths), and at most one
collapsed retention summary. Nothing else stands there while the appliance is
healthy.

**Header (REQ-UX-3e01a5b6).** `.activity-head` stops being a column and becomes
the ordinary `.page-head` row: `h2` first, the Export `Menu` last with
`margin-left: auto`. `.activity-head-top`, `.activity-head-text` and the
`.page-sub` paragraph go, and `activity.subtitle` leaves the three catalogues.
Export keeps its `.activity-export` trigger class and its CSV/JSONL actions
over the applied filter; only its position moves. This is deliberately not the
left-aligned `.page-title-actions` group of REQ-UX-b75fbbc3: that pattern
carries page-scoped *View activity* buttons on other pages, and Export is a
different action class the issue places top right.

**Banner (REQ-UX-c5976f3b).** `ActivityHealthBanner` keeps the
`notice-danger` degraded notice and the per-category gap lines and loses the
healthy-state paragraph and the retained-count row, so it renders nothing at
all on a healthy appliance with untruncated windows. `activity.health.ok` and
`activity.retained` leave the catalogues, and `.activity-banner-state`,
`.activity-banner-retention` and `.activity-banner-count` leave the stylesheet.
`state-dot`/`state-ok` stay: five other components use them.

**Filter line (REQ-UX-f8c58c4c, REQ-UX-ba46f6d3).** `.activity-filters` changes
from a `repeat(auto-fit, minmax(12rem, 1fr))` grid to `display: flex` with
`flex-wrap: wrap` and `align-items: flex-end`; each field becomes
`flex: 1 1 12rem; min-width: 0`; the actions group and the note lose
`grid-column: 1 / -1` and become ordinary wrapping items.

The More filters disclosure stops being a `<details>` whose open box would take
a row to itself, and becomes an ARIA disclosure button (`aria-expanded`) on the
line whose three fields, when expanded, join the same wrapping flow as the six
primary fields. The `moreOpen` state, its auto-open when a hidden filter
arrives in effect, and the accessible name `More filters` are unchanged, so the
existing `getByText('More filters').click()` in both the component and browser
tests still drives it. Forced by REQ-UX-f8c58c4c: a disclosure that expands
into a full-width block cannot satisfy "no control holding a row of its own" in
its open state.

Scope placement follows RULING-001 and the existing `useViewportClass` split,
so script and stylesheet keep agreeing about which shell is on screen. The
chips are one element rendered in one of two positions: inside the filter form,
first on the line, while the panel is inline (`wide`); above the closed drawer,
outside it, while the panel is collapsed (`compact`, `constrained`). Rejected
alternative: chips always outside the form, joined to the filter line by CSS
alone - two flex containers cannot wrap as one line without absolute
positioning, and the requirement is about layout order as well as appearance.

**Page size (REQ-UX-7781cc4e).** `ActivityPageSize` becomes
`10 | 25 | 50 | 100 | 'all'`, `DEFAULT_ACTIVITY_QUERY.limit` becomes 10, the
select offers `[10, 25, 50, 100]` plus all, and the two `?? 25` fallbacks in
the page become `?? 10`. No contract, parser or export change.

**Failure, security, concurrency, migration.** None of these change. The
retention control keeps its `canManageRetention` gate and its save path;
`applyQuery` still owns every filter change and its generation guard; the
export still sends the applied filter; there is no persisted layout state to
migrate, and a browser holding an older preference is unaffected because the
page size is not persisted.

**Test seams.**
- `src/components/pages/ActivityPage.test.tsx` - header contents on both export
  permissions, banner in its three states, retention position and permission,
  the unchanged filter queries, and the page-size default and choices.
- `src/components/activity/ActivityHealthBanner.tsx` is exercised through the
  page rather than a new file, as today.
- `e2e/field-display.spec.ts` - the one-line layout and its wrapping, scope
  reachable with the drawer closed, and the first row inside a 1024x600
  viewport.
- `src/i18n/messages.test.ts` needs no edit; it derives its key set.

**Expected files.** `src/components/pages/ActivityPage.tsx`,
`src/components/activity/ActivityHealthBanner.tsx`, `src/api/activity.ts`,
`src/styles/shell.css`, `src/i18n/{en,de,ro}.ts`,
`src/components/pages/ActivityPage.test.tsx`, `src/layout.test.ts`,
`e2e/field-display.spec.ts`. `src/layout.test.ts` reads the shipped stylesheet
and asserts the Activity head and filter rules directly, so it is the third
test seam the changed CSS has to satisfy.

**Risks.** The browser suite runs only in the appliance stage under WSL2, so
the one-line and first-row assertions cannot be proven from the Windows host;
they are proven by `check --strict --run`, which includes that stage. A flex
filter line at 800x480 risks a field narrower than its `datetime-local`
intrinsic width - the existing `min-width: 0` rule already covers that and is
kept.

**Open decisions.** None. No architecture drawing is owed: this is a
behavior-and-layout change within one existing component.

## Log

- 2026-09-15 frame: framed Gitea issue #20 as a change to the Activity page
  layout. Full path: the header line, one-line filters, retention position and
  a 10-record default are observable behavior no active requirement covers.
  Branch `ddp/nilo-activity-page-layout` was created by the user from
  `origin/master` before framing, so no task branch was opened here. Scope
  awaiting user confirmation; one decision-blocking question recorded about the
  scope chips at field viewports, where the filter form is already a drawer.
- 2026-09-15 frame: scope accepted by the user with the proposed chip
  placement; see RULING-001. Advanced to specify.
- 2026-09-15 specify: created 7 UX requirements, revised 0, reused 0, retired 0.
  No spec: every behavior is a placement or a default the requirement states in
  full. REQ-UX-f30891b4 stays as it is - Activity field readability and the row
  columns are unchanged by this task.
- 2026-09-15 specify: user accepted the requirements and switched this task to
  autopilot through to deliver, with delivery stopping before any Git change;
  see RULING-002. Advanced to design.
- 2026-09-15 design: note written under `## Design`; no spec and no
  architecture drawing owed. Advanced to build.
- 2026-09-15 build red: REQ-UX-3e01a5b6, REQ-UX-c5976f3b, REQ-UX-f8c58c4c,
  REQ-UX-0291ba55, REQ-UX-7781cc4e; command=`npm run test:ci --
  src/components/pages/ActivityPage.test.tsx` in `05_Software/nilo_gui`;
  exit=1; 6 of 23 failed. Expected missing behavior: the header still renders
  the subtitle paragraph and no Export, the banner still renders the healthy
  sentence and the retained counts, the chips are not part of the filter form
  on a wide display, the retention disclosure still follows the filters, and
  the listing still opens at a limit of 25. REQ-UX-ba46f6d3 is green as
  written - the chips are already outside the drawer, and its test guards
  RULING-001 against the change the other requirements make.
- 2026-09-15 build: `src/layout.test.ts` reads the shipped stylesheet and
  asserted the old `.activity-head-top` column and the filter grid directly, so
  it is a third test seam the design's file list had not named. It was updated
  to assert the wrapping filter line instead - in scope, since the frame put the
  tests that assert the current layout inside it - and added to `## Design`.
- 2026-09-15 build green: all seven requirements; command=`npm run format:check
  && npm run lint && npm run build && npm run test:ci` in `05_Software/nilo_gui`;
  exit=0; 494 component tests across 35 files, Prettier and ESLint clean, and
  the production build emitted. The browser assertions for REQ-UX-dd4b2c16,
  REQ-UX-f8c58c4c and REQ-UX-ba46f6d3 are written but unproven here: the
  Playwright suite runs only in the appliance stage, which validate reaches
  through `check --strict --run`.
- 2026-09-15 build: `evolution` reports 10 modified files and no additions or
  removals, all of them on the expected list; `check` reports nothing against
  this task beyond a Handoff staleness that this transition clears.
- 2026-09-15 validate: diff reviewed hunk by hunk. Two stale statements were
  corrected in the candidate rather than left for the reader: the `ActivityPage`
  doc comment still described chips beside the title and retention below the
  listing, and the export note kept `activity-filter-note` after leaving the
  filter form (now `activity-export-note`). Component knowledge reconciled:
  `gui-foundation.md` Change map, the Activity file description, the seven new
  IDs under Key behaviors, and a current-state section for the page furniture.
  `project.md` needs no change - its header-group statement is about
  page-scoped View activity actions on other pages, which this task leaves
  alone. Re-proved after those edits: `npm run format:check && npm run lint &&
  npm run test:ci`, exit=0, 494 tests.
- 2026-09-15 validate review round 1: specification compliance = approved. All
  seven requirements are implemented as written, the three stated exclusions
  hold (no Activity API, parser or contract change; no table columns, sorting,
  details modal or export format change; no other page touched), and
  RULING-001 is realised by rendering the chips inside the form only where the
  panel is inline. Considered and accepted: the export note now renders between
  the conditions strip and the retention disclosure. It is transient feedback
  for an action rather than a standing label, so REQ-UX-c5976f3b, which governs
  what is reported about recording conditions, is not weakened by it.
- 2026-09-15 validate review round 1: implementation quality = approved. The
  More filters control keeps disclosure semantics as an `aria-expanded` button
  with `aria-controls`, and its fields stay reachable to the same
  `getByText('More filters')` interaction; `hidden` leaves the UA `display:none`
  in force, so `display: contents` applies only when open. No behavior outside
  the seven requirements changed: `applyQuery` still owns every filter change
  and its generation guard, export still sends the applied filter, retention
  keeps its permission gate and save path. Two small changes beyond the literal
  ask are retained deliberately: the `ACTIVITY_PAGE_SIZES` constant, which keeps
  the offered sizes and the cast in one place, and the note class rename.
- 2026-09-15 validate: first `check --task TASK-29673ddb --strict --run`
  returned exit 1. The appliance stage ran 42 browser tests and one failed:
  `e2e/field-display.spec.ts::1366: the activity header and its filters are
  each one line`, at the assertion that More filters shares a row. The defect
  was in the new test, not the candidate. Measured in a real browser against
  the mock build at 1366: the wrapping line puts scope, From, To, Actor and
  Target on the first row and Action, Outcome and the More filters disclosure
  on the second, so the disclosure does share its row with filters. The
  assertion had compared it only against the scope and `From`, both on the
  first row - stricter than REQ-UX-f8c58c4c, which asks that the disclosure
  never hold a row alone, and wrong about a layout whose whole point is that it
  wraps. It now checks the disclosure against the scope and all six fields. No
  product file changed; the same browser check also confirmed the header is one
  row with Export ending 16px from its right edge and carrying no paragraph,
  no conditions strip renders on a healthy appliance, the retention disclosure
  precedes the filter form, and the page-size control reads 10.

- 2026-09-15 deploy: the user authorised deployment of this candidate to the
  Raspberry Pi 4 appliance `ddp-remote` (Wi-Fi `DDP`, 192.168.2.11; reached over
  the nilo0 path 10.205.148.11 after the wireless path dropped TCP 22, same host
  key SHA256:d5okLKeJtvgvvK/IQy3TSZZBZ/lfwyNaqGgl9tv4PYI). GUI assets only; no
  core binary, helper, data or network change. Rollback archive
  `/var/backups/nilo/pre-activity-layout-20260915T072810Z/gui.tar` (634880 bytes).
  `/opt/nilo/gui` replaced with the local production build; the served
  JavaScript hash e5cce7d79ec80004b28932c509acc546adc3bc935e141c1a8fe6e0569103c84f
  matches the built file. Bench Core holds the interface in memory from start, so
  the swap took effect only after `systemctl restart nilo`; `nilo` and
  `nilo-nethelper` are active, the recovery timer is scheduled, HTTPS returns 200
  and `/api/v1/activity` returns 401 unauthenticated. The deployed build carries
  no commit identity: the candidate is uncommitted by the user's instruction, so
  this journal entry and the asset hash are the only record of what is on the
  appliance.

- 2026-09-15 validate review round 2: the corrected browser assertion changed
  the candidate after round 1, so both verdicts were taken again on the final
  tree. Specification compliance = approved: the only change since round 1 is
  `e2e/field-display.spec.ts`, whose assertion now states REQ-UX-f8c58c4c's own
  boundary - the disclosure never holds a row alone - instead of the stricter
  and incorrect claim that it shares row one. No product file, requirement,
  design decision or exclusion moved. Implementation quality = approved:
  unchanged product code, and the assertion now reads the scope and all six
  fields rather than two of them.
- 2026-09-15 validate green: command=`python .agentic/aegis.py check --task
  TASK-29673ddb --strict --run`; exit=0; `strict: ok`, errors=0, warnings=37,
  notes=2. Both configured commands ran: `test` (script tests, Go, Python, GUI
  `test:ci` and `build`, then the WSL2 appliance stage with its 42 browser
  tests) and `lint`. The console capture of this run was truncated to its tail
  by the invoking shell, not by the runner; a failed command would have
  appeared as `ERROR command-failed (run)`, as it did on the previous attempt,
  and none is present. Remaining warnings are pre-existing and belong to other
  tasks and requirements - stale requirement paths under `00_Planning/` and
  `06_SoftwareDocu/`, and review or Handoff state on seven other journals.
  None is attributable to this task.

- 2026-09-15 deliver -> build: human review finding accepted (P2): at widths at
  or below 56rem the `@media` block overrode `.activity-head .menu` with
  `margin-left: 0`, so Export rendered beside the title instead of at the right
  edge of the header - reported at 800x480 as a right edge of 197px against a
  header edge of 800px. REQ-UX-3e01a5b6 asks for Export aligned to the right of
  the title line at every width, so this is a candidate defect, not a review
  preference. The override was carried over from the old
  `.activity-filters .form-actions .menu` rule, which had a reason while Export
  lived in the filter actions row and none once it moved to the header. The
  finding also names a real coverage gap: the new browser assertions run only at
  1920 and 1366, both of which keep the auto margin. Returning to build; both
  round-2 verdicts are void.

- 2026-09-15 build red: REQ-UX-3e01a5b6; command=`npm run test:ci --
  src/layout.test.ts` in `05_Software/nilo_gui`; exit=1; 1 of 23 failed.
  Expected missing behavior: the `@media (width <= 56rem)` block still carried a
  `.activity-head .menu` margin override, so the assertion that no narrow-width
  rule takes back the auto margin did not hold.
- 2026-09-15 build green: REQ-UX-3e01a5b6; the two narrow-width `.activity-head`
  overrides were deleted, so the auto margin holds at every width and the header
  wraps Export below the title rather than pulling it against the title when the
  two no longer fit. Browser coverage was restructured first: the header
  assertions now run at all four viewports (1920, 1366, 1024, 800) and the
  filter-line assertions stay at the two widths that show the panel inline.
  command=`npm run test:ci && npm run format:check && npm run lint && npm run
  build`; exit=0; 494 tests across 35 files. Measured in a real browser at the
  reported viewport: at 800x480 Export now ends at 788px against a header edge
  of 800px, on the title's line, with a 44px target and no header paragraph -
  against the 197px the review reported.

- 2026-09-15 validate: `check --task TASK-29673ddb --strict --run` returned
  exit 1 again, on `e2e/field-display.spec.ts::1920: the activity filters are
  one wrapping line` at `sharesRow(scope, fields[0])`. Diagnosed by making the
  assertion print the geometry it judges, then measuring the same page in a
  browser. The layout is correct at 1920: the scope chips, the six field
  wrappers and the More filters disclosure all sit on one line with their
  bottoms aligned at 297, and only the form actions take the next row. The
  assertion was wrong for the second time in the same way - it compared the
  chips container against the `input` inside a field rather than against the
  field wrapper, and two controls on one line do not overlap vertically when
  the line aligns them differently. `sharesRow` is not a sound test of "same
  line" for this row.
- 2026-09-15 build: the filter-line assertion was rewritten to read the flex
  items in document order and identify line starts by the container's content
  edge, which a wrapped line restarts at and cross-axis alignment cannot
  disturb. It now asserts what REQ-UX-f8c58c4c states and nothing more: the
  eight controls are scope, six fields and the disclosure; every line carries at
  least two of them, so none holds a row of its own; and at 1920 there is
  exactly one line. The failure message carries the measured geometry, so a
  future failure reports the layout instead of a bare false.
- 2026-09-15 build green: command=`npm run test:ci && npm run lint && npm run
  build` in `05_Software/nilo_gui`; exit=0; 494 tests across 35 files, and the
  production build emitted. One unrelated flake was seen once on the way:
  `src/App.test.tsx::connection states > retries every resource with the same
  session when the backend was unreachable` expected 3 attempts and observed 2,
  then passed alone and passed again in the full suite. It is a retry-count
  timing assertion in a file this task does not touch; recorded as an
  observation about that test, not as evidence about this candidate.

- 2026-09-15 validate review round 3: specification compliance = approved.
  Every one of the seven requirements holds on the final tree, the three stated
  exclusions are intact, and the accepted P2 finding is fixed at its own
  requirement rather than worked around: REQ-UX-3e01a5b6 now holds at 800 and
  1024 as well as at 1366 and 1920, proven at all four. Implementation quality =
  approved: the only product change since round 2 is the deletion of two
  narrow-width `.activity-head` overrides, and the two browser assertions that
  failed were corrected to state their requirements rather than an observed
  arrangement. No unrequested work was added and no test was weakened - the
  filter-line check is stricter than before, since it now counts the controls
  and rejects any line holding one alone.
- 2026-09-15 validate green: command=`python .agentic/aegis.py check --task
  TASK-29673ddb --strict --run`; exit=0; `run: test ok (333.0s)`,
  `run: lint ok (7.5s)`, `strict: ok`, errors=0, warnings=37, notes=2. The test
  command includes the WSL2 appliance stage and its 46 browser tests. The 37
  warnings are the same pre-existing set as before: stale requirement paths
  under `00_Planning/` and `06_SoftwareDocu/`, and review or Handoff state on
  seven other journals. None belongs to this task.

- 2026-09-15 deploy: the user authorised redeploying the corrected candidate
  once verification was green. `/opt/nilo/gui` on `ddp-remote` replaced from the
  validated tree after `systemctl restart nilo`; rollback archive
  `/var/backups/nilo/pre-activity-layout-20260915T081238Z/gui.tar` (634880
  bytes) holds the build this replaced, and
  `pre-activity-layout-20260915T072810Z/gui.tar` still holds the pre-task
  interface. Served hashes match the local build: JavaScript
  e5cce7d79ec80004b28932c509acc546adc3bc935e141c1a8fe6e0569103c84f - unchanged,
  because the P2 fix was CSS only - and stylesheet
  39da10d6a226e9ba9ff731e719e1476ad382d9fc0c68650e69cb07c8c22191a0. HTTPS
  returns 200, `/api/v1/activity` returns 401 unauthenticated, and `nilo` and
  `nilo-nethelper` are active. The appliance now runs the build the strict run
  proved; it still carries no commit identity.

- 2026-09-15 deliver -> build: second human review finding accepted (P2). Where
  the viewport collapses the filter panel, the scope row renders as a direct
  flex child of `.page-body`, and `.activity-chips` carries `overflow-x: auto`
  with no `flex`. A non-visible overflow suppresses a flex item's automatic
  minimum size, so the row absorbs the column's shrink. Reproduced at 800x480
  as an administrator, with the retention disclosure present: the row computes
  to `flex: 0 1 auto` and renders 14px tall while each scope button is 44px, so
  All, Audit and Events are clipped to their middle 14 pixels. This is the same
  trap `.activity-page .table-scroll` already carries `flex: none` for, and
  moving the chips out of the header walked into it. REQ-UX-ba46f6d3 asks that
  the scope controls stay visible and selectable where the panel collapses, so
  a row clipped to 14px does not satisfy it. The finding also names a real
  blind spot: `expectFieldTargets` measures each control's own bounding box and
  never its visible intersection with an overflow ancestor, so a clipped but
  nominally 44px target passes. Returning to build; both round-3 verdicts void.

- 2026-09-15 build red: REQ-UX-ba46f6d3; command=`npm run test:ci --
  src/layout.test.ts` in `05_Software/nilo_gui`; exit=1; 1 of 23 failed.
  Expected missing behavior: `.activity-chips` declared no `flex`, so nothing
  held the scope row open against the page body's shrink.
- 2026-09-15 build green: REQ-UX-ba46f6d3; `.activity-chips` now declares
  `flex: none`, the same remedy and the same reason as
  `.activity-page .table-scroll`. Measured at 800x480 as an administrator with
  the retention disclosure present: the row computes to `flex: 0 0 auto` and
  renders 48px, and All, Audit and Events each have 44px of visible height,
  against 14px before. At 1024x600 the row is 48px with all three fully visible
  and the filter drawer still ends at 371 of a 600px body, so the 34px the row
  regained does not threaten REQ-UX-dd4b2c16. Browser coverage added for the
  defect class the review named: the field-viewport Activity test now measures
  each scope button's visible intersection with its row rather than its own
  box, so a clipped control fails even while reporting 44px. command=`npm run
  format:check && npm run lint && npm run test:ci && npm run build`; exit=0; 494
  tests across 35 files.
- 2026-09-15 build: the review's wider point - that `expectFieldTargets` reads
  each control's own rect and never its visible intersection with an overflow
  ancestor - is true of every page the helper covers, not only this row.
  Hardening it would change what the suite enforces for Network, Authentication,
  Bench and About and would surface unrelated pre-existing findings, so it is
  recorded as separate work rather than folded into this layout change.

- 2026-09-15 validate review round 4: specification compliance = approved. All
  seven requirements hold on the final tree and the three stated exclusions are
  intact. The accepted finding is fixed at the requirement it belonged to:
  REQ-UX-ba46f6d3 asks that scope selection stay usable where the panel
  collapses, and a row clipped to 14px did not satisfy it. Implementation
  quality = approved: one declaration changed, matching an established rule in
  the same stylesheet, and the new assertion measures visible geometry rather
  than a reported box. No unrequested work was taken on - the shared helper's
  wider blind spot was deliberately left for separate work.
- 2026-09-15 validate: review round 4 exceeds `[review] max_fix_cycles = 2`,
  which bounds agent-initiated fix cycles at three rounds. Rounds 3 and 4 were
  entered on accepted human review findings, each an explicit decision by the
  user to return the candidate to build, not a self-directed retry. Recorded
  here rather than silently, and raised with the user at delivery.
- 2026-09-15 validate green: command=`python .agentic/aegis.py check --task
  TASK-29673ddb --strict --run`; exit=0; `run: test ok (369.9s)`,
  `run: lint ok (7.8s)`, `strict: ok`, errors=0, warnings=36, notes=2. The test
  command includes the WSL2 appliance stage and its browser suite. The 36
  warnings are the pre-existing set on other tasks and requirements; none
  belongs to this task.

- 2026-09-15 deploy: the user authorised deploying the revalidated candidate to
  `ddp-remote`. `/opt/nilo/gui` replaced from the validated tree and `nilo`
  restarted; rollback archive
  `/var/backups/nilo/pre-activity-layout-20260915T092842Z/gui.tar` (634880
  bytes) holds the build this replaced, and
  `pre-activity-layout-20260915T072810Z/gui.tar` still holds the pre-task
  interface. Served hashes match the local build: JavaScript
  e5cce7d79ec80004b28932c509acc546adc3bc935e141c1a8fe6e0569103c84f - unchanged
  across all three deployments, since both accepted findings were CSS only -
  stylesheet 5ae6bf2536bb3ac31004262fbdeb4006faa8a55f5c31a0edccae226a35c35948,
  and index ab85dceafece0875822869efd8dc5d774e564152672a9756dc7b25c208f89ea4.
  The served stylesheet carries `.activity-chips{...flex:none...}`, so the
  appliance has the scope-row fix and not merely a new file name. HTTPS returns
  200, `/api/v1/activity` returns 401 unauthenticated, `nilo` and
  `nilo-nethelper` are active. The appliance still carries no commit identity.

- 2026-09-15 deliver: the user asked for the project version to be assigned
  before redeploying. Version assignment is a repository operation rather than
  an AEGIS stage, so it follows the repository's own procedure and is recorded
  here only as delivery history. 0.5.0 -> 0.6.0 across `VERSION`,
  `internal/buildinfo/buildinfo.go`, `nilo_gui/package.json` and
  `nilo_gui/package-lock.json`, with a `## [0.6.0] - 2026-09-15` CHANGELOG
  section describing the Activity page in current-state terms. Changing the
  lockfile changes its digest, which three gates check, so
  55b9055c7b27677b9157f96d5c7f2422df93131b875a50a2362f833309e7ce3f ->
  45bd096d55b05db121787ae48dcc1902d8915da024fb7e65448ce08c4fb6908f was applied
  to `02_Tools/toolchain.versions`,
  `02_Tools/containers/build/build-environment.lock.json` and
  `05_Software/scripts/tests/test_toolchain_bootstrap.py`. The shape matches
  commit a9573fc, which landed the same digest regeneration in the same files
  and left the multipart build-environment archive untouched. Delivery
  presentation up to this point had omitted the version and changelog, which
  the last three feature commits both carry; the omission was the user's catch,
  not a rule being followed.

- 2026-09-15 deliver: the version bump required the four component assertions
  that read the displayed version to move with it - `App.test.tsx` three times
  and `shell.test.tsx` once - because REQ-UX-8a1ddb96 shows the checked-in GUI
  package version before sign-in. Commits 2d277a6 and a9573fc updated the same
  literals for their own bumps, so this is the repository's procedure rather
  than a test bent to fit. `src/App.test.tsx::connection states > retries every
  resource with the same session when the backend was unreachable` failed a
  second time on the way (expected 3 attempts, observed 2) and passed on rerun
  again; it is unrelated to this candidate and is now recorded as separate work
  rather than touched here.
- 2026-09-15 validate green: command=`python .agentic/aegis.py check --task
  TASK-29673ddb --strict --run`; exit=0; `run: test ok (336.9s)`,
  `run: lint ok (7.8s)`, `strict: ok`, errors=0, warnings=43, notes=2. Warnings
  rose from 36 to 43 because the version assignment touched five files no
  requirement's `affected_paths` covers - `internal/buildinfo/buildinfo.go`,
  `nilo_gui/package.json`, `nilo_gui/package-lock.json`, `src/App.test.tsx` and
  `src/components/shell/shell.test.tsx` - each reported as `uncovered-source`.
  The reports are accurate: version assignment is a repository operation outside
  AEGIS, so those files are governed by the repository's procedure and not by a
  requirement of this task. They are not suppressed by listing repository files
  under a UX requirement, which would misstate what that requirement governs.
  The remaining warnings are the pre-existing set on other tasks, plus this
  task's `review_round 4 exceeds configured total 3` and the expected
  `unticked-deliver`.

- 2026-09-15 deploy: the 0.6.0 interface is on `ddp-remote`. `/opt/nilo/gui`
  replaced from the validated tree and `nilo` restarted; rollback archive
  `/var/backups/nilo/pre-0.6.0-20260915T095134Z/gui.tar` (634880 bytes) holds
  the build this replaced. Served hashes match the local build: JavaScript
  f3435529c9f83b9b0e94f5b6df9287d212bff3198adb3990d98db9dfa9841b0d - changed
  this time because the version string is compiled into the bundle - stylesheet
  5ae6bf2536bb3ac31004262fbdeb4006faa8a55f5c31a0edccae226a35c35948, unchanged
  since the previous deployment, and index
  7634f52f844ab69a23d2194c3b5e147cc447dfbb2b35719d53265dd912c7bdd7. The served
  bundle carries 0.6.0. HTTPS returns 200, `/api/v1/activity` returns 401
  unauthenticated, `nilo` and `nilo-nethelper` are active.
- 2026-09-15 deploy: the appliance now reports two different versions. Its
  interface is 0.6.0 and `nilo version` on the appliance still answers
  `{"name":"nilo","version":"0.5.0",...}`, because only the interface assets
  were deployed and `internal/buildinfo` reaches an appliance through a rebuilt
  ARM64 core binary. An operator sees 0.6.0 before sign-in, from the checked-in
  GUI mirror, and 0.5.0 in the authenticated build identity. The project keeps
  one Semantic Version across `VERSION`, `internal/buildinfo` and
  `nilo_gui/package.json`, so this is a real inconsistency on that appliance
  rather than a cosmetic one. Deploying a core binary is a larger operation
  than an asset swap - it replaces the running service - so it was not done
  without being asked. Raised with the user.

- 2026-09-15 deploy: core deployment prepared and stopped before the swap. The
  appliance binaries were built from this tree with `build-appliance.sh`
  (linux/arm64, CGO-free, -trimpath, pinned toolchain): core
  5a0a1d70592f95e9311651acdf9cb65c60149e87095c490e14d16178c10017df, helper
  5f32b2a64772451c92afeccfbcb8de060346c8e6ec4af8fc1394eb10d571d81f. A control
  build of the same tree pinned back to 0.5.0 produced
  5a6df9ecc7066a5478f5996eb2442a44d71f964c50a78c8fe788402d115abe42 at 23778666
  bytes, while `/usr/local/bin/nilo` on `ddp-remote` is
  9eb3109444672a875539e475b8e92fff8f36d25d7a149896d6be6de200de36c6 at 23778810
  bytes, dated 2026-09-15 00:46. The running core is therefore not this tree at
  either version, so replacing it would change core code and not only the
  version string. It is not the main checkout's current state either: that
  working tree carries 83 uncommitted modified Go files and now builds to
  27809505 bytes with an added `go-qrcode` dependency. The installed helper
  differs from this tree's too (7020475 against 7020419 bytes). The appliance is
  running an unidentified build, so the swap was not performed: the user
  authorised a core deployment on the stated premise that it carried only the
  version bump, and that premise is false. Raised with the user before any
  change. Reading the main checkout to establish this mutated nothing in it -
  its `go.mod` and `go.sum` timestamps predate the comparison build.

- 2026-09-15 deploy: the user reaffirmed the instruction after the finding
  above - one project version, rebuild the whole project and deploy all of it -
  so the full appliance deployment was carried out. `build-appliance.sh`
  rebuilt both binaries for linux/arm64 from this tree, reproducing the earlier
  hashes exactly: core
  5a0a1d70592f95e9311651acdf9cb65c60149e87095c490e14d16178c10017df, helper
  5f32b2a64772451c92afeccfbcb8de060346c8e6ec4af8fc1394eb10d571d81f. The
  binaries they replaced are archived whole at
  `/var/backups/nilo/pre-0.6.0-core-20260915T101144Z/` - core
  9eb3109444672a875539e475b8e92fff8f36d25d7a149896d6be6de200de36c6 and helper
  f291b31f521447a230db38c2ffc2378a78f5175a16ea61caa94dc6b57f2ec9f8, the
  unidentified build - so the previous appliance state is restorable. Installed
  with `install -o root -g root -m 0755` to `/usr/local/bin/nilo` and
  `/usr/local/libexec/nilo-nethelper`, helper started before core. Verified:
  both services active, `nilo version` reports 0.6.0, the core logs
  `{"status":"listening","address":"[::]:8443","profile":"appliance",...}`, the
  helper recovery timer is scheduled, HTTPS returns 200, `/api/v1/activity`
  returns 401 unauthenticated, and the interface still serves the 0.6.0 bundle
  index-B6XYEJ5h.js. `ddp-remote` now runs core, helper and interface all built
  from this tree at one version.

- 2026-09-15 deliver: the user approved delivery by asking for the candidate to
  be staged, and asked for a commit message rather than a commit. `aegis report`
  appended to this journal. 31 entries staged: 21 modified and 10 new, all of
  them this task's - the Activity page work, the 0.6.0 version assignment and
  its digest regeneration, the seven requirement files and this journal with its
  anchor and evolution records. Nothing is committed or pushed: the repository
  requires a current instruction for each Git step, and this one named staging
  only. The commit itself is left for the human, per the repository's delivery
  convention.

- 2026-09-15 done: delivered. The human committed the staged candidate as
  1f6fabe on `ddp/nilo-activity-page-layout` and merged it into master as
  d0c5c81, pull request #21, carrying all 31 files - 5517 insertions, 278
  deletions - including this journal, its anchor and evolution records and the
  seven requirements. `ddp-remote` runs the same tree: core and helper
  5a0a1d70592f95e9311651acdf9cb65c60149e87095c490e14d16178c10017df and
  5f32b2a64772451c92afeccfbcb8de060346c8e6ec4af8fc1394eb10d571d81f, interface
  bundle index-B6XYEJ5h.js, all reporting 0.6.0, so the deployed appliance now
  has the commit identity it lacked through the earlier deployments. Gitea issue
  #20 has not been commented on or closed; that remains open for the human.
  Stage set to done in the master checkout, which is where the journal lives now
  that the task worktree has been removed.

## Rulings

- RULING-001 | date=2026-09-15 | decision=Where the Activity filter panel is collapsed behind a disclosure the category scope controls stay outside it; the single scope-and-filters line applies where the filter panel is shown inline. | reason=1024x600 is the panel this change is for, and putting scope selection behind the filter drawer there would cost a step on the one filter always in play. | source=user | supersedes=none
- RULING-002 | date=2026-09-15 | decision=This task runs in autopilot from design through deliver, and deliver stops at a presented candidate: no staging, commit, branch change or push is performed. | reason=The user asked to continue without prompting and said explicitly not to commit; repository Git authority requires a current instruction for each Git step. | source=user | supersedes=none


## Handoff


current: done
status: ready
next: none; the change is merged as d0c5c81 and deployed to ddp-remote
must_read: .agentic/tasks/TASK-29673ddb.md
blockers: none

## Stage report

stage: deliver
result: completed
current: done
changed: 31 files committed as 1f6fabe and merged as d0c5c81 (pull request #21)
evidence: python .agentic/aegis.py check --task TASK-29673ddb --strict --run - exit 0, run: test ok (336.9s), run: lint ok (7.8s), strict ok, errors=0, warnings=43, notes=2
issues: review round 4 exceeded the configured bound of three, entered on two accepted human findings; Gitea issue #20 is still open and uncommented
next: none for this task; issue #20 is the human's to close
needs_user: no
spec_compliance: approved
implementation_quality: approved
review_round: 4
