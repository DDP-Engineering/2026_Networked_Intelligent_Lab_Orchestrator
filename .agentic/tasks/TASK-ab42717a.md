<!--
Copyright (c) 2026 DDP Engineering
This software is licensed under the PolyForm Noncommercial License 1.0.0.
Commercial use requires a separate commercial license.
See the LICENSE.txt in the .agentic folder for full license details.
SPDX-License-Identifier: LicenseRef-PolyForm-Noncommercial-1.0.0
-->


# TASK-ab42717a - Configure the appliance password policy

stage: done
type: feature
path: full
interaction: autopilot
requirements: REQ-SEC-492e15ed, REQ-SEC-1583a012, REQ-SEC-06c320c8, REQ-SEC-fe469bac, REQ-SEC-13b2e0d5, REQ-SEC-007c3be6, REQ-DATA-757ca87b, REQ-FUNC-f9dbf9aa, REQ-FUNC-03c21252, REQ-UX-5e428717, REQ-SEC-0ecda0e7
parent:
branch: ddp/nilo-password-policy
base: d0c5c81438c02add3dd99b9ba7f48f218bada641
opened: 2026-09-15

## Done checklist (current stage)

- [x] validated candidate confirmed unchanged; otherwise returned to validate
- [x] validation evidence, diff, exclusions, and outcome presented
- [x] delivery confirmed by the human
- [x] journal and report included in the delivery commit
- [x] agreed outcome delivered, normally a PR
- [x] close-out committed after the outcome succeeded, or carried by the
      delivery commit where nothing could fail after it

## Frame

Problem:
The appliance password rule is two compiled-in constants, `MinPasswordBytes = 12`
and `MaxPasswordBytes = 1024`, and the same bounds are repeated at the HTTP
boundary. A site whose policy needs a longer minimum, or a composition rule,
cannot express either without rebuilding the appliance. There is no persisted,
administrator-settable appliance policy surface for passwords at all.

Desired outcome:
The appliance holds exactly one password policy - a minimum length and an
optional character-class requirement - that an administrator reads and changes
through the ordinary authenticated, audited administration surface, that
survives a restart, that governs setting a password and never checking one, and
that cannot be set below an absolute floor.

Actors:
- Administrator: reads and changes the policy; sets passwords held to it.
- Operator, Viewer, unauthenticated caller: can neither read nor change it.
- Every identity: has its password held to the policy when that password is set.
- An unauthenticated login attempt: still bounded by the maximum length.

Primary scenario: trigger -> system action -> observable result
An administrator sends a policy with a higher minimum length to the
administration surface -> the appliance validates it against the absolute
floor, persists it, and appends one Activity audit event carrying no secret ->
the next password set anywhere - HTTP identity administration or the `nilo`
command line, local or remote mode - is refused unless it meets the new
minimum, while every existing identity continues to sign in unchanged.

Examples:
- Policy minimum raised to 16. `POST /api/v1/identity/users` with a 14-byte
  password is refused with a message naming the configured minimum; the same
  create through `nilo admin user create --data-dir ...` is refused too.
- Policy requires a digit. A password of 20 letters is refused on both surfaces.
- An identity whose 12-byte password was set under the shipped policy signs in
  successfully after the minimum is raised to 16.
- The credential published with the appliance image (`admin` /
  `admin_password`, 14 bytes) still signs in on an appliance whose policy has
  since been raised past 14.

Counterexamples:
- A policy minimum of 8 is refused with a message naming the floor; the stored
  policy is unchanged.
- An Operator session reading the policy route receives the ordinary
  authorization refusal, not the policy.
- A 2000-byte password at the login route is still refused before the Argon2
  work runs, whatever the policy says.

In scope:
- A policy editor on the browser Authentication page, in all three languages.
- One persisted appliance password policy: minimum length plus an optional
  character-class requirement.
- An administrator-only, audited read and change surface for it: an HTTP route
  pair declaring its permission in the one route table, and a `nilo` command
  line client of that route.
- Separating the shared `validatePassword` so the minimum is enforced on the
  set path only, and the maximum stays on the verification path.
- An absolute floor the configured minimum cannot go below.
- Enforcement wherever a password is set: `Bootstrap`, `CreateUser`,
  `ResetPassword`, over HTTP and over the command line in local and remote mode.
- Updating the affected requirements, specs, component page and delivery
  documentation.

Out of scope:
- Password expiry, rotation schedules, reuse history, forced change at next
  sign-in (STATED out of scope).
- Breached-password blocklists (STATED out of scope).
- Per-identity or per-role policies (STATED out of scope).
- Changes to authentication, authorization, session or CSRF handling (STATED).
- The first-boot published credential itself, issue #13 (STATED).
- Client-side password validation in the browser. The browser keeps surfacing
  the appliance's refusal rather than re-implementing the rule; only the policy
  editor itself is added there.

