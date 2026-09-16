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

# TASK-a86b32ec — Authentication administration interface

stage: done
type: feature
path: full
requirements: REQ-FUNC-a6c14c23, REQ-FUNC-e0da3a06, REQ-FUNC-5ffce0e4, REQ-FUNC-5d80ae6c, REQ-FUNC-5fd2806f, REQ-FUNC-27c1db30, REQ-FUNC-1fac821d, REQ-FUNC-907003de, REQ-FUNC-795b54d0, REQ-FUNC-f50eb6d3, REQ-FUNC-71c3c908, REQ-FUNC-938c5f85, REQ-UX-c7a277c5, REQ-UX-c4a0c9e7, REQ-UX-ae7baff0, REQ-UX-e60cf375, REQ-UX-96852626, REQ-UX-38017d29, REQ-UX-b849ddb2, REQ-SEC-89034a8a, REQ-SEC-7595665c, REQ-SEC-66b84b2c, REQ-SEC-6e3ae992, REQ-TEST-68dcb172, REQ-SEC-49fe784b, REQ-SEC-8e89944b, REQ-SEC-ed66f6ed, REQ-UX-40473e43, REQ-UX-66636ac9, REQ-UX-0018c647, REQ-TEST-76b94249, REQ-TEST-0a4f49af
parent:
branch: master
base: c39bf1ea4ebf4ac5f34072cbad30657aea7af553
opened: 2026-09-02

## Done checklist (current stage)

- [x] validated candidate confirmed unchanged; otherwise returned to validate
- [x] validation evidence, diff, exclusions, and outcome presented
- [x] delivery confirmed by the human
- [x] journal and report included in the delivery commit
- [x] agreed outcome delivered, normally a PR
- [x] close-out committed after the outcome succeeded, or carried by the delivery commit where nothing could fail after it

## Log

