<!--
Copyright (c) 2026 DDP Engineering
This software is licensed under the PolyForm Noncommercial License 1.0.0.
Commercial use requires a separate commercial license.
See the LICENSE.txt in the .agentic folder for full license details.
SPDX-License-Identifier: LicenseRef-PolyForm-Noncommercial-1.0.0
-->


# TASK-60bcc914 - Set the password policy by checkboxes in a collapsible panel

stage: done
type: change
path: full
interaction: autopilot
requirements: REQ-SEC-492e15ed, REQ-SEC-fe469bac, REQ-DATA-757ca87b, REQ-FUNC-f9dbf9aa, REQ-FUNC-03c21252, REQ-UX-5e428717, REQ-UX-c117cbf9
parent:
branch: ddp/nilo-password-policy
base: 0c5928691f5da9801d2edc496e051241897f448f
opened: 2026-09-16

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
The password policy panel asks an administrator for a number nobody can act on.
"Minimum character classes, from lowercase, uppercase, digit and other" expects
a count from 0 to 4 and never says which classes; a reader cannot tell what
setting 3 will require of a password. The panel also occupies the top of the
Authentication page permanently for a rule that is set once and then left, and
it carries a paragraph of explanation above controls that should explain
themselves.

Desired outcome:
An administrator sets the policy by ticking the classes a password must carry -
a lowercase letter, an uppercase letter, a digit, a special character - beside a
minimum length on the same line, inside a panel that is collapsed until they
want it.

Actors:
- Administrator: reads and sets the policy on the Authentication page.
- Every identity: has its password held to whatever classes are ticked.
- The `nilo` command line and the HTTP API: the same policy, same vocabulary.

Primary scenario: trigger -> system action -> observable result
An administrator opens Authentication -> the Users listing leads the page and
the password policy is one collapsed section -> they open it, see the minimum
length and four checkboxes with the current policy, tick "a digit", save -> the
appliance stores it and the next password set anywhere is refused unless it
carries a digit, naming that requirement.

Examples:
- Minimum 12 with "a digit" and "an uppercase letter" ticked: `passwordpassword`
  is refused naming both unmet requirements; `Passwordpassword1` is accepted.
- Nothing ticked: length alone governs, which is what the appliance ships with.

Counterexamples:
- The panel does not expand itself on arrival; the Users listing is what the
  page opens on.
- No checkbox is a count: ticking three boxes requires those three classes, not
  "any three of four".

In scope:
- The stored policy changes from one count to four named requirements, in the
  model, the database, the HTTP payload, the command line and the browser.
- The panel: minimum length and the class checkboxes on one line, inside a
  collapsible section that starts collapsed.
- Removing two texts: the Users page's "Signed in as ... permanent deletion
  removes the record itself and cannot be undone", and the policy panel's "The
  rule this appliance applies when a password is set ...".
- The 0.7.0 changelog entry, the delivery documentation, the spec, the affected
  requirements and both knowledge pages.

Out of scope:
- The absolute floor of 8 bytes, the fixed maximum of 1024, the permission, the
  audit event, and the rule that the policy governs setting a password and never
  checking one. All unchanged.
- Any change to who may read or set the policy.
- Redeploying the appliance. Separate, and the operator's call.

Constraints:
- OBSERVED `internal/store/sqlite/migrations.go:78` and the appliance itself:
  `015_password_policy` is applied on the bench appliance with digest
  `957dc81f...`. Migrations are digest-checked on open, so editing 015 would
  stop that appliance from starting. The reshape must be a new `016`.
- STATED: the project version stays 0.7.0 and its changelog entry is corrected
  rather than a new version being opened.
- OBSERVED project rule: superseded functionality is removed directly during
  active development; no compatibility alias for the retired count.
- OBSERVED `.agentic/knowledge/gui-foundation.md`: no operator-facing text below
  14px and every interactive target at least 44x44 at field sizes - a row of
  checkboxes beside a number field has to hold both at 1024x600.

