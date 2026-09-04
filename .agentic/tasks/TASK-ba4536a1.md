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

# TASK-ba4536a1 — Complete Network and remote administration CLI parity

stage: done
type: feature
path: full
requirements: REQ-FUNC-a1cefd8f, REQ-FUNC-6590e0c5, REQ-SEC-bfd1cf86, REQ-DATA-b4cdd70a, REQ-DATA-20645b45, REQ-UX-3f48f396
parent:
branch: feat/cli-remote-admin-parity
base: 45bb036646d3ba5862e33ebbef6ff7c9bedfaa72
opened: 2026-09-04

## Done checklist (current stage)

- [x] validated candidate confirmed unchanged; otherwise returned to validate
- [x] validation evidence, diff, exclusions, and outcome presented (in the journal and the session's final report)
- [x] delivery confirmed by the human (in advance, by the brief; RULING-003)
- [x] journal and report included in the delivery commit
- [x] agreed outcome delivered: one local commit, no PR (the human pushes)
- [x] close-out carried by the delivery commit, since nothing can fail after a local commit

## Frame

Problem: The `nilo` command line cannot list managed network operations, and
`nilo admin` only works with local write access to the data directory, so an
administrator of a Raspberry Pi appliance cannot administer users and API
tokens (including permanent deletion) securely from another machine. Refused
permanent deletions leave no audit trace of who tried what and why it was
refused.
Desired outcome: `nilo network operation list` exists; `nilo admin user|token`
gain an authenticated remote mode over the existing HTTP API with the full user
and token surface including permanent deletion behind explicit destructive
confirmation; every deletion attempt, applied or refused, is auditable with
stable display information; root help makes every command group discoverable;
documentation carries realistic remote examples.
Actors: appliance administrator at a terminal (interactive); automation script
(noninteractive, `--json`); Bench Core HTTP API (authority for permissions,
identity rules, audit); the identity service (owns deletion invariants).
Primary scenario: administrator runs `nilo admin user delete --server
https://nilo.local --token-file ~/.nilo/token --username operator` -> the
command lists the identity, prints exactly what will be deleted on stderr,
asks the username to be typed back (or requires `--confirm-delete` when stdin
is not a terminal) -> sends `DELETE /api/v1/users/operator` with the bearer
token -> exit 0 and the refreshed user listing on stdout; a refusal (last
administrator, owned tokens, not found, forbidden) exits 1 with the API's
stable error code and is recorded in Activity with the reason.
Examples: `nilo network operation list --server URL --token-file PATH --json`
prints the appliance's `GET /api/v1/network/operations` document unchanged;
`printf 'pw\npw\n' | nilo admin user create --server URL --token-file PATH
--username op --role operator` creates over `POST /api/v1/users`; `nilo admin
token create --server URL --token-file PATH --owner op --name ci --scopes
status.read --expires-in 720h` prints the secret once with a stderr warning;
`nilo admin token delete --id T --server URL --token-file PATH` without
`--confirm-delete` and without a terminal exits 2 and sends nothing.
Counterexamples: `nilo admin user list --data-dir D --server URL --token-file
PATH` is ambiguous and exits 2; `--acknowledge` is not an admin flag and never
authorizes a deletion; `nilo admin user delete --data-dir D` (local mode) is not
offered, because the local commands keep their documented "disable, never
delete" invariant; no flag anywhere accepts a token value or a password.
In scope: `nilo network operation list` (no filters: the API offers none);
remote mode for `nilo admin user list|create|set-role|enable|disable|reset-password|delete`
and `nilo admin token list|create|revoke|delete`; mode exclusivity rules;
interactive typed confirmation and `--confirm-delete`; human-readable and
`--json` output; exit codes 0/1/2; audit of refused deletions with reason and
actor/target display data, and target labels on the applied deletion events;
a concurrency test for last-administrator deletion and disable; root usage
listing `network`, `activity`, `diagnostics` and remote admin; documentation
(delivery reference, module README, network status doc, changelog).
Out of scope: new HTTP endpoints or filters; local-mode deletion; GUI changes;
deployment configuration; pushing; MCP; operator self-service tokens; any new
network behavior.
Constraints: reuse `internal/apiclient`, the route table permissions and the
identity service; never accept a secret on the command line; never print a
password; secret shown once on token creation only; local `--data-dir`
commands stay compatible; no push by the agent, and no commit unless the
human authorizes it (CLAUDE.md).
Success signals: the focused Go tests named in the brief pass; `go test ./...`
and the repository validation suite pass; `nilo` root usage names every group;
docs show remote Raspberry Pi examples with placeholders only.
Evidence: STATED - the brief enumerates the commands, security properties,
tests and delivery expectations. OBSERVED - `cmd/nilo/network.go` has status,
statistics, interface, wifi, vpn, device-networks and operation
status|acknowledge|confirm|rollback but no `operation list`;
`GET /api/v1/network/operations` exists (`internal/httpapi/server.go:236`,
`network_operations.go:378`) and takes no query parameters; `cmd/nilo/admin.go`
is local-only and offers no delete; `internal/httpapi/administration.go` and
`server.go:249-258` provide the full user and token API; `internal/apiclient`
reads the token only from a file; `httpapi/activity.go:23-57` already records
every 403 as `identity.access.denied`; `identity/service.go:440-482,635-664`
apply deletions inside `BEGIN IMMEDIATE` transactions and record
`identity.user.deleted` / `identity.token.deleted` but record nothing for a
refusal; `identity/service_test.go:1235` asserts refusals record no deleted
event; token deletion details carry `owner_id` only, no owner username; the GUI
calls `/api/v1/network/operations` (list) and every administration route.
INFERRED - remote administration needs no new server code beyond the audit of
refusals; the network CLI covers every network route except the operation
list. UNKNOWN - none decision-blocking.
Assumptions: the brief is the scope acceptance for the frame checkpoint (the
session is noninteractive); a refused-deletion audit record is written outside
the rolled-back transaction, like a failed login, and its own failure does not
change the refusal.
Unknowns: whether the human wants the agent to make the local commit despite
CLAUDE.md (raised at delivery, not before).
Decision-blocking questions: none.
Proposed behaviors: B1 `network operation list`; B2 remote admin mode with
exclusivity; B3 destructive confirmation contract; B4 audit of refused
deletions; B5 stable labels on deletion records; B6 root usage; B7 docs.
Path and reason: full - new externally observable behavior, a new CLI
interface, security- and audit-relevant changes.

| criterion | yes | no | evidence |
|---|---|---|---|
| no new externally observable behavior | | no | new commands and audit events |
| no new interface or data format | | no | new CLI flags and audit actions |
| no new dependency | yes | | standard library only |
| existing requirement covers the behavior | | no | REQ-FUNC-519eff58 is local-only |
| no security/data-handling change | | no | confirmation and audit changes |

## Requirement coverage

| Frame outcome | Atomic behavior | Requirement | Positive case | Boundary or failure | Verification | Design decision or no impact |
|---|---|---|---|---|---|---|
| B1 | `network operation list` calls GET /api/v1/network/operations | REQ-FUNC-a1cefd8f | listing printed, exit 0 | extra argument or filter flag exits 2; 403 exits 1 | stub test | D1: `list` branch before the id-taking actions in `networkOperation` |
| B2 | remote mode for every user/token subcommand | REQ-FUNC-6590e0c5 | each request mapped per spec | mode conflict/incomplete exits 2; bad token file exits 1; 401/403/404 exit 1 | stub tests + live simulation server test | D2: mode resolution in `admin.go`, remote implementation in `admin_remote.go` over `internal/apiclient` |
| B2 | token secret shown once with warning; --json pure | REQ-FUNC-6590e0c5 (inseparable: it is the create command's output contract) | secret on stdout once, warning on stderr | secret absent from every later listing | stub test | D2 |
| B3 | typed confirmation / --confirm-delete before DELETE | REQ-SEC-bfd1cf86 | DELETE sent after exact match or flag | mismatch exits 1; missing flag exits 2; --acknowledge rejected; nothing sent | stub tests | D3: confirmation runs after the listing read and before the DELETE, in `admin_remote.go` |
| B4 | refused deletion recorded with reason | REQ-DATA-b4cdd70a | one denied record per refusal | no deleted record; subject unchanged; concurrent delete+disable keeps one admin | service, HTTP and concurrency tests | D4: refusal classified after the transaction rolls back and recorded through the audit writer |
| B5 | applied deletion records carry labels | REQ-DATA-20645b45 | target label, actor username, owner username | readable after rows are gone; no secrets | service and HTTP tests | D5: `record` gains a label; owner username read inside the transaction |
| B6 | root usage names every group | REQ-UX-3f48f396 | usage names groups and both admin modes | unknown command exits 2 | binary test | D6: `writeUsage` rewritten |
| B7 | documentation | none (documentation, not behavior) | remote examples with placeholders | no real credentials | inspection at validate | D7: delivery reference section 3, README, network status doc, CHANGELOG |

## Design

Requirements coverage: D1 REQ-FUNC-a1cefd8f; D2 REQ-FUNC-6590e0c5; D3
REQ-SEC-bfd1cf86; D4 REQ-DATA-b4cdd70a; D5 REQ-DATA-20645b45; D6
REQ-UX-3f48f396. REQ-SEC-7861a0b2 (no token secret as an argument) gains
`admin.go` and `admin_remote.go` in its affected paths and one test; its prose
is unchanged.

Component responsibilities: Bench Core foundation only. `cmd/nilo` remains an
API client; it adds no authorization or identity rule. `internal/identity`
keeps every deletion invariant and gains the audit of refusals. `internal/httpapi`
is unchanged in behavior (its access guard already records 403s); only its
tests grow.

Interfaces and contracts: exactly as `specs/REQ-FUNC-6590e0c5.md`. Shared
options reuse the `networkOptions` names (`--server`, `--token-file`,
`--ca-file`, `--json`) so an operator learns one vocabulary. `adminIO` gains
`interactive bool` filled from `networkTerminal()` in `main.go`, so a test can
drive both modes with a buffer.

Execution flow (D2): `runAdminUser`/`runAdminToken` parse one flag set that
declares both the local and the remote options plus `--confirm-delete`, resolve
the mode (spec list) and dispatch: local -> the existing `withIdentityService`
closures, untouched; remote -> `remoteAdmin{client, options, streams}` methods
in `admin_remote.go`, one per subcommand, each building the request from the
mapping table, and a `render` step that prints the raw document for `--json`
or the readable rendering otherwise. A 204 is followed by the listing GET so
the operator sees the result, as the local command does.

Deletion (D3): `deleteSubject(kind, id)` reads the listing, describes the
subject on stderr (or says it is not listed), then requires consent:
`--confirm-delete`, or an exact typed match when `streams.interactive`; a
noninteractive run without the flag exits 2, a cancelled prompt exits 1, and in
both cases the only request made was the listing GET. `--acknowledge` is not
declared on the admin flag set, so the standard library rejects it as unknown.

Audit (D4, D5): `record` gains a `label` argument (the target label) and every
caller passes one where a display name is known. `DeleteUserAs` and
`DeleteTokenAs` wrap the transaction: on `ErrNotAdministrator`,
`ErrLastAdministrator`, `ErrUserHasTokens`, `auth.ErrUserNotFound` or
`auth.ErrTokenNotFound` they call `recordDeletionRefusal`, which writes one
`identity.user.delete_refused` / `identity.token.delete_refused` record with
outcome `denied`, severity `warning`, the actor (user or token kind, as the
applied path attributes it), `TargetLabel` = the requested username or token
id, and details `{reason, username|token_id, actor_username}`. Any other error
is a store failure, not a refusal, and is returned unrecorded. The refusal
record is written through the ordinary writer after the rollback; its failure
is ignored, exactly as a failed-login record is, because the refusal already
happened and must not depend on the store. Applied deletion details add
`actor_username`, and the token record adds `owner` (username read by
`UserByID` inside the same transaction; if the owner row is missing the value
is empty rather than an error).

Concurrency: no new mechanism. Both deletion paths already run inside
`BEGIN IMMEDIATE`; the new test proves a concurrent delete and disable across
two connections leave one enabled administrator and that the losing deletion
records its refusal.

Security and trust boundaries: the CLI holds the bearer token only in memory
via `apiclient`; passwords go from stdin to the JSON body and nowhere else; no
secret reaches an error message (apiclient errors carry status and code only),
an audit detail, a process argument or a test fixture string that is asserted
against output. `--ca-file` pins a private CA as `nilo network` does.

Compatibility: local mode is byte-compatible (same JSON, same flags, same exit
codes); the new flags are additive. New audit actions are additive; existing
`*.deleted` filters keep matching only applied deletions.

Observability: the refusal records are listable through `nilo activity list
--component identity --outcome denied`.

Test seams: `cmd/nilo/network_test.go` (D1, stub); `cmd/nilo/admin_remote_test.go`
(D2, D3, stub); `cmd/nilo/admin_live_test.go` (D2 permission, scope, revoked and
expired cases against a `nilo serve --profile simulation` subprocess seeded
with the local commands); `internal/identity/service_test.go` (D4, D5,
concurrency); `internal/httpapi/administration_test.go` (D4, D5 over the
routes); `cmd/nilo/main_test.go` (D6).

Alternatives rejected: a separate `nilo remote-admin` group (splits one
administration vocabulary in two); local-mode deletion (contradicts the
documented local invariant, RULING-001); recording refusals inside the
transaction (rolled back with it); a `--yes` flag shared with network
`--acknowledge` (the brief forbids a generic flag authorizing deletion).

Expected files: cmd/nilo/{admin.go, admin_remote.go, admin_remote_test.go,
admin_live_test.go, admin_test.go, network.go, network_test.go, main.go,
main_test.go}; internal/identity/{service.go, service_test.go};
internal/httpapi/administration_test.go; 07_Delivery/NILO_CLI_and_Authentication.md;
05_Software/nilo_assistant/README.md; 06_SoftwareDocu/Network_Service_Implementation_Status.md;
CHANGELOG.md; .agentic/knowledge/bench-core-foundation.md (validate).

Risks: the live subprocess test depends on a free loopback port and the built
binary, like the existing serve tests; the expired-token case sets the expiry
in the past through the database directly, as the admin tests already open it.

Open decisions: none. No architecture drawing is owed: the component
structure is unchanged (the CLI already sits behind `internal/apiclient`).

## Log

- 2026-09-04 frame: scope accepted from the user's written brief (noninteractive session; the brief enumerates commands, security properties, tests and delivery). Full path: new CLI interface, new audit events, security-relevant confirmation contract. Worktree F:/NILO_cli-remote-admin-parity on branch feat/cli-remote-admin-parity from master 45bb036. Toolchain linked from the main checkout (junctions 02_Tools/runtime, 02_Tools/cache); `go build ./...` green.
- 2026-09-04 specify: created 6 requirements (REQ-FUNC-a1cefd8f, REQ-FUNC-6590e0c5, REQ-SEC-bfd1cf86, REQ-DATA-b4cdd70a, REQ-DATA-20645b45, REQ-UX-3f48f396), one spec (specs/REQ-FUNC-6590e0c5.md, shared with REQ-SEC-bfd1cf86), reused 1 (REQ-SEC-7861a0b2, paths only), revised 0, retired 0. Documentation has no requirement of its own.
- 2026-09-04 design: note in `## Design` (D1-D7); no architecture change, no drawing owed. Refusal audit is written after the rollback through the ordinary writer, like a failed login.
- 2026-09-04 build red (REQ-FUNC-a1cefd8f, REQ-FUNC-6590e0c5, REQ-SEC-bfd1cf86, REQ-UX-3f48f396): command=`go test ./cmd/nilo -run 'TestOperationList|TestRootUsage|TestRemote|TestNoninteractiveDeletion|TestDeletingAnUnlisted|TestNoAdminFlag' -count=1`; exit=1; expected missing behavior=`operation list` is parsed as an action that needs an id, the admin flag sets do not define --server/--token-file/--confirm-delete, the root usage names neither activity nor diagnostics.
- 2026-09-04 build red (REQ-DATA-b4cdd70a, REQ-DATA-20645b45): command=`go test ./internal/identity -run 'TestRefusedDeletionsAreRecorded|TestConcurrentDeleteAndDisable|TestDeletionsAreAuditedOnce' -count=1` and `go test ./internal/httpapi -run TestRefusedAndAppliedDeletionsReadFromTheHistoryAlone -count=1`; exit=1 each; expected missing behavior=no identity.user.delete_refused record after a refusal, deletion records carry no target label or actor/owner username.
- 2026-09-04 build green (all six requirements): `go test ./cmd/nilo ./internal/identity ./internal/httpapi -count=1` exit 0 (the CLI package includes the live simulation-server test: viewer, scope-narrowed, revoked and expired tokens refused; remote create and delete applied; refused deletion recorded with actor kind token). `gofmt -l` empty; `go vet ./...` exit 0. `python .agentic/aegis.py test --impacted --base HEAD` fell back to the full Go suite (documentation edits are unlinked): every package green except internal/integration/manifest, which failed only because the fresh worktree lacked the fake-integration venv; after copying it from the main checkout, `go test ./internal/integration/manifest ./internal/acceptance -count=1` exit 0. Environment gap, not a candidate problem.
- 2026-09-04 build check: `check` reports secret-in-diff on cmd/nilo/admin_remote.go and admin_remote_test.go: the hits are the JSON field name "password" placed in the request body and the fixed test constants named *Password; no credential value is in the source. outside-layout on CHANGELOG.md: the root changelog is not in the aegis.toml docs globs (pre-existing configuration). Both accepted as advisory noise, not deviations. Evolution reviewed: 10 files, all expected.
- 2026-09-04 validate: diff reviewed hunk by hunk against scope, requirements, design, error paths, portability and secrets; no unrequested work. Knowledge page `.agentic/knowledge/bench-core-foundation.md` migrated toward the template (Routing, Purpose and boundaries, Invariants added) and reconciled: public interface, key behaviors, deletion audit paragraph, files. Requirements re-read: all six active with true paths and links; REQ-SEC-7861a0b2 and REQ-FUNC-6590e0c5 paths extended. No manual verification owed. Worktree provisioning for the full suite: fake-integration venv copied, node_modules and every runtime subdirectory junctioned to the main checkout (WSL follows NTFS junctions).
- 2026-09-04 validate final proof: `powershell -File 02_Tools/env/with-tools.ps1 python .agentic/aegis.py check --task TASK-ba4536a1 --strict --run`; exit 0; `run: test ok (318.9s)` (scripts unit tests, `go test ./...`, Python suites, GUI `test:ci`, `build` and `test:e2e`), `run: lint ok (9.2s)`, `strict: ok`. Remaining warnings: secret-in-diff (field names and test constant names, no values), outside-layout CHANGELOG.md (pre-existing aegis.toml layout), and unticked-deliver/journal-header on five older tasks (pre-existing, not this task). Evolution refreshed after the last candidate edit.
- 2026-09-04 deliver: candidate unchanged since the final proof (only `.agentic/` journal edits after it). Delivery authorized in advance by the brief (RULING-003): one local commit, no push. CLI coverage still missing after this task, by design of the brief: `GET /api/v1/status`, `/integrations`, `/devices` and `/auth/session` have no command; browser login/logout are session-only; an absolute token expiry (`expires_at`) is reachable only through `--expires-in`.
- 2026-09-04 close-out: delivery report appended (28 files, checks errors=0). Stage set to done before the single local delivery commit, which carries this close-out; no push and no PR (RULING-003). The commit hash is reported in the session's final message and in `git log` on feat/cli-remote-admin-parity.

## Rulings

- RULING-001 | date=2026-09-04 | decision=Local `--data-dir` admin commands keep their documented invariant and offer no deletion; permanent deletion is remote-mode only. | reason=The delivery reference and README state that the local commands disable and never delete, and the brief asks for remote parity with the HTTP API rather than a change to the local boundary. | source=repository | supersedes=none
- RULING-002 | date=2026-09-04 | decision=The user's brief is treated as the frame-stage scope acceptance. | reason=The session is noninteractive and the brief already fixes scope, exclusions, security properties and tests; stopping would deliver nothing. | source=user | supersedes=none
- RULING-003 | date=2026-09-04 | decision=The delivery outcome is one local commit on feat/cli-remote-admin-parity in the worktree, staged by explicit path; no push, no PR. | reason=The brief says "Commit the completed files locally ... Do not push"; AGENTS.md permits a local commit when the user authorizes it, and the brief is that authorization, while CLAUDE.md's digest forbids agent commits in general. The conflict is stated in the delivery report so the human can revert the commit if the digest was meant to win. The human pushes. | source=user | supersedes=none

## Handoff

current: done
status: ready
next: the human reviews the local commit on feat/cli-remote-admin-parity and pushes or merges it; nothing remains for an agent
must_read: .agentic/tasks/TASK-ba4536a1.md
blockers: none

## Stage report

stage: deliver
result: completed
current: done
changed: cmd/nilo/{admin.go,admin_remote.go,main.go,network.go} and tests, internal/identity/service.go and tests, internal/httpapi/deletion_audit_test.go, 07_Delivery/NILO_CLI_and_Authentication.md, 05_Software/nilo_assistant/README.md, 06_SoftwareDocu/Network_Service_Implementation_Status.md, CHANGELOG.md, knowledge page, six new requirements, one revised requirement, one spec
evidence: check --task TASK-ba4536a1 --strict --run exit 0: test ok 318.9s (Go, Python, GUI unit, build, e2e), lint ok 9.2s, strict ok; go vet exit 0; gofmt clean
issues: advisory warnings only: secret-in-diff (field and constant names), outside-layout CHANGELOG.md (pre-existing layout), older tasks' unticked deliver items
next: the human pushes or merges the local delivery commit
needs_user: yes - review and push the local commit; revert it if CLAUDE.md's no-commit digest was meant to override the brief (RULING-003)
spec_compliance: disabled
implementation_quality: disabled
review_round:

## Delivery report (2026-09-04)

- stage: deliver; type: feature; path: full; branch: feat/cli-remote-admin-parity
- requirements:
  - REQ-FUNC-a1cefd8f - List managed network operations from the command line
  - REQ-FUNC-6590e0c5 - Administer identities and API tokens remotely from the command line
  - REQ-SEC-bfd1cf86 - Require explicit destructive confirmation for command-line permanent deletion
  - REQ-DATA-b4cdd70a - Audit refused permanent deletions with their reason and stable subject labels
  - REQ-DATA-20645b45 - Carry stable display labels on identity deletion audit records
  - REQ-UX-3f48f396 - Make every command group discoverable from the root usage
- deviations: none recorded
- explicit review: disabled; existing validation remains required
- diff vs task base 45bb036: 28 file(s) changed, 5761 line(s) added
- checks: errors=0 warnings=10 notes=1
- checklist: 0/6 done; open: validated candidate confirmed unchanged; otherwise returned to validate; validation evidence, diff, exclusions, and outcome presented; delivery confirmed by the human; journal and report included in the delivery commit; agreed outcome delivered, normally a PR; close-out committed after the outcome succeeded, or carried by the delivery commit where nothing could fail after it