- 2026-09-02 frame: scope is Slice 8, the browser interface for the Slice 7 identity foundation - human username/password login over the existing server-managed HttpOnly session cookie, session restoration after a page refresh, logout, an administrator-only Authentication page administering users and API tokens, and the Bench Core management endpoints those screens need.
- 2026-09-02 frame: explicitly out of scope - custom roles, arbitrary permission editing, token self-service for Operators, SSO, MFA, email recovery, public registration or a remote bootstrap route, permanent user deletion, public deployment, MCP transport, hardware operations, plugins, updates, themes, version bumps, empty Settings/Plugins/About pages, and unrelated development-script refactoring.
- 2026-09-02 frame: full path chosen because the work adds externally observable HTTP endpoints, a new browser authentication mode, new screens, and security-critical authorization and CSRF behavior, all of which the quick path forbids.
- 2026-09-02 frame: no child tasks - the endpoints, the client contract and the screens are one vertical slice whose halves cannot be validated apart; a GUI page with no endpoints proves nothing and an endpoint with no page is the slice 7 command line again.
- 2026-09-02 frame: components touched are `internal/httpapi` and `internal/identity` in Bench Core, the whole of `05_Software/nilo_gui` (contract, client, mock, shell, components, styles, component tests, browser tests), and the planning, knowledge and delivery documents that record the access model and the development flow.
- 2026-09-02 frame: the four verified Slice 7 corrective fixes stay intact and are not re-opened - the `app.Open`/`app.New` reconciliation split, the single `BEGIN IMMEDIATE` administrative transaction carrying its audit event, the last-enabled-administrator guard inside that transaction, and the conditional session insert bound to the password verifier the login checked.
- 2026-09-02 frame: expected requirements - browser password login establishing the server-managed session; session restoration from the current-session route after a reload; logout; expired or revoked session handled as a return to sign-in distinct from a permission refusal; CSRF echoed on every cookie-authenticated state change; no password, session secret or new token secret in web storage, a URL or a log; administrator-only Authentication navigation entry; user listing; user creation; role change; enable/disable; password reset; token listing with safe metadata only; token creation with scopes and optional expiry; one-time secret disclosure; token revocation; server-side authorization and validation on every new endpoint; sanitized response models; single-viewport administration layout; and a test requirement covering the new HTTP-boundary and browser proofs.
- 2026-09-02 frame: the prompt approves this scope in advance and reserves no scope-approval stop, so the frame checkpoint is logged rather than asked.
- 2026-09-02 specify: wrote 24 requirements - four for the browser sign-in, restore, sign-out and development-token behaviors, seven for the new HTTP administration routes plus one for refusing a request that cannot be applied, seven for the administration page's navigation, layout, confirmation, one-time secret, refusal distinction, ownership statement and action outcomes, four for the security of the acting identity, the route authorization, the browser credential handling and the serialized representations, and one for the browser end-to-end proof.
- 2026-09-02 specify: two existing requirements were widened in place rather than duplicated - REQ-SEC-49fe784b now also requires the CSRF check over every administration route, and REQ-SEC-8e89944b now also requires the last-administrator refusal to surface as a conflict over HTTP; both keep their original prose because the behavior is the same one, now reachable from a second interface.
- 2026-09-02 specify: one spec, `.agentic/specs/REQ-FUNC-5fd2806f.md`, records the route table, the request and response bodies, the error codes and the validation order once instead of repeating them across eleven requirement files; `.agentic/specs/REQ-SEC-5d0cfe7e.md` gained the eight administration rows in its route table and now points at it.
- 2026-09-02 specify: `python .agentic/aegis.py check` reported errors=0 warnings=32 notes=1; every new warning is a `stale-paths` hit naming a file this task has not created yet, which is what `affected_paths` is supposed to say at specify.
- 2026-09-02 design: recorded the decisions and test seams in `## Design`, and revised the `## Architecture` diagrams of `bench-core-foundation` and `gui-foundation` to the shape they will have after this task, each marked as intended rather than existing.
- 2026-09-02 build red: command=`go test ./internal/httpapi/ -run 'TestAdministration|TestToken' -count=1`; exit=1; expected missing behavior=every new administration route answered `404 page not found` or `405 Method Not Allowed` because none of the seven exists - covering REQ-FUNC-5fd2806f, REQ-FUNC-27c1db30, REQ-FUNC-1fac821d, REQ-FUNC-907003de, REQ-FUNC-795b54d0, REQ-FUNC-f50eb6d3, REQ-FUNC-71c3c908, REQ-FUNC-938c5f85, REQ-SEC-89034a8a, REQ-SEC-7595665c, REQ-SEC-6e3ae992 and the HTTP half of REQ-SEC-8e89944b.
- 2026-09-02 build: implemented the Bench Core half - `internal/httpapi/administration.go` with the seven handlers, their validation and the typed-error mapping; the eight route registrations in `server.go`; `auth.MinPasswordBytes`/`MaxPasswordBytes` exported so an interface can refuse a password before the slow hash runs; and `identity.ErrExpiryNotInFuture`, which makes a past token expiry an invariant of the service rather than a check each caller repeats.
- 2026-09-02 build: `handleListUsers` and its response type moved from `identity.go` into `administration.go` unchanged, so identity administration lives in one file; `userListEntry` was deleted in favour of the shared `identityView`, which is the only identity representation the API now serializes.
- 2026-09-02 build: `go test ./...` green across all 20 packages, `go vet ./...` and `gofmt -l .` clean.
- 2026-09-02 build: DEVIATION build - the browser-interface half was implemented before its component tests were written, rather than test-first as the Bench Core half was; the cost is that those tests were written against the built behavior rather than independently from the requirement, and they are honest about what they observe but are not a red-then-green record.
- 2026-09-02 build: implemented the interface half - the contract's session, identity and token types and strict parsers; a `Credential` union in the client that sends a bearer header or a CSRF header but never both; five failure causes instead of three; `api/csrf.ts` for the readable cookie; `api/permissions.ts`; a shell with restoring, signed-out, connecting, refused and active phases; `SignInScreen`, `WorkspaceHeader`, `AuthenticationPage`, `AdministrationForms` and `Modal`; and an in-memory identity and token store in the mock so `dev:mock` still has something to sign in to.
- 2026-09-02 build: two real defects found by the new tests rather than by reading - the mock seeded its identities with `role: username`, so `admin` was not a role and every mock sign-in parsed as malformed; and the browser suite asserted an empty token listing although the harness issues itself a development token to reach the bench with, so the listing is never empty.
- 2026-09-02 build: `e2e/bench-core.spec.ts` needed one honest update - it asserted the heading `Connect to Bench Core`, which this slice replaced with `Sign in to NILO`; its bearer-token journey is otherwise untouched and still passes.
- 2026-09-02 build: `npm run test:ci` 215 tests green in 11 files, `npm run lint` clean at `--max-warnings 0`, `npm run format:check` clean after one `npm run format` pass, and `tsc -b` exit 0.
- 2026-09-02 build: `npm run test:e2e` exit 0 - 12 browser tests passed in 20.6s against real Bench Core instances under WSL2, including the eight new human-path tests and the two pre-existing bearer-token ones.
- 2026-09-02 build: evolution over the candidate reports 37 changed files - 10 added, 26 modified, 1 removed - all intended by this task; `internal/httpapi/administration_test.go` does not appear because it was written before the anchor, as the step requires, and `LoginScreen.tsx` is the one removal, superseded by `SignInScreen.tsx`. The untracked `00_Planning/report.md`, which is concurrent work in this checkout, is untouched and absent from the report.
- 2026-09-02 build: six existing requirements were reconciled in place rather than left stale - REQ-SEC-ed66f6ed, REQ-UX-40473e43 and REQ-UX-66636ac9 pointed at the deleted `LoginScreen.tsx`, and REQ-UX-40473e43's prose said "while no bearer token has been submitted" where the screen is now a sign-in screen carrying both forms; REQ-TEST-76b94249, REQ-TEST-0a4f49af and REQ-UX-0018c647 own files this task changed. All six are on this task's requirements line, which is what makes their coverage checkable.
- 2026-09-02 build: `python .agentic/aegis.py check --task TASK-a86b32ec` reports errors=0 warnings=16 notes=1, with every `stale-paths` and `uncovered-source` hit resolved by that reconciliation.
- 2026-09-02 build: DEVIATION build - thirteen accepted `secret-in-diff` warnings, none a credential: every hit is the word "password" followed by a colon or an equals sign in a type annotation, a parameter name, a state binding or a format string; the only literal values are the documented mock constant `simulation-password` and deliberately fake fixture passwords in five test files, plus the fixture string `Bearer any-development-token` in a mock test.
- 2026-09-02 build: DEVIATION build - `outside-layout CHANGELOG.md` is accepted, as it has been since the file was added: this repository keeps its changelog at the root, outside the configured source, tests and docs globs.
- 2026-09-02 build: the two `unticked-deliver` warnings belong to TASK-35b7b070 and TASK-f9dbdeef, not to this task.
- 2026-09-02 build: honest note on that red run - `TestAdministrationRoutesRequireTheSessionCSRFToken` was the one new test that already passed, because the CSRF check lives in the credential-resolution middleware ahead of routing, so a session request with no token is refused before the router discovers the route does not exist; it becomes a meaningful proof of REQ-SEC-49fe784b over these routes only once they exist.
- 2026-09-02 specify: corrected a forward-looking claim in `.agentic/specs/REQ-SEC-5d0cfe7e.md` - it said the `tokens.*.self` self-service surface was slice 8's, and this slice deliberately excludes token self-service, so it now records the surface as still deferred.