Success signals:
- An administrator can tell, without documentation, what a password will be
  required to contain.
- The Authentication page opens on the identities, not on a rule.
- `check --strict --run` green, including the browser stage at field size.

Evidence:
- STATED (2026-09-16): checkboxes for special character, digit, lowercase and
  uppercase; minimum length on the same line; collapsible, collapsed by default
  because it is not used every day; remove the two named texts.
- OBSERVED `src/components/authentication/UsersPage.tsx:155-156`: the page-sub
  renders `auth.signedInAs` followed by `auth.identitiesNote`.
- OBSERVED `src/components/authentication/PasswordPolicyPanel.tsx:102`: the
  `auth.policyNote` paragraph.
- OBSERVED `internal/auth/password_policy.go`: `MinimumCharacterClasses int`
  with `countCharacterClasses`, and `PasswordPolicyMaxCharacterClasses = 4`.
- OBSERVED the deployed appliance: `password_policy` holds no row, so no
  configured count exists anywhere that a reshape would have to convert.
- INFERRED: four named booleans model the checkboxes exactly and remove the set
  semantics a list would need validating for order and duplicates.

Assumptions:
- "special character" keeps the classification total: any rune that is neither a
  letter nor a digit, which is what the retired "other" class already meant.

Unknowns: none.

Decision-blocking questions: all resolved by the user on 2026-09-16 and recorded
as RULING-001 to RULING-003 below.

Proposed behaviors:
- B1. The policy is a minimum length and four independent requirements:
  lowercase, uppercase, digit, special. Each is on or off.
- B2. A password being set is refused unless it satisfies every requirement that
  is on, and the refusal names the requirements it missed.
- B3. The Authentication page shows the policy as a collapsed section that opens
  on demand, with the minimum length and the four checkboxes on one line.
- B4. The Users page and the policy panel carry no explanatory paragraph.

Path and reason: full.

| criterion | yes | no | evidence |
|---|---|---|---|
| no new externally observable behavior | | no | the policy's meaning and its controls change |
| no new interface or data format | | no | a new payload shape and migration 016 |
| no new dependency | yes | | none |
| an existing requirement already covers the behavior | | no | REQ-SEC-492e15ed and REQ-UX-5e428717 describe the retired count |
| no security or data-handling behavior changes | | no | this is the credential rule itself |

## Requirement coverage

| Frame outcome | Atomic behavior | Requirement | Positive case | Boundary or failure | Verification | Design decision |
|---|---|---|---|---|---|---|
| B1 named requirements replace the count | the policy is a minimum length and four independent class requirements | REQ-DATA-757ca87b | a recorded policy survives a restart with its requirements | an appliance with no record applies minimum 12 requiring nothing | store and reopen test, plus a 015-to-016 upgrade test | migration 016 reshapes; 015 is never edited (RULING-004) |
| B2 a password is held to every required class | a password set anywhere is refused unless it meets the length and every required class | REQ-SEC-492e15ed | a password meeting length and all required classes is accepted | one byte short, and long enough but missing a class, each refused naming what is missing | auth, identity, httpapi and command-line tests | Check returns every unmet requirement, in a fixed order |
| B2 the floor still bounds the policy | a minimum outside 8..1024 is refused and changes nothing | REQ-SEC-fe469bac | a policy at the floor is accepted | 7 and 1025 refused naming the bound; stored policy unchanged | policy-model and HTTP refusal tests | the four booleans need no bounds of their own |
| B1 the wire contract | the route serves five values and stores a body carrying all five | REQ-FUNC-f9dbf9aa | a complete replacement is echoed and persists | a body omitting any value is refused without changing the stored policy | HTTP test over a real database | pointer fields tell an absent value from false |
| B1 the command line | the command prints the policy, or submits a length and a class list | REQ-FUNC-03c21252 | both values submitted and stored | exactly one value, or an unknown class name, exits 2 contacting nothing | command-line test against a recording stub | --require takes a comma list or the word none |
| B3 the panel | the policy is a disclosure that starts closed, with length and four checkboxes on one line | REQ-UX-5e428717 | opening it shows the current policy and a change is submitted | a refused change shows the appliance's message and restores the shown values | component test plus the three language catalogues | a closed <details>, as the Activity retention disclosure already does |
| B4 no standing prose | the identities page carries no paragraph describing the signed-in actor or the actions | REQ-UX-c117cbf9 | the listing, its actions and their help remain | the retired strings are absent from all three catalogues | component test asserting absence | the i18n keys are removed, not merely unused |