Constraints:
- OBSERVED `.agentic/context/project.md`: Bench Core is authoritative; every
  protected route declares its permission in `internal/httpapi/server.go`; no
  response, log, audit event, export or error message carries a password or
  verifier; an identity change and its audit event commit in one transaction.
- OBSERVED `.agentic/knowledge/bench-core-foundation.md`: the command line is an
  API client and adds no authorization or identity rule of its own.
- OBSERVED `internal/identity/service.go:869-892`: password hashing runs outside
  any transaction because it is deliberately slow and must not hold the write
  lock. Policy loading has to respect that ordering.
- OBSERVED `internal/store/sqlite/migrations.go:79-140`: migrations are embedded
  and digest-checked; a new table is a new numbered migration file.
- OBSERVED project rule: superseded functionality is removed directly during
  active development; no compatibility aliases.

Success signals:
- Each acceptance line of the ticket is settled by a named automated test.
- The full suite and `check --strict` are green on this branch.
- No test needs an existing identity's stored verifier to be rewritten when the
  policy tightens.

Evidence:
- STATED (issue #16): the problem, the required behavior, every acceptance line,
  the out-of-scope list, that the floor's value is a decision for this ticket,
  and the NIST SP 800-63B consideration about character-class rules.
- OBSERVED `internal/auth/password.go:16-20,86-92`: the two constants, and
  `validatePassword` called from both `HashPassword` and `VerifyPassword`.
- OBSERVED `internal/httpapi/administration.go:393-398,443-445`: the duplicated
  bounds check in `readPassword` and its `passwordMessage()`.
- OBSERVED `internal/identity/service.go:872-892`: `prepareUser` hashes for
  `Bootstrap` and `CreateUser`; `:388-396` `ResetPasswordAs` hashes directly.
  Both are the set path.
- OBSERVED `internal/identity/service.go:205-211`: `NewService` calls
  `auth.HashPassword` on a fixed placeholder to build the timing dummy
  verifier, at construction time and before any repository read.
- OBSERVED `internal/activity/policy.go` with
  `internal/store/sqlite/activity_repository.go:136-171`,
  `internal/httpapi/activity.go:278-328` and
  `internal/httpapi/server.go:341-347`: the appliance already has one
  administrator-settable persisted policy - Activity retention - as a singleton
  row of JSON, a dedicated permission, a GET/PUT route pair and one audit event
  per change. It is the pattern this policy should follow.
- OBSERVED `internal/auth/role.go:50-96`: `PermissionActivityRetentionManage`
  is administrator-only; the role matrix is compiled in.
- OBSERVED `internal/store/sqlite/migrations/013_commissioning.sql`: the
  singleton-table convention, `STRICT`, with a `CHECK` on the stored value.
- OBSERVED `internal/appliance/published_credential.go:18-20`: the published
  credential is `admin` / `admin_password`, 14 bytes, above the shipped
  minimum of 12.
- OBSERVED `cmd/nilo/admin.go:112-186,397-423`: the command line reads
  passwords from standard input and, in local mode, calls `identity.Service`
  directly through `app.Open`; `cmd/nilo/admin_remote.go:191-201` reads the
  password before contacting the appliance in remote mode.
- OBSERVED `cmd/nilo/activity.go:115-141`: `nilo activity retention` is the
  precedent for administering a persisted policy from the command line, as an
  authenticated API client.
- OBSERVED `05_Software/nilo_gui/src/components/AdministrationForms.tsx` and
  `src/i18n/en.ts`: the browser interface applies no client-side length rule and
  surfaces the appliance's refusal message. Only `src/api/mock.ts:1490,1551`
  repeats 12, in the mock backend.
- OBSERVED `.agentic/requirements/security/REQ-SEC-0ecda0e7.toml`: the active
  requirement says "bounded password input" and links
  `TestPasswordFunctionsRejectOutOfBoundsByteLengths`, which asserts today's
  shared check on both functions. Splitting the check changes this contract.
- INFERRED: enforcing the policy inside `identity.Service` rather than inside
  `auth.HashPassword` covers the HTTP routes and both command-line modes at one
  seam, because local mode calls the same service and remote mode calls the
  same routes.
- INFERRED: the policy must not be applied to the construction-time dummy
  verifier, which is not a user password and is hashed before any policy can be
  read.
- STATED (2026-09-15, RULING-003): the browser interface gets a policy editor
  on the Authentication page in this task.

Assumptions:
- The floor is a compiled-in constant, not itself configurable.
- The policy is read from the database on each password set, so a change takes
  effect immediately without restarting or invalidating anything.
- The `nilo` policy command is a remote-mode API client, like
  `nilo activity retention`, rather than a local `--data-dir` command.

Unknowns:
- Whether an appliance upgraded from an earlier database should start at the
  shipped default policy. Proposed: yes - an absent record means the default,
  the same way an absent Activity policy does.

Decision-blocking questions: all three resolved by the user on 2026-09-15,
recorded as RULING-001, RULING-002 and RULING-003.
1. The absolute floor is 8 bytes, the NIST SP 800-63B minimum for a
   user-chosen secret. The shipped default minimum stays 12, so an appliance
   that is never configured behaves exactly as it does today; the floor bounds
   only what an administrator may deliberately choose. An administrator can
   therefore configure a minimum below the rule the appliance ships with, which
   is the accepted cost of making the minimum configurable at all.
2. The character-class requirement is a minimum count of distinct classes
   present, drawn from lowercase, uppercase, digit and symbol, and is 0 by
   default, so the shipped appliance applies no composition rule.
3. The browser interface gets a policy editor in this task, on the
   Authentication page, with its English, German and Romanian strings, its
   mock backend behavior and browser-stage coverage. Browser GUI foundation
   is therefore a second component in scope.
Proposed behaviors:
- B1. The appliance holds one password policy - minimum length and a set of
  required character classes - persisted in its database and read on every
  password set. An absent record means the shipped default: minimum 12, no
  class requirement.
- B2. An administrator reads the policy and replaces it through an
  administrator-only route on the authenticated `/api/v1` surface, and through
  a `nilo` command that is a client of that route.
- B3. A policy whose minimum is below the absolute floor is refused, with a
  message naming the floor, and the stored policy is left unchanged.
- B4. Setting a password - bootstrap, create, reset, over HTTP or the command
  line - is refused unless the password satisfies the current policy, with a
  message naming what it failed.
- B5. Checking a stored verifier never consults the policy. The maximum length
  stays on the verification path and keeps bounding the Argon2 work an
  unauthenticated login attempt can ask for.
- B6. Every applied policy change appends exactly one Activity audit event
  naming the actor and the before and after policy, and carrying no password,
  verifier or secret.

Path and reason: full.

| criterion | yes | no | evidence |
|---|---|---|---|
| no new externally observable behavior | | no | a new administrator-only policy route and a new command-line command |
| no new interface or data format | | no | a new JSON policy payload and a new persisted table |
| no new dependency | yes | | everything is in the existing Go module |
| an existing requirement already covers the behavior | | no | REQ-SEC-0ecda0e7 states a fixed bounded input, not a configurable policy |
| no security or data-handling behavior changes | | no | this is exactly a change to the credential rule and to where it is enforced |

Four criteria answer no, so the full path applies.

## Requirement coverage

| Frame outcome | Atomic behavior | Requirement | Positive case | Boundary or failure | Verification | Design decision or no impact |
|---|---|---|---|---|---|---|
| B1 one policy, read on every set | an appliance with no recorded policy applies minimum 12 and no class rule; a recorded policy survives a restart | REQ-DATA-757ca87b | changed policy read back after reopening the same data directory | never-recorded policy reads the shipped default | automated store and reopen test | new singleton table and migration 015 |
| B4 setting a password is held to the policy | a password set through any interface is refused unless it meets the configured length and class count | REQ-SEC-492e15ed | a password meeting both is accepted and signs in | one byte short, and long enough with too few classes, each refused naming the unmet part | automated tests on the HTTP routes and the local and remote command line | the check lives in identity.Service, the one seam both surfaces pass through |
| B5 checking never consults the policy | an identity created under an earlier policy still signs in after the policy is tightened | REQ-SEC-1583a012 | sign-in succeeds and reads a protected route | the same identity's next password set is refused | automated tightening test | auth.validatePassword is split; VerifyPassword loses the minimum |
| B5 the maximum stays on the verification path | a password longer than 1024 bytes is refused before any key derivation | REQ-SEC-06c320c8 | refused under the shipped policy | still refused under a changed policy | automated oversize test | the maximum is a compiled bound, never policy |
| B3 the floor | a proposed policy below the floor or outside the class range is refused and changes nothing | REQ-SEC-fe469bac | a policy at the floor of 8 is accepted | 7 refused naming the floor; class count 5 refused; stored policy unchanged | automated refusal test that reads the policy back | floor 8 compiled in, default 12 (RULING-001) |
| B2 only an administrator | an actor without the password-policy permission can neither read nor change it | REQ-SEC-13b2e0d5 | an administrator session performs both | Operator, Viewer, unscoped token and anonymous each refused with no policy in the body | automated authorization tests per caller | new permission password.policy.manage in the compiled role matrix |
| B6 audit | an applied change appends exactly one audit event with before and after and no secret | REQ-SEC-007c3be6 | one event carrying actor and both policies | a refused change appends nothing | automated Activity read plus the credential-redaction check | follows the activity.retention.changed precedent |
| B2 the HTTP surface | the authenticated interface serves the policy and stores a complete replacement | REQ-FUNC-f9dbf9aa | read returns the default; a complete replacement is echoed and persists | a partial body is refused without changing the stored policy | automated HTTP test over a real database | spec REQ-FUNC-f9dbf9aa.md carries the wire format |
| B2 the command-line surface | the nilo command prints the policy with no values given and submits it with both | REQ-FUNC-03c21252 | both values submitted and stored | exactly one value exits 2 with usage and contacts nothing | automated command-line test against a test appliance | remote mode only, like nilo activity retention |
| RULING-003 the browser surface | the Authentication page shows the current policy and offers to change both values | REQ-UX-5e428717 | a submitted change calls the client with those values and the shown policy follows the response | a refused change shows the appliance's message and leaves the previous values shown | automated component test plus the three language catalogs | new client contract calls and mock backend bounds |
| B5 revised contract | a stored verifier is checked against a password bounded by the fixed maximum alone | REQ-SEC-0ecda0e7 (revised) | a correct password of any length within the maximum verifies | a password over the maximum is refused on both paths | the existing authentication tests, with the out-of-bounds case narrowed to the maximum | deliberate revision: the shared minimum check on the verification path is removed |

Every row maps to exactly one requirement. REQ-SEC-0ecda0e7 is an existing
active requirement this task deliberately revises rather than duplicates: its
subject is what the appliance stores and the bound it accepts, and narrowing
that bound to the maximum is the same behavior restated correctly, not a new one.

## Design

Two components change: **Bench Core foundation** owns the policy, its
persistence, its authorization and both machine surfaces; **Browser GUI
foundation** owns the editor. The policy is one appliance-wide record, so it
joins the appliance state already kept in SQLite beside the management access
and commissioning records, not `internal/config`, which resolves once at
startup (REQ-DATA-757ca87b).

### The one decision that shapes everything else

The policy check goes in `identity.Service`, not in `auth.HashPassword`
(REQ-SEC-492e15ed). Every surface that sets a password passes through that
service: the HTTP administration routes call it, `nilo admin user` in local
mode calls it directly through `app.Open`, and remote mode calls the routes. One
seam therefore covers all three. Putting the check in `auth.HashPassword`
instead would also police `NewService`'s construction-time placeholder verifier,
which exists so an unknown username costs the same time as a known one and is
hashed before any policy can be read.

`auth` keeps the policy *type* and its rules, because that is where the
credential model lives, but it never reads the database.

### Interfaces and contracts

New `internal/auth/password_policy.go` (REQ-SEC-492e15ed, REQ-SEC-fe469bac):

- `PasswordPolicy{MinimumLength, MinimumCharacterClasses int}`
- `PasswordPolicyFloorBytes = 8`, `DefaultMinimumPasswordBytes = 12`
- `DefaultPasswordPolicy() PasswordPolicy`
- `(PasswordPolicy) Validate() error` - the policy's own bounds, wrapping
  `ErrPasswordPolicyInvalid`
- `(PasswordPolicy) Check(password []byte) error` - the set path, wrapping
  `ErrPasswordRejected`

`internal/auth/password.go` loses `MinPasswordBytes` entirely and
`validatePassword` becomes `validatePasswordLength`, enforcing a non-empty
password and the unchanged `MaxPasswordBytes` (REQ-SEC-06c320c8,
REQ-SEC-0ecda0e7). `VerifyPassword` keeps only that bound; the minimum never
runs there again (REQ-SEC-1583a012).

`identity.Service` gains (REQ-FUNC-f9dbf9aa):

- `PasswordPolicy(ctx) (auth.PasswordPolicy, error)`
- `SetPasswordPolicy(ctx, actor, policy) error` / `...As` for token attribution

and `identity.Repository` gains `LoadPasswordPolicy(ctx)` and
`SavePasswordPolicy(ctx, policy, at)`. `prepareUser` and `ResetPasswordAs` load
the policy and `Check` the password *before* hashing, outside any transaction,
preserving the existing rule that the deliberately slow hash never holds the
database write lock.

`auth.PermissionPasswordPolicyManage = "password.policy.manage"` joins the
compiled role matrix for Administrator only (REQ-SEC-13b2e0d5). Both routes
declare it in the one route table:
`GET`/`PUT /api/v1/password-policy` in `internal/httpapi/server.go`.

The wire format, refusal codes, the migration and the command line are in
`.agentic/specs/REQ-FUNC-f9dbf9aa.md` and are not repeated here.

### Data ownership and state transitions

Migration `015_password_policy.sql` creates a singleton `STRICT` table with
`CHECK` constraints mirroring the Go bounds, following `013_commissioning.sql`.
**No row is seeded.** An absent row means the shipped default, exactly as an
absent Activity retention policy does, so an appliance upgraded from an earlier
schema keeps behaving as it did (REQ-DATA-757ca87b). The `CHECK` constraints are
a second line under the Go validation, not the primary one: the error a client
sees is always the Go message naming the bound.

`IdentityRepository` implements both methods. It is already bound-aware - a
repository handed to an administrative transaction runs its statements inside
it - so the save joins that transaction for free.

### Execution flow and the audit event

A policy change runs through `WithinAdministrativeTransaction`, so the save and
its audit event commit together or not at all (REQ-SEC-007c3be6). This is
stricter than the Activity retention precedent, which records its event on a
separate call, and it matches the component invariant that an administrative
change and its audit event are one transaction. A refused change never reaches
the transaction, so it appends nothing.

`requireAdministrator` runs inside that transaction as every other
administrative change does, giving the same defence as the route's permission
gate against a role that changed between authorization and application.

### Failure recovery

Every refusal is a plain error before any write: a policy outside its bounds,
and a password failing the policy, both leave the database untouched. There is
no partial state to clean up and nothing to retry.

### Security and trust boundaries

`readPassword` in `internal/httpapi/administration.go` stops duplicating the
bounds. It keeps only the maximum, which is what lets the boundary refuse an
oversized body before the slow hash; the policy refusal arrives as a typed
service error mapped to the existing `invalid_password` code. That removes the
duplication the ticket names without losing the fast rejection.

No policy value appears in an authorization refusal, and no message, audit
event or log line carries a password or verifier.

### Compatibility and migration

No compatibility alias is added, per the project's active-development rule:
`auth.MinPasswordBytes` is removed rather than deprecated, and the GUI mock
backend stops hardcoding 12.

### Browser

`PasswordPolicyPanel` in `src/components/authentication/`, rendered by
`UsersPage` above the identity listing and only for a session holding
`password.policy.manage` (REQ-UX-5e428717). `contract.ts` gains the permission
constant, the path and the parsed type; `client.ts` gains
`getPasswordPolicy` / `setPasswordPolicy` in the shape
`getActivityRetention` / `setActivityRetention` already use; `mock.ts` applies
the same bounds so the page behaves identically against it. The page shows the
appliance's refusal beside the control and re-reads nothing on failure, so the
shown values stay the stored ones.

### Test seams

| Requirement | Seam |
|---|---|
| REQ-SEC-492e15ed | `internal/identity/service_test.go`, `internal/httpapi/administration_test.go`, `cmd/nilo/admin_test.go` |
| REQ-SEC-1583a012 | `internal/identity/service_test.go`, `internal/auth/password_test.go` |
| REQ-SEC-06c320c8 | `internal/auth/password_test.go` |
| REQ-SEC-fe469bac | `internal/auth/password_policy_test.go`, `internal/httpapi/administration_test.go` |
| REQ-SEC-13b2e0d5 | `internal/httpapi/administration_test.go` |
| REQ-SEC-007c3be6 | `internal/httpapi/administration_test.go` |
| REQ-DATA-757ca87b | `internal/store/sqlite/identity_repository_test.go` |
| REQ-FUNC-f9dbf9aa | `internal/httpapi/administration_test.go` |
| REQ-FUNC-03c21252 | `cmd/nilo/admin_remote_test.go` |
| REQ-UX-5e428717 | `src/components/AuthenticationPage.test.tsx`, `src/api/mock.test.ts` |

### Alternatives and tradeoffs

- **Policy inside `auth.HashPassword`.** Rejected: it would police the timing
  placeholder verifier and would force `auth` to read the database.
- **Caching the policy in the service.** Rejected: a change would then need
  invalidation across processes, since `nilo admin` in local mode opens the same
  data directory beside a running server. One indexed singleton read per
  password set is invisible beside an Argon2 derivation.
- **Reusing `PermissionUsersManage`.** Rejected: REQ-SEC-13b2e0d5 wants the
  policy separable from ordinary identity administration, and a token scoped to
  create users should not be able to weaken the rule those users are held to.
- **A named required-class set rather than a count.** Decided against by
  RULING-002.
- **A third Authentication view for the policy.** Rejected: it would change the
  shell navigation for one two-field form; the policy belongs beside the
  identities it governs.

### Expected files

New: `internal/auth/password_policy.go`, `internal/auth/password_policy_test.go`,
`internal/store/sqlite/migrations/015_password_policy.sql`,
`src/components/authentication/PasswordPolicyPanel.tsx`.

Changed: `internal/auth/password.go`, `internal/auth/password_test.go`,
`internal/auth/role.go`, `internal/auth/role_test.go`,
`internal/identity/service.go`, `internal/identity/service_test.go`,
`internal/store/sqlite/identity_repository.go`,
`internal/store/sqlite/identity_repository_test.go`,
`internal/store/sqlite/migrations.go`,
`internal/httpapi/administration.go`, `internal/httpapi/administration_test.go`,
`internal/httpapi/server.go`, `cmd/nilo/admin.go`, `cmd/nilo/admin_remote.go`,
`cmd/nilo/admin_test.go`, `cmd/nilo/admin_remote_test.go`,
`src/api/contract.ts`, `src/api/client.ts`, `src/api/mock.ts`,
`src/components/authentication/UsersPage.tsx`,
`src/components/AuthenticationPage.test.tsx`, `src/i18n/{en,de,ro}.ts`,
`07_Delivery/NILO_CLI_and_Authentication.md`.

No architecture drawing is owed: this adds a record and a route inside the
existing composition and changes no component structure.

### Risks

- **A test elsewhere asserts the old shared bound.** `auth.MinPasswordBytes` has
  two call sites, but tests may assert the message. Build greps for the literal
  and updates what it finds; validate's full suite is the backstop.
- **Fixtures with short passwords.** Existing tests that create identities with
  a password under 12 bytes would newly fail only if the default changed, and it
  does not. The risk is a test that sets a policy and leaves it set for a shared
  fixture; each test opens its own data directory, so this stays contained.

### Open decisions

None. RULING-001 to RULING-003 settled the three that were open.


## Log

- 2026-09-15 frame: framed issue #16, a configurable appliance password policy.
  Full path: a new administrator-only route, a new persisted record, and a change
  to where the credential rule is enforced. No child tasks - one tightly coupled
  change across auth, identity, store, httpapi and cmd. Branch
  ddp/nilo-password-policy was established before this task and the user asked for
  the work on it, so no task branch was created. Scope presented for acceptance
  with three decision-blocking questions: the floor's value, the class rule's
  shape and default, and whether the browser gets a policy editor.
- 2026-09-15 frame: scope accepted. Floor 8 with the shipped default staying 12,
  a minimum distinct-class count defaulting to 0, and a browser policy editor in
  scope (RULING-001 to RULING-004). Stage advanced to specify.
- 2026-09-15 specify: 10 requirements created (6 security, 1 data, 2 functional,
  1 ux), 1 revised, 0 reused, 0 retired. REQ-SEC-0ecda0e7 was revised because the
  shared minimum check it links is being removed from the verification path: its
  prose now bounds password input by the fixed maximum and its verification no
  longer claims a minimum is enforced when a verifier is checked. One spec,
  .agentic/specs/REQ-FUNC-f9dbf9aa.md, carries the policy model, the wire format,
  the refusal codes, the migration and the command line, because seven behaviors
  are judged against one rule and it has to be written down once.
- 2026-09-15 specify: stage advanced to design; the user asked to run to delivery
  without stage prompts and not to commit (RULING-005, RULING-006).
- 2026-09-15 design: note in ## Design, with .agentic/specs/REQ-FUNC-f9dbf9aa.md
  carrying the formats. The shaping decision is that the policy check lives in
  identity.Service, the one seam the HTTP routes and both command-line modes all
  pass through. Stage advanced to build.
- 2026-09-15 build red: REQ-SEC-492e15ed REQ-SEC-fe469bac REQ-SEC-1583a012
  REQ-SEC-06c320c8; command=`go test ./internal/auth/`; exit=1; expected missing
  behavior=auth.PasswordPolicy, its floor constant and its two error sentinels do
  not exist, so the policy model tests do not compile.
- 2026-09-15 build green: REQ-SEC-492e15ed REQ-SEC-1583a012 REQ-SEC-06c320c8
  REQ-SEC-fe469bac REQ-DATA-757ca87b; command=`go test ./internal/auth/
  ./internal/identity/ ./internal/store/sqlite/`; exit=0. The policy model, the
  split of validatePassword, the singleton table with migration 015, and the
  identity service surface that applies the policy before hashing and records a
  change with its audit event in one transaction.
- 2026-09-15 build red: REQ-FUNC-f9dbf9aa REQ-SEC-13b2e0d5 REQ-SEC-007c3be6;
  command=`go vet ./internal/httpapi/`; exit=1; expected missing
  behavior=auth.MinPasswordBytes no longer exists, and neither the routes nor
  the permission that guards them are defined.
- 2026-09-15 build green: REQ-FUNC-f9dbf9aa REQ-SEC-13b2e0d5 REQ-SEC-007c3be6
  REQ-SEC-0ecda0e7; command=`go test ./internal/httpapi/ ./internal/auth/`;
  exit=0. Two routes declaring password.policy.manage, readPassword reduced to
  the maximum it alone must refuse, and the policy failures mapped onto
  invalid_password and invalid_password_policy.
- 2026-09-15 build green: REQ-FUNC-03c21252; command=`go test ./cmd/...`;
  exit=0. `nilo admin password-policy` as a remote-mode client. The appliance's
  refusal message reached the operator only after internal/apiclient was taught
  to carry it beside the stable code: the requirement says the refusal travels
  unchanged, and the client was dropping everything but the code. That file was
  not in the design's expected list and is now on REQ-FUNC-03c21252.
- 2026-09-15 build green: REQ-UX-5e428717; command=`npm run test:ci` then
  `npm run build`; exit=0; 497 tests in 35 files. The panel, the client
  contract, the mock backend applying the same rule, and en/de/ro. The number
  inputs carry no min or max: the bounds are the appliance's, and a browser
  restating them both drifts from it and swallows the submission that earns the
  appliance's own message.
- 2026-09-15 build: whole Go suite `go test ./...` exit=0 after every
  increment. `evolution` reports 33 changed files, all expected: the design's
  list plus internal/apiclient/client.go as recorded above. Documentation
  updated in 07_Delivery/NILO_CLI_and_Authentication.md: the permission table,
  the password rule, the remote command reference, the route table, the
  invalid_password_policy code, and a section describing the policy itself.
  Stage advanced to validate.
- 2026-09-15 validate review round 1: two findings, both accepted and fixed.
  (1) Specification compliance: the ticket's acceptance that a character-class
  requirement holds "on the same two surfaces" had an explicit refusal on the
  HTTP routes but only an acceptance case on the command line;
  TestLocalUserCommandsAreHeldToTheStoredPasswordPolicy now asserts a long
  password of one class refused there too. (2) Implementation quality: the
  browser policy form took its enabled state from users.manage, so a credential
  holding password.policy.manage without it would have been shown a disabled
  form whose tooltip claimed it lacked the policy permission; UsersPage now
  reads the policy permission from the session for both the render and the
  form, with a test for that credential. Also hardened
  httpapi.policyMessageOf to strip exactly the two known sentinels rather than
  everything before the first colon, so a refusal that ever carries a wrapped
  cause cannot reach a client with its first clause cut off.
- 2026-09-15 validate: knowledge and context reconciled with the candidate -
  bench-core-foundation (public interface, the policy invariant, three file
  entries), gui-foundation (key behavior REQ-UX-5e428717, the panel's file
  entry, a Change map row) and context/project.md's identity responsibility.
  The spec was corrected to the built names: validatePasswordLength on both
  paths, getPasswordPolicy/setPasswordPolicy in api/client.ts, and the
  command line exiting 2 naming both options.
- 2026-09-15 validate review round 2: specification compliance approved and
  implementation quality approved on the current candidate.
- 2026-09-15 validate: the first full `check --task --strict --run` failed,
  exit 1, on two of its own gates and exposed a real candidate problem.
  `lint` failed on Prettier formatting in five frontend files. `test` failed in
  the appliance browser stage: e2e/field-display.spec.ts "1024x600: the shell
  and the Network page are readable and complete", inside
  exerciseAuthentication, 1 failed and 35 passed. The cause was the policy
  panel's hand-rolled label-and-input markup on a `.admin-form` class that does
  not exist, so its controls carried none of the field sizing the 1024x600
  field-target rule needs. The panel was rebuilt on the shared
  `components/help/Field` every other form uses, with `.admin-policy-form` and
  `.admin-policy-actions` added to shell.css and a help string per control in
  en, de and ro. Component tests 498 passed, build, lint and format clean.
- 2026-09-15 validate: the second `check --task --strict --run` also failed,
  exit 1, and the candidate was not the cause. Running
  `python scripts/aegis_test.py full` directly, where nothing is truncated,
  showed 118 script tests, the whole Go suite, three Python suites (32, 7+1
  skipped, 28), 498 component tests in 35 files, the production build and all
  46 browser tests green - and then the appliance stage failing in its own
  teardown: "The appliance verification stage changed this host network", with
  two of its Docker bridges still present afterwards. Its container was still
  running, so cleanup's `docker network rm` failed behind a `|| true` and the
  bridges survived. The AEGIS runner keeps a failing command's tail and the
  teardown message goes to stderr, which is why two full runs reported an
  unexplained `test exit 1`. Removed the leaked container and its two networks;
  the pre-existing, unrelated `nilo-network-test-676` was left alone.
- 2026-09-15 validate final proof:
  `python .agentic/aegis.py check --task TASK-ab42717a --strict --run`; exit 0;
  `test ok (375.8s)` and `lint ok (7.7s)`; `strict: ok`; 46 warnings, 2 notes,
  0 errors. Afterwards, traceability only and touching no executable file,
  db_test.go and commissioning_repository_test.go were linked to
  REQ-DATA-757ca87b and shell.css, App.test.tsx and shell.test.tsx to
  REQ-UX-5e428717, clearing five `uncovered-source` warnings;
  `check --task --strict` then reports 0 errors, 41 warnings, `strict: ok`.
  Remaining warnings on this task's files are three `secret-in-diff` hits on
  password literals in `internal/auth/password_policy_test.go`,
  `internal/auth/password_test.go` and `nilo_gui/src/api/mock.ts`. All three
  are test fixtures of the kind the suite already carries, not credentials;
  accepted. Every other warning belongs to another branch's open tasks or to
  pre-existing stale paths.
- 2026-09-16 deliver: delivery approved by the user, who asked for the tree to
  be staged and will make the commit themselves. The outcome is that one
  repository-local commit: no push and no PR are part of it, so nothing can
  fail after it and close-out is carried by the same commit. Per RULING-006 the
  agent staged only.
- 2026-09-16 deliver: the commit also carries two repository operations that
  are outside AEGIS scope and needed no journal of their own - the project
  version 0.6.0 -> 0.7.0 with its two mirrors and a CHANGELOG entry, and
  `.agentic/aegis.toml` completed to every table and key the tool reads, with
  `[layout] default_branch`, a populated `[check] strict_promote` and an
  explicit `[interaction]`. Both were verified inert or correct before staging:
  `go test ./internal/buildinfo/` ok, GUI build ok, `check --strict` ok.

## Rulings

- RULING-001 | date=2026-09-15 | decision=the absolute floor for the configured minimum password length is 8 bytes, while the shipped default minimum stays 12 | reason=8 is the NIST SP 800-63B minimum for a user-chosen secret and the user chose the wider configuration range; keeping the default at 12 means an appliance nobody configures behaves exactly as it does today, and an administrator who lowers the minimum below 12 does so deliberately | source=user | supersedes=none
- RULING-002 | date=2026-09-15 | decision=the character-class requirement is a minimum count of distinct classes present, from lowercase, uppercase, digit and symbol, defaulting to 0 | reason=the user chose a count over a named set; 0 by default keeps the shipped appliance free of a composition rule, which is what NIST SP 800-63B advises and what the ticket's consideration asks to decide deliberately | source=user | supersedes=none
- RULING-003 | date=2026-09-15 | decision=the browser Authentication page gets a password-policy editor in this task, with English, German and Romanian strings, mock backend behavior and browser-stage coverage | reason=the user widened scope beyond the ticket's two named surfaces; Browser GUI foundation becomes a second component in scope | source=user | supersedes=none
- RULING-004 | date=2026-09-15 | decision=scope as recorded in ## Frame is accepted and the task advances to specify | reason=user accepted the frame at the checkpoint boundary | source=user | supersedes=none
- RULING-005 | date=2026-09-15 | decision=this task runs in autopilot from design through deliver, with no stage-boundary prompts | reason=user asked to continue up to delivery without prompting; the journal still records every stage outcome | source=user | supersedes=none
- RULING-006 | date=2026-09-15 | decision=deliver prepares and presents the commit; it does not stage, commit or push | reason=user said not to commit, and the repository's Git authority rule in CLAUDE.md requires a current instruction for any Git write | source=user | supersedes=none

## Handoff


current: done
status: ready
next: none - the human makes the staged commit
must_read: .agentic/tasks/TASK-ab42717a.md
blockers: none

## Stage report

stage: deliver
result: completed
current: done
changed: 39 files staged - the candidate, its AEGIS records, the 0.7.0 version bump and aegis.toml
evidence: python .agentic/aegis.py check --task TASK-ab42717a --strict --run - exit 0, test ok 375.8s, lint ok 7.7s, strict: ok; go test ./internal/buildinfo/ ok and GUI build ok after the version bump
issues: none open; deployment to the Raspberry Pi was not performed and is not part of this delivery
next: none - the human commits the staged tree
needs_user: no
spec_compliance: approved
implementation_quality: approved
review_round: 2

## Delivery report (2026-09-16)

- stage: deliver; type: feature; path: full; branch: ddp/nilo-password-policy
- requirements:
  - REQ-SEC-492e15ed - Hold a password to the configured policy when it is set
  - REQ-SEC-1583a012 - Keep the password policy off the verification path
  - REQ-SEC-06c320c8 - Bound the work an unauthenticated password check can ask for
  - REQ-SEC-fe469bac - Refuse a password policy minimum below the absolute floor
  - REQ-SEC-13b2e0d5 - Restrict the password policy to administrators
  - REQ-SEC-007c3be6 - Record every password policy change in the audit history
  - REQ-DATA-757ca87b - Persist the appliance password policy
  - REQ-FUNC-f9dbf9aa - Read and replace the password policy over the authenticated HTTP interface
  - REQ-FUNC-03c21252 - Read and replace the password policy from the command line
  - REQ-UX-5e428717 - Administer the password policy from the Authentication page
  - REQ-SEC-0ecda0e7 - Protect administrator passwords
- deviations: none recorded
- explicit review: enabled; spec_compliance=approved; implementation_quality=approved; review_round=2
- diff vs task base d0c5c81: 59 file(s) changed, 7502 line(s) added
- checks: errors=0 warnings=45 notes=2
- checklist: 0/6 done; open: validated candidate confirmed unchanged; otherwise returned to validate; validation evidence, diff, exclusions, and outcome presented; delivery confirmed by the human; journal and report included in the delivery commit; agreed outcome delivered, normally a PR; close-out committed after the outcome succeeded, or carried by the delivery commit where nothing could fail after it