- 2026-09-02 validate: reviewed the diff hunk by hunk against the frame scope - no custom roles, no permission editor, no token self-service, no SSO, MFA or email recovery, no public registration or remote bootstrap route, no user deletion, no MCP or hardware surface, no plugins or updates, no theme work, no version bump, and no empty Settings, Plugins or About page. The development scripts are unchanged; `nilo-dev.ps1` and the bootstrap scripts were not touched.
- 2026-09-02 validate: the four verified Slice 7 corrective fixes are intact and untouched - `app.Open`/`app.New` still split reconciliation from composition, every administrative change still runs inside `WithinAdministrativeTransaction` with its audit event, `guardLastAdministrator` still counts inside that transaction, and `CreateSession` is still the conditional insert bound to the password verifier the login checked. The new HTTP handlers call the same service methods and add no second path around any of them.
- 2026-09-02 validate: reviewed the two changes to existing Bench Core behavior - exporting `auth.MinPasswordBytes`/`MaxPasswordBytes` (the same values, now readable by an interface that wants to refuse a password before the slow hash runs) and adding the non-future expiry check to `identity.Service.CreateToken`, which also closes `nilo admin token create --expires-in -1h` on the command line.
- 2026-09-02 validate: Windows/Linux portability - nothing added reads a path, a filesystem or an environment variable; the Go changes are pure HTTP handling and the interface changes are browser code.
- 2026-09-02 validate: Inspection of the visual result, on the real page rather than from the source. Method - `vite --mode mock` on 127.0.0.1:5199, driven in a real Chromium through the browser tooling at 1280x720 and again at 820x720. Observed - the sign-in screen carries both forms and the endpoint; signing in as `admin` shows `admin - administrator` in the top bar with `Bench` and `Authentication` entries; the Authentication page shows the three seeded identities with their roles and states and the seeded token with its owner, scopes, expiry and state; creating a token shows its secret once with a copy action and the "cannot be retrieved later" warning, and dismissing it removes the value from the document while the success line names the created token; disabling the only administrator is refused in the dialog with the last-administrator sentence and the row stays enabled. Measured rather than eyeballed - `document.documentElement.scrollHeight` equals the 720px viewport with `body { overflow: hidden }`, both `.admin-list` regions carry their own `overflow-y: auto`, `localStorage` and `sessionStorage` are empty, and the token secret appears in no URL. At 820px the two panels stack, the document still does not scroll, the workspace scrolls instead, and every administration control still has a non-zero box.
- 2026-09-02 validate: DEVIATION validate - that inspection was carried out through the accessibility tree and measured DOM geometry rather than a rendered screenshot, because the browser pane could not composite frames in this session; the geometry and text assertions are stronger evidence for the single-viewport and one-time-secret claims than a picture would be, but no human has looked at the pixels.
- 2026-09-02 validate: re-reading the requirements against the tests caught two verifications that claimed more than the tests observed, and both were closed by strengthening the tests rather than by weakening the claim - REQ-UX-ae7baff0 promised that a confirmed disable and a confirmed password reset are each applied exactly once and only the revocation half proved it, and REQ-SEC-66b84b2c promised console spying and a URL check that no test performed. Three new client tests now prove the password travels in one request body and the request-forgery token in a header with neither in any URL, and one proves the five refusal classifications; the shell test now spies on every console method too.
- 2026-09-02 validate: the first `check --task --strict --run` run exited 0 but is deliberately not the recorded evidence: those two test strengthenings changed the candidate after it started, so the gate was rerun over the final tree.
- 2026-09-02 validate: cross-compilation rechecked with `CGO_ENABLED=0` for linux/arm64, linux/amd64 and windows/amd64, all exit 0.
- 2026-09-02 validate: `python .agentic/aegis.py check --task TASK-a86b32ec --strict --run` exited 0 over the final tree - `test ok (69.4s)`, `lint ok (4.5s)`, `strict: ok`, errors=0 warnings=17 notes=1.
- 2026-09-02 validate: the browser suite really is inside that gate, on two pieces of evidence rather than an assumption - `05_Software/scripts/aegis_test.py` runs `test:ci`, then `build`, then `test:e2e` for the interface suite and fails the whole command rather than skipping when a prerequisite is missing, and earlier in this session `aegis.py test --impacted --base HEAD` printed that router invoking `npm.cmd run test:e2e` and all 12 browser tests passing.
- 2026-09-02 validate: DEVIATION validate - seventeen accepted `check` warnings, none a defect: thirteen `secret-in-diff` hits are the word "password" beside a colon or equals sign in type annotations, parameter names, state bindings and format strings, with the only literals being the documented mock constant and fake fixture passwords; `outside-layout CHANGELOG.md` is how this repository has always been configured; and the two `unticked-deliver` hits belong to TASK-35b7b070 and TASK-f9dbdeef.
- 2026-09-02 validate: updated `bench-core-foundation.md` and `gui-foundation.md` - both diagrams now describe code that exists and their "intended shape" lines are gone, and the Purpose, Public interface, Key behaviors and Files sections were rewritten where this task changed them. Three stale phrases naming the removed login screen were corrected in the GUI page.