## Design

Six requirements are revised and one is new; the contract they are all judged
against is `.agentic/specs/REQ-FUNC-f9dbf9aa.md`, rewritten for this shape.

### The model

`auth.PasswordPolicy` becomes a minimum length and four booleans -
`RequireLowercase`, `RequireUppercase`, `RequireDigit`, `RequireSpecial`
(REQ-SEC-492e15ed). Four booleans rather than a list of class names: they map
one-to-one onto the checkboxes, and a list would need validating for unknown
names, duplicates and order in every layer that carries it. `Validate` keeps
only the length bounds, because a boolean has none. `Check` gathers every unmet
requirement and names them all in one refusal, in a fixed order, so an
administrator fixes a password once rather than discovering one rule per
attempt.

`countCharacterClasses` is replaced by a per-rune classifier that answers which
single class a rune belongs to. The classification stays total - special is
"neither a letter nor a digit" - so every password can be judged and invalid
UTF-8 still contributes special rather than erroring.

### Storage

Migration `016_password_policy_classes` creates the reshaped table, carries the
configured `minimum_length` across, drops the old one and renames
(REQ-DATA-757ca87b). **015 is not edited** (RULING-004): the bench appliance has
it applied with digest `957dc81f...` and `applyMigrationSet` compares digests on
every open, so editing it would stop that appliance starting. The retired count
is not converted - "three of four classes" names no particular three - so an
appliance that had configured one keeps its length and starts from no class
required, which is the shipped behaviour.

### Boundary and clients

The HTTP payload carries five values in both directions, each a pointer in the
request struct so an absent value is told from `false` (REQ-FUNC-f9dbf9aa). The
command line takes `--minimum-length` with `--require lower,upper,digit,special`
or `--require none`, refusing an unknown name before the appliance is contacted
(REQ-FUNC-03c21252).

### Browser

`PasswordPolicyPanel` becomes a `<details className="form-group">` with no
`open` attribute, which is how the Activity retention disclosure already
presents an administrator-only rule that is set rarely (REQ-UX-5e428717,
RULING-002). Inside, one `.admin-policy-line` lays the minimum length and a
`.admin-policy-classes` fieldset of four checkboxes on one wrapping row; the
fieldset carries a legend so the group has a name, and each checkbox is a
`Field`-labelled control so it keeps its 44px target at field sizes.

The page-sub paragraph leaves `UsersPage`, and the note leaves the panel
(REQ-UX-c117cbf9). Their i18n keys are removed from all three catalogues rather
than left unused, because the catalogue tests compare key sets across languages.

### Test seams

`internal/auth/password_policy_test.go`, `internal/store/sqlite/password_policy_repository_test.go`,
`internal/identity/password_policy_test.go`, `internal/httpapi/password_policy_test.go`,
`cmd/nilo/admin_password_policy_test.go`, `src/components/AuthenticationPage.test.tsx`.

### Alternatives

- **A list of class names on the wire.** Rejected: unknown names, duplicates and
  ordering to validate at every layer, for a fixed set of four.
- **Editing 015 in place.** Rejected by RULING-004; it would stop the deployed
  appliance from starting.
- **A button-and-state disclosure** rather than `<details>`. Rejected: the
  repository already has the `<details>` idiom for exactly this case, and it is
  keyboard- and screen-reader-complete without extra code.

### Risks

- The refusal message is assembled from several requirements; the HTTP boundary
  passes it through, so the tests assert the message names each unmet class.