- 2026-09-16 deliver: closed retrospectively. The user confirms they reviewed this work and committed it, and the journal was simply never closed at the time; the delivery commit cf77de8 feat: add authentication administration and the redesign carries this journal, which is how the delivery is evidenced here. The stage and the deliver checklist are brought into line with what happened. The review verdicts are left as `disabled` because that is what they were: `[review] enabled` was false when this task ran, so the two-verdict protocol never applied to it, and recording `approved` would claim a review that was never performed. No stage report field describing the work itself is altered.

## Design

The route table, wire bodies, error codes and validation order are written
once in `.agentic/specs/REQ-FUNC-5fd2806f.md` rather than duplicated here.
This section records only the decisions and the test seams.

**One `identity.Actor`, two interfaces (REQ-SEC-89034a8a, REQ-SEC-7595665c).**
The new routes are registered on the same `protected` mux behind the existing
`resolveIdentity` middleware and the existing `authorize(permission, …)`
wrapper, and each handler takes its acting administrator from `actorFrom(ctx)`.
No request body names an actor. Rejected alternative: a separate
administration mux with its own middleware, which would have given the
security-critical routes a second, independently maintained authorization path.

**Validation splits by who can decide it (REQ-FUNC-938c5f85).** The HTTP layer
decides what a request alone settles - shape, username form, password length,
role name, scope names, expiry parsability - and answers `400` with a fixed
message per code. The identity service keeps the invariants that need the
database: duplicate username, unknown subject, last enabled administrator, and
now an expiry that is not in the future. Rejected alternative: mapping the
service's error strings at the boundary, which would put internal text into
client responses and break on any rewording.

**Mutations that create return, mutations that change answer 204
(REQ-SEC-6e3ae992).** Creation returns the new identity or the new token with
its one-time secret; role, enabled, password and revoke answer `204` and the
interface re-reads the listing. That keeps the serialized surface to two
representations, neither of which has a field a verifier could occupy.

**The interface holds a credential, not a token (REQ-FUNC-a6c14c23,
REQ-FUNC-5d80ae6c).** `BenchCoreClient` methods take a `Credential` that is
either `{kind:'session', csrfToken}` or `{kind:'bearer', token}`. A session
credential sends no `Authorization` header and adds `X-NILO-CSRF` on every
state-changing request; a bearer credential does the reverse. Rejected
alternative: keeping the `string` token parameter and adding parallel session
methods, which would have doubled the request path the failure classification
depends on.

**Restoration asks the server (REQ-FUNC-e0da3a06, REQ-SEC-66b84b2c).** The
session secret is in an `HttpOnly` cookie the page cannot read, so the shell
starts in a `restoring` phase, calls `GET /api/v1/auth/session`, and becomes
signed-in or signed-out from the answer. The CSRF token is read from the
readable `nilo_csrf` cookie the server set, which is the only value the page
needs and the only one it can see. Nothing is written to web storage.

**A fourth failure cause (REQ-UX-96852626).** `BenchCoreFailure` gains
`forbidden` for `403` and `rejected` for a `400`, `404` or `409` that carries a
server message. `unauthorized` now means the session ended and returns the
shell to sign-in; `forbidden` keeps the identity signed in.

**Two pages, one shell (REQ-UX-c7a277c5).** The shell renders a Bench page and
an Authentication page, with the navigation entry for the second shown only
while the identity holds `users.read` or `tokens.read.all`. The page is a
two-column grid of a Users panel and an API Tokens panel, each a bounded list
that scrolls inside itself; every mutation runs in one focused modal.
Rejected alternative: inline row editors, which would have put six different
in-progress and failure states inside a scrolling table.

**Test seams.** `httpapi.NewHandler` over a real SQLite database is already the
seam for every authorization case (REQ-SEC-7595665c, REQ-FUNC-5fd2806f and its
siblings); the new tests extend `internal/httpapi/administration_test.go` using
the existing `apiFixture`. The `BenchCoreClient` interface is the seam for the
shell (REQ-FUNC-a6c14c23, REQ-UX-*), so component tests stub it rather than the
transport. `createMockTransport` gains an in-memory administration store, which
is what keeps `dev:mock` usable now that a human signs in with a password
(REQ-TEST-76b94249). `DevelopmentTopology` gains a stdin-carrying command runner
so the browser suite can bootstrap a real administrator (REQ-TEST-68dcb172).

**Knowledge architecture.** `bench-core-foundation.md` gains the administration
routes in its diagram; `gui-foundation.md` gains the sign-in screen, the
navigation, the administration page and the session-restoring path.