- The browser stage checks 44x44 targets at 1024x600: a checkbox row is the
  first place in this interface where several small controls share a line.

No architecture drawing is owed: the composition is unchanged.


## Log

- 2026-09-16 frame: framed the password-policy redesign - named class
  requirements in place of a count, a collapsed collapsible panel, and two
  texts removed. Full path: the stored policy, the API payload, the command
  line and the browser all change shape. Branch ddp/nilo-password-policy
  continues, on top of 0c59286 which delivered the count model.
- 2026-09-16 specify: 6 requirements revised, 1 created (REQ-UX-c117cbf9), 0
  retired. The spec REQ-FUNC-f9dbf9aa.md was rewritten for the named-requirement
  model, its migration section now describing 016 and why 015 stays untouched.
- 2026-09-16 design: note in ## Design. Four booleans over a name list, a closed
  <details> following the Activity retention idiom, and one wrapping line for the
  length and the checkbox fieldset. User asked to run to delivery without stage
  prompts (RULING-005). Stage advanced to build.
- 2026-09-16 build red: REQ-SEC-492e15ed REQ-SEC-fe469bac REQ-DATA-757ca87b;
  command=`go test ./internal/auth/`; exit=1; expected missing
  behavior=PasswordPolicy has no Require* fields, so the named-requirement model
  does not compile.
- 2026-09-16 build green: REQ-SEC-492e15ed REQ-SEC-fe469bac REQ-DATA-757ca87b;
  command=`go test ./internal/auth/ ./internal/store/sqlite/`; exit=0. The four
  named requirements, a refusal naming every unmet one, migration 016 reshaping
  the table, and two guards: that an appliance on the 015 schema upgrades
  keeping its configured length, and that 015 itself is never edited.
- 2026-09-16 build green: REQ-FUNC-f9dbf9aa REQ-FUNC-03c21252; command=`go test
  ./...`; exit=0. Five pointer fields on the wire so an absent value is told
  from false, and `--require lower,upper,digit,special|none` on the command
  line, refusing an unknown class name before the appliance is contacted.
- 2026-09-16 build green: REQ-UX-5e428717 REQ-UX-c117cbf9; command=`npm run
  test:ci`, `npm run lint`, `npm run build`; exit=0; 501 tests in 35 files. The
  panel is a closed `<details>`, its length and four checkboxes on one wrapping
  line; the identities page carries no standing paragraph.
- 2026-09-16 build: DEVIATION build - three GUI assertions hardcoding
  `Version 0.6.0` were repaired. They are outside this task's frame scope: the
  0.7.0 bump delivered in 0c59286 broke them, and the miss was mine, because
  that change was proved with `npm run build` and never with `npm run test:ci`.
  They now read the version from package.json so the next bump cannot repeat
  it. Accepted because the configured suite cannot be green without it.
- 2026-09-16 build: `auth.signedInAs` was retired from the catalogues and then
  restored. The API tokens page still renders it above its own listing, and the
  user asked only for the identities page paragraph; REQ-UX-c117cbf9's
  verification was corrected to say so. One header test that asserted a
  `page-sub` follows the title group on both Authentication views is now
  view-aware, which is the contract REQ-UX-b75fbbc3 keeps.
- 2026-09-16 build: evolution reports 26 changed files, all expected. Stage
  advanced to validate.
- 2026-09-16 validate: the first `check --task --strict --run` failed, exit 1,
  in `test_build_environment`: "GUI package lock digest does not match the
  environment lock", five failures. Cause is the same miss as the version
  assertions: the 0.7.0 bump in 0c59286 rewrote nilo_gui/package-lock.json, and
  the digest of that file is recorded twice - `playwright-lock` in
  02_Tools/containers/build/build-environment.lock.json and
  PLAYWRIGHT_LOCK_SHA256 in 02_Tools/toolchain.versions. Both regenerated from
  45bd096d... to 3497765e...; `python -m unittest scripts.tests.
  test_build_environment` then ran 51 tests OK.
  DEVIATION validate: that repair is outside this task's agreed scope. It is
  accepted because the configured suite cannot be green without it and the
  cause is a change this branch already delivered.
- 2026-09-16 validate: the digest is recorded in a third place -
  scripts/tests/test_toolchain_bootstrap.py pins it as a literal beside the
  remote-artefact digests. Updated to match. Unlike a Go or Node tarball, this
  one is the digest of a repository file that changes with any dependency or
  version change, so a literal there is a copy somebody must hand-sync; whether
  it should be derived from the file instead is a supply-chain pinning decision
  for the repository owner, recorded here rather than taken. 118 script tests
  then ran OK.
- 2026-09-16 validate review round 1: two findings, both fixed.
  (1) `auth.help.policyMinimumClasses` survived the reshape unused and still
  named the retired count; it is now `auth.help.policyClasses` and is rendered
  by a HelpPopover in the class fieldset's legend, so the group carries the
  guidance a single Field would. (2) `auth.signedInAs` had been retired from the
  catalogues although the API tokens page still renders it; restored, with
  REQ-UX-c117cbf9's verification corrected to say the signed-in sentence remains
  there.
- 2026-09-16 validate: knowledge and context reconciled - bench-core-foundation
  (the policy invariant and the 015/016 file entries), gui-foundation
  (REQ-UX-5e428717 restated, REQ-UX-c117cbf9 added, the panel's file entry and
  its Change map row), CHANGELOG 0.7.0 and the delivery documentation.
- 2026-09-16 validate final proof:
  `python .agentic/aegis.py check --task TASK-60bcc914 --strict --run`; exit 0;
  `test ok (372.3s)` and `lint ok (8.6s)`; `strict: ok`; 0 errors, 38 warnings,
  2 notes. That run covers 118 script tests, go test ./..., three Python suites,
  501 GUI component tests, the production build and the appliance browser stage.
  Remaining warnings belong to other branches' open tasks and pre-existing stale
  paths; the three `secret-in-diff` hits on password literals in test fixtures
  and the GUI mock remain accepted, as in TASK-ab42717a.
- 2026-09-16 validate review round 2: specification compliance approved and
  implementation quality approved on the current candidate.
- 2026-09-16 deliver: redeployed to the bench appliance ddp-remote at the user's
  request, before approval, so the change could be tried on real hardware. New
  one-use script 02_Tools/runtime/deploy-0.7.0-classes-pi.py, which pins both
  the new binary digest a6bc99ea... and the previous ba56eb50... so a stale
  staging directory cannot report a no-op as deployed, and which asserts
  016_password_policy_classes in the appliance's own ledger rather than
  inferring the reshape from a service that came back up. Verified on the
  appliance: migrations 016 over 015, the table carrying minimum_length and the
  four require_ columns with the count column gone, no policy row so the shipped
  default still applies, the service active on 0.7.0, and the interface serving
  index-DFJqRZYb.js with the four class labels and neither retired string.
  Backup at /var/backups/nilo/20260916T084741Z.
- 2026-09-16 deliver: the user tried the panel on the appliance and accepted it.
  Delivery approved; they asked for the tree to be staged and will make the
  commit themselves. The outcome is that one repository-local commit, so nothing
  can fail after it and close-out is carried by the same commit. Per RULING-006
  the agent staged only.
- 2026-09-16 deliver: the commit also carries two repairs of the 0.7.0 version
  bump delivered in 0c59286, both recorded above as deviations - the GUI version
  assertions, now derived from package.json, and the playwright-lock digest in
  02_Tools/containers/build/build-environment.lock.json,
  02_Tools/toolchain.versions and scripts/tests/test_toolchain_bootstrap.py.
  Whether that third copy should be derived from the lockfile rather than pinned
  as a test literal is left with the repository owner.

## Rulings

- RULING-001 | date=2026-09-16 | decision=the character-class rule becomes four independent named requirements set by checkbox - lowercase, uppercase, digit, special - replacing the minimum distinct-class count, which retires the choice TASK-ab42717a recorded as its RULING-002 | reason=user reported the count is not actionable: a reader cannot tell what setting 3 requires of a password | source=user | supersedes=none
- RULING-002 | date=2026-09-16 | decision=the panel is collapsible and starts collapsed | reason=the user's stated reason - the policy is not used every day - was asked back against their literal wording and they chose collapsed | source=user | supersedes=none
- RULING-003 | date=2026-09-16 | decision=the project version stays 0.7.0 and its changelog entry is corrected to describe the shipped shape | reason=user chose to fold this into 0.7.0; the version is committed but not released or tagged | source=user | supersedes=none
- RULING-004 | date=2026-09-16 | decision=the reshape is a new migration 016; 015 is never edited | reason=the bench appliance has 015 applied with a recorded digest and migrations are digest-checked on open, so editing 015 would stop that appliance starting | source=technical | supersedes=none
- RULING-005 | date=2026-09-16 | decision=this task runs in autopilot from specify through deliver, with no stage-boundary prompts | reason=user asked to continue to specify and run to delivery without prompting | source=user | supersedes=none
- RULING-006 | date=2026-09-16 | decision=deliver prepares and presents; it does not commit or push | reason=the repository Git authority rule requires a current instruction for any Git write, and the user has given none for this task | source=repository | supersedes=none

## Handoff


current: done
status: ready
next: none - the human makes the staged commit
must_read: .agentic/tasks/TASK-60bcc914.md
blockers: none

## Stage report

stage: deliver
result: completed
current: done
changed: 43 files staged - the candidate, its AEGIS records and the two repairs of the 0.7.0 bump
evidence: python .agentic/aegis.py check --task TASK-60bcc914 --strict --run - exit 0, test ok 372.3s, lint ok 8.6s, strict: ok; plus the bench appliance ddp-remote running the candidate with migration 016 applied and the panel accepted by the user
issues: none open; one question left with the repository owner about deriving the playwright-lock digest rather than pinning it in a test
next: none - the human commits the staged tree
needs_user: no
spec_compliance: approved
implementation_quality: approved
review_round: 2

## Delivery report (2026-09-16)

- stage: deliver; type: change; path: full; branch: ddp/nilo-password-policy
- requirements:
  - REQ-SEC-492e15ed - Hold a password to the configured policy when it is set
  - REQ-SEC-fe469bac - Refuse a password policy minimum below the absolute floor
  - REQ-DATA-757ca87b - Persist the appliance password policy
  - REQ-FUNC-f9dbf9aa - Read and replace the password policy over the authenticated HTTP interface
  - REQ-FUNC-03c21252 - Read and replace the password policy from the command line
  - REQ-UX-5e428717 - Administer the password policy from the Authentication page
  - REQ-UX-c117cbf9 - Leave the Authentication pages without a standing explanatory paragraph
- deviations: 2 recorded
  - build - three GUI assertions hardcoding `Version 0.6.0` were repaired. They are outside this task's frame scope: the 0.7.0 bump delivered in 0c59286 broke them, and the miss was mine, because that change was proved with `npm run build` and never with `npm run test:ci`. They now read the version from package.json so the next bump cannot repeat it. Accepted because the configured suite cannot be green without it.
  - validate: that repair is outside this task's agreed scope. It is accepted because the configured suite cannot be green without it and the cause is a change this branch already delivered.
- explicit review: enabled; spec_compliance=approved; implementation_quality=approved; review_round=2
- diff vs task base 0c59286: 43 file(s) changed, 5476 line(s) added
- checks: errors=0 warnings=38 notes=2
- checklist: 0/6 done; open: validated candidate confirmed unchanged; otherwise returned to validate; validation evidence, diff, exclusions, and outcome presented; delivery confirmed by the human; journal and report included in the delivery commit; agreed outcome delivered, normally a PR; close-out committed after the outcome succeeded, or carried by the delivery commit where nothing could fail after it