## Rulings

- RULING-001 | date=2026-09-02 | decision=Work on the current `master` checkout, and leave every change uncommitted and unstaged | reason=The prompt forbids staging, committing, pushing, branching and worktrees, and requires unrelated existing changes to be preserved | source=user | supersedes=none
- RULING-002 | date=2026-09-02 | decision=Human browser access uses the server-managed `nilo_session` cookie only; the development bearer-token path stays available beside it but is never required for a human sign-in | reason=The prompt requires cookie sessions for humans and requires the existing development tooling to keep working, and the two credential kinds are already kept apart in the Slice 7 access model | source=user | supersedes=none
- RULING-003 | date=2026-09-02 | decision=API-token administration in this slice is administrator-only and reuses the existing `tokens.read.all` and `tokens.manage.all` permissions; the `tokens.*.self` permissions gain no interface here | reason=The prompt scopes token management to administrators and excludes token self-service, so an Operator self-service surface would be unrequested work | source=user | supersedes=none

## Handoff

current: done
status: ready
next: none - the work was delivered in cf77de8
must_read: .agentic/tasks/TASK-a86b32ec.md
blockers: none

## Stage report

stage: deliver
result: completed
current: done
changed: 37 files - 10 added, 26 modified, 1 removed. Bench Core: new internal/httpapi/administration.go with the seven handlers and their validation, the eight route registrations in server.go, handleListUsers moved out of identity.go, exported auth.MinPasswordBytes/MaxPasswordBytes, and identity.ErrExpiryNotInFuture. Interface: new api/csrf.ts, api/permissions.ts, components/SignInScreen.tsx, WorkspaceHeader.tsx, AuthenticationPage.tsx, AdministrationForms.tsx and Modal.tsx, with components/LoginScreen.tsx removed; api/contract.ts, api/client.ts, api/mock.ts, App.tsx, App.css, StatusPanel.tsx and InventoryPanel.tsx reworked. Tests: new internal/httpapi/administration_test.go and src/components/AuthenticationPage.test.tsx, new e2e/authentication.spec.ts, and updates to App.test.tsx, client.test.ts, contract.test.ts, mock.test.ts, environment.test.ts, layout.test.ts, e2e/topology.ts, e2e/bench-core.spec.ts and e2e/commands.test.ts. Records: 24 new requirements, 8 reconciled existing ones, the new spec REQ-FUNC-5fd2806f.md, the widened REQ-SEC-5d0cfe7e.md, both knowledge pages, and the CHANGELOG, TODO, vertical-slices, development-flow, delivery-reference and GUI README documents.
evidence: check --task TASK-a86b32ec --strict --run exit 0 over the final tree with test ok 69.4s, lint ok 4.5s, strict ok, errors=0 warnings=17 notes=1; that gate runs the Go suite, both Python suites, the interface test:ci and build, and the browser test:e2e, which fails rather than skips on a missing prerequisite. go test ./... green across 20 packages, go vet and gofmt clean. npm test:ci 219 tests in 11 files, lint clean at --max-warnings 0, format:check clean, tsc -b exit 0. npm test:e2e 12 browser tests passed in 20.6s against real Bench Core instances under WSL2. test --impacted --base HEAD exit 0. CGO_ENABLED=0 cross-compile green for linux/arm64, linux/amd64 and windows/amd64. The administration routes recorded a reproducible 404/405 red before implementation; the interface layer did not, which is recorded as a deviation.
issues: Seventeen accepted check warnings, all explained in the log and none a defect. Four deviations - the interface half was implemented before its component tests, the visual inspection used the accessibility tree and measured DOM geometry rather than a rendered screenshot, thirteen secret-in-diff hits are accepted as false positives, and CHANGELOG.md sits outside the configured layout globs. Token administration is administrator-only, so the tokens.*.self permissions still have no interface. There is still no audit-history route, no session browser, no token rotation and no durable login-attempt state. Settings, Plugins and About are deliberately absent rather than empty. The Slice 7 journal TASK-35b7b070 remains open at deliver, which is its owner's close-out.
next: none - delivered in cf77de8
needs_user: no
spec_compliance: disabled
implementation_quality: disabled
review_round:
