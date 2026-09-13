<!--
Copyright (c) 2026 DDP Engineering
This software is licensed under the PolyForm Noncommercial License 1.0.0.
Commercial use requires a separate commercial license.
See the LICENSE.txt in the .agentic folder for full license details.
SPDX-License-Identifier: LicenseRef-PolyForm-Noncommercial-1.0.0
-->


# TASK-ac1e9003 - Record the appliance commissioning lifecycle state

stage: done
type: feature
path: full
interaction: autopilot
requirements: REQ-DATA-72876d18, REQ-REL-d9bfd8ae
parent:
branch: ddp/nilo-first-boot-provisioning
base: 5ef872e1bbab6df6264b5f1cb1ec007f96c4eb77
opened: 2026-09-12

## Done checklist (current stage)

- [x] validated candidate confirmed unchanged; otherwise returned to validate
- [x] validation evidence, diff, exclusions, and outcome presented
- [x] delivery confirmed by the human
- [x] journal and report included in the delivery commit
- [x] agreed outcome delivered, normally a PR
- [x] close-out committed after the outcome succeeded, or carried by the
      delivery commit where nothing could fail after it

## Frame

Problem: NILO has no notion of an appliance that has not been commissioned yet.
`Identity.CommissionedAt` is only the timestamp at which the identity row was
first written, so nothing distinguishes a clean installation still being set up
from an appliance an administrator has finished configuring. Every first-boot
behavior in issue #6 is conditional on that distinction, and an upgrade that
guessed it wrongly would either re-open first-boot exposure on a deployed
appliance or leave a new one with no way in.

Desired outcome: one durable appliance-level lifecycle state, `provisioning` or
`commissioned`, that survives restart and reboot, that a clean installation
enters and an upgraded installation never enters, and that later work packages
read to decide first-boot behavior.

Actors: the SQLite migration set (decides the initial state from what the
database already holds); Bench Core startup (reads and exposes the state); later
first-boot provisioning, commissioning-transition and GUI behaviors (the only
callers that act on it).

Primary scenario: the appliance database is opened -> the migration set runs
before any identity row is written, and records `provisioning` for a database
that holds no prior appliance state and `commissioned` for one that does ->
Bench Core reads that state on every start and reports it unchanged until a
later work package deliberately completes commissioning.

Examples: a factory image whose database is created by the first
`nilo admin user create` at the console records `provisioning`, because
migrations run before the identity row exists. An appliance upgraded from a
build without this migration records `commissioned`, because its identity row is
already there.

Counterexamples: an upgrade of a deployed appliance does not enter
`provisioning`, whether it runs interface-managed management access or the
legacy single `--bind` listener; restarting a provisioning appliance does not
advance it to `commissioned`.

In scope: migration `013_commissioning.sql` with its singleton table and the
seeding rule; a repository for it beside the existing ones; a small service that
loads the state and exposes it; persistence proven across reopen; focused Go
tests including a migration-set test that builds a database at the earlier
schema and upgrades it in place.

Out of scope: changing what `serve` accepts on the command line; deriving,
applying or freezing any management allowlist; any firewall, listener or mDNS
behavior; the transition that completes commissioning; any HTTP route, API field
or GUI change. This task records the state and reads it back; nothing consumes
it yet.

Constraints: migrations are digest-checked and append-only in
`internal/store/sqlite/migrations.go`, so `013_commissioning.sql` is added, never
edited afterwards (OBSERVED: `migrations.go` refuses a changed recorded digest).
The seeding decision must be expressible in the migration SQL itself, because
that is the only code that runs before the identity row is written. Tables in
this schema are `STRICT` with explicit `CHECK` constraints, and the file follows
that convention.

Success signals: a test opens a brand-new database and observes `provisioning`;
a test builds a database at migration 012, writes an identity row, then applies
013 and observes `commissioned`; a test reopens a provisioning database and
observes it unchanged; the focused package tests and the Go suite stay green.

Evidence:
- OBSERVED `internal/store/sqlite/db.go:46` - `applyMigrations` runs inside
  `sqlite.Open`, before the caller does anything else with the database.
- OBSERVED `internal/app/app.go:79-114` - `app.Open` calls `sqlite.Open` first
  and `appliance.Provision`, which writes the `appliance_identity` row,
  afterwards. The migration set therefore always runs before the first identity
  row exists.
- OBSERVED `internal/store/sqlite/migrations/006_appliance_audit_operations.sql:11`
  - `appliance_identity` is a singleton table keyed `CHECK (id = 1)`.
- OBSERVED `internal/store/sqlite/migrations.go` - the ledger records a digest
  per migration and refuses a mismatch, so an applied migration is immutable.
- OBSERVED `internal/store/sqlite/migrations.go` - `applyMigrationSet` exists
  precisely so a test can build a database at an earlier schema and prove an
  in-place upgrade.
- OBSERVED `cmd/nilo/main.go:174` - `serve` refuses to start without `--bind` or
  interface-managed options. Relaxing that is deliberately left to the work
  package that can also serve.
- OBSERVED `internal/appliance/identity.go:33-35` - `CommissionedAt` is the
  identity creation time and carries no lifecycle meaning.
- STATED (user, issue #6) - "Enter first-boot provisioning only for a clean
  installation that has not completed commissioning", "Persist provisioning
  state and selected roles across service restart and reboot", "Do not place
  existing installations into first-boot provisioning during upgrade",
  "Preserve legacy single-bind behavior until interface-based management is
  deliberately configured"; acceptance tests 12 and 13.
- INFERRED - the presence of an `appliance_identity` row at migration time is
  the one fact that separates a fresh database from an upgraded one, because the
  row is written on the first open of any data directory and never removed.

Assumptions: no supported path writes an `appliance_identity` row before the
migration set runs, and none removes it afterwards. Both follow from
`sqlite.Open` owning migrations and `appliance.Provision` being the only writer.

Unknowns: whether a future factory process might create the database by some
route other than opening it through `app.Open`. Not a question this task can
settle; the seeding rule reads the database rather than trusting a process, so
such a route would still be classified by what the database actually holds.

Decision-blocking questions: none. One scope refinement is proposed below and is
the only thing needing agreement.

Proposed behaviors:
1. A newly created appliance database records the commissioning lifecycle state
   `provisioning`; a database that already holds appliance state when the
   migration is applied records `commissioned`.
2. Bench Core reads the recorded lifecycle state when it opens the appliance and
   reports it unchanged across restart.

Path and reason: full.

| criterion | yes | no | evidence |
|---|---|---|---|
| no new externally observable behavior | | no | a durable lifecycle state is new appliance state, and it decides later first-boot exposure |
| no new interface or data format | | no | a new schema migration and singleton table |
| no new dependency | yes | | SQLite and the existing repository pattern only |
| an existing requirement already covers the behavior | | no | nothing in `.agentic/requirements/` describes a commissioning lifecycle |
| no security- or data-handling behavior changes | | no | the state gates first-boot management exposure in later work |

Proposed scope refinement against the agreed plan: the plan's WP2 also relaxed
the `serve` command-line gate at `cmd/nilo/main.go:174`. That is moved to the
work package that derives the provisioning allowlist, because the gate can only
be relaxed usefully together with something that can actually serve: the state
is read after `app.New` opens the database, while the gate is evaluated before
it, and relaxing the gate alone would let `serve` accept a configuration it
cannot yet honor. Keeping them together means no intermediate commit leaves
`serve` able to start without a listener.

## Requirement coverage

| Frame outcome | Atomic behavior | Requirement | Positive case | Boundary or failure | Verification | Design decision or no impact |
|---|---|---|---|---|---|---|
| A clean installation enters provisioning and an upgraded one never does | The migration records the initial lifecycle state from whether the database already holds an appliance identity | REQ-DATA-72876d18 | Fresh database records provisioning; a database holding an identity row records commissioned | A value outside the two permitted states is rejected, and a second row is rejected | `commissioning_repository_test.go` over a fresh database and over a database built at the previous migration with an identity row written into it | Seeding expressed in the migration SQL itself, because it is the only code that runs before the identity row exists |
| The state survives restart and reboot | The appliance reports the recorded state on every open and refuses to assume one | REQ-REL-d9bfd8ae | Open, close and reopen a data directory and read the same state, for both permitted values | An absent or unrecognized recorded value is refused with an error rather than defaulted | `commissioning_test.go` open/reopen cycles plus directly written absent and unrecognized values | A read-only service over the repository; no writer in this task, so the state cannot drift |


## Design

**Requirements coverage.** REQ-DATA-72876d18 is carried entirely by migration
`013_commissioning.sql`, whose seeded value is decided in SQL. REQ-REL-d9bfd8ae
is carried by a repository read, a validated domain type, and the load that
`app.Open` performs on every open.

**Component responsibilities.** Both land in *Bench Core foundation*. The
migration set owns the initial value, because it is the only code that runs
before an appliance identity exists. The `appliance` package owns the state's
vocabulary and its validity rule and reads nothing itself. The SQLite repository
owns retrieval, beside the existing repositories. `app.Open` owns loading the
state once per open and refusing to continue when the database cannot supply a
valid one. Nothing writes the state in this task.

**Data ownership and state transitions.** One singleton row:

    CREATE TABLE commissioning (
        singleton INTEGER PRIMARY KEY CHECK (singleton = 1),
        state TEXT NOT NULL CHECK (state IN ('provisioning', 'commissioned'))
    ) STRICT;

`STRICT` with explicit `CHECK` constraints matches every table this schema
already defines, and the two checks are what make the boundary cases in
REQ-DATA-72876d18 observable at the database rather than only in Go. The row is
seeded in the same migration:

    INSERT INTO commissioning (singleton, state)
    SELECT 1, CASE WHEN EXISTS (SELECT 1 FROM appliance_identity)
                   THEN 'commissioned' ELSE 'provisioning' END;

No timestamp column. Neither requirement asks when the state was recorded, and
`appliance_identity.commissioned_at` already exists with a different meaning, so
a second, similarly named time would invite exactly the confusion this task is
correcting. A later work package that needs a completion time adds its own
migration; the set is append-only either way (REQ-DATA-72876d18).

**Interfaces and contracts.**

- `appliance.CommissioningState` - a string type with `CommissioningProvisioning`
  and `CommissioningCommissioned`, and `Validate() error` refusing anything else,
  including the empty value. There is no zero-value default: an unset state is an
  error, never silently one of the two (REQ-REL-d9bfd8ae).
- `appliance.CommissioningStore` - `LoadCommissioning(context.Context) (CommissioningState, error)`,
  the narrow interface the domain owns and SQLite satisfies, matching how
  `network.ManagementAccessStore` is declared by its domain package.
- `sqlite.CommissioningRepository` with `NewCommissioningRepository(*DB)` -
  reads the singleton row, returns a named error when the row is missing and
  wraps a decode failure, and asserts `var _ appliance.CommissioningStore` the
  way `ManagementAccessRepository` does.
- `App.Commissioning() appliance.CommissioningState` - the value loaded during
  `app.Open`.

**Execution flow.** `app.Open` already calls `sqlite.Open`, which applies the
migration set, and then `appliance.Provision`. The commissioning load is placed
immediately after `appliance.Provision`, so the ordering that the seeding rule
depends on stays visible in one place and the failure closes the database the
same way the surrounding steps do.

**Failure recovery.** A missing row, an unrecognized value, or a read error
fails `app.Open`. Refusing to start is the safe direction: a guessed
`commissioned` would silently skip first-boot on an appliance that needs it, and
a guessed `provisioning` would widen exposure on one that does not
(REQ-REL-d9bfd8ae).

**Security and trust boundaries.** The state carries no secret and grants no
capability in this task. It is the input a later work package uses to decide
management exposure, which is why an invalid value must fail rather than default.

**Compatibility and migration.** The migration is additive and the ledger is
digest-checked, so `013_commissioning.sql` is never edited after it ships. An
upgraded appliance takes `commissioned` from the identity row it already holds,
which covers both the interface-managed and the legacy single `--bind`
deployment without either having to be detected separately. No API, CLI or GUI
surface changes, so no client contract moves.

**Deliberately not done here.** The `serve` gate at `cmd/nilo/main.go:174` is
untouched, per the accepted scope refinement: it is evaluated before `app.New`
opens the database, so it cannot read this state, and relaxing it without the
allowlist that makes provisioning serve would admit a configuration nothing can
honor.

**Test seams.**

- `internal/store/sqlite/commissioning_repository_test.go` - a brand-new database
  reads back `provisioning`; a database built with `applyMigrationSet` up to
  `012_management_access`, given an `appliance_identity` row, then carried to the
  full set, reads back `commissioned` with that row and the earlier schema
  intact; direct writes prove the two `CHECK` constraints refuse an unknown state
  and a second row.
- `internal/appliance/commissioning_test.go` - `Validate` accepts both states and
  refuses the empty and unknown values; the load helper refuses an absent row and
  an unrecognized recorded value rather than defaulting.
- `internal/app/app_test.go` - open a data directory, read the state, close,
  reopen and read the same value.

**Alternatives rejected.** Deciding the seed in Go after the migration - it
would have to run before `appliance.Provision` to see an empty identity table,
which inverts the ordering `app.Open` establishes for good reasons. Treating an
absent row as `provisioning` - that is the guess that widens exposure on a
database whose row failed to seed. Reusing `Identity.CommissionedAt` - it is the
identity creation time on every appliance, including one mid-commissioning, so
it cannot express this state. Adding a writer now - nothing in scope completes
commissioning, and an unused writer would ship untested.

**Expected files.** `internal/store/sqlite/migrations/013_commissioning.sql`
(new), `internal/store/sqlite/migrations.go`,
`internal/store/sqlite/commissioning_repository.go` (new),
`internal/store/sqlite/commissioning_repository_test.go` (new),
`internal/appliance/commissioning.go` (new),
`internal/appliance/commissioning_test.go` (new), `internal/app/app.go`,
`internal/app/app_test.go`.

**Architecture.** Behavior-only: one table, one repository beside the existing
ones and one load step in an existing startup sequence. No component boundary
moves, so no architecture drawing is owed.

**Risks.** The seeding rule depends on `app.Open` keeping migrations ahead of
identity provisioning; the repository-level test pins that by building the
earlier schema explicitly rather than trusting the ordering.

**Open decisions.** None.

## Log

- 2026-09-12 frame: scope drafted for the durable commissioning lifecycle
  state that first-boot provisioning (issue #6, work package 2 of the agreed
  plan) depends on. Full path chosen: four of the five quick-path criteria
  fail. The seeding discriminator was confirmed against the checkout before
  drafting - `sqlite.Open` applies migrations and `app.Open` provisions the
  identity afterwards, so the migration always runs before the first identity
  row exists. One scope refinement proposed: the `serve` command-line gate
  moves to the allowlist work package. Awaiting scope acceptance.
- 2026-09-12 frame: scope and the proposed refinement accepted by the user,
  who also instructed this task to run specify through validate without stage
  prompts. Journal interaction set to autopilot; validate remains the agreed
  stopping point.
- 2026-09-12 specify: 2 requirements created, 0 revised, 0 reused, 0 retired.
  Split along the two independently verifiable outcomes: what the initial
  state is, and that the recorded state is reported unchanged afterwards.
- 2026-09-12 design: note recorded in `## Design`; no spec is owed. The table
  carries no timestamp column and no writer exists in this task; an absent or
  unrecognized recorded state fails `app.Open` rather than defaulting.

- 2026-09-12 build red: REQ-DATA-72876d18, REQ-REL-d9bfd8ae;
  command=`go test ./internal/appliance/ ./internal/store/sqlite/ ./internal/app/ -run 'Commissioning|CommissioningState|ProvisioningCommissioning|UpgradedDatabase' -count=1`
  (run from `05_Software/nilo_assistant`); exit=1; expected missing
  behavior=`appliance.CommissioningProvisioning`,
  `appliance.CommissioningCommissioned`, `appliance.LoadCommissioning`,
  `sqlite.NewCommissioningRepository` and `App.Commissioning` do not exist, and
  no `commissioning` table is created, so none of the three packages compiles.
  One record covers both requirements: the same run demonstrates both gaps.

- 2026-09-12 build green: REQ-DATA-72876d18, REQ-REL-d9bfd8ae;
  command=`go test ./internal/appliance/ ./internal/store/sqlite/ ./internal/app/ -run 'Commissioning|CommissioningState|ProvisioningCommissioning|UpgradedDatabase' -count=1`;
  exit=0.
- 2026-09-12 build: whole Go suite run early rather than at validate, because a
  new migration changes the schema every database test opens.
  `go test ./... -count=1` first failed one assertion:
  `internal/store/sqlite/db_test.go::TestOpenConfiguresDurableDatabaseAndMigratesAllFoundationTables`
  lists every non-internal table by name and did not yet include
  `commissioning`. That inventory is meant to be updated when a table is added,
  so `commissioning` was added to its expected list; the assertion was not
  relaxed and still fails on any unlisted table. Rerun exit=0 over every package
  in `05_Software/nilo_assistant`, with `go vet ./...` exit=0.
- 2026-09-12 build: evolution reports six files - the five the design listed plus
  `internal/store/sqlite/db_test.go` for that inventory update. No unexpected
  candidate file, and no test was weakened.

- 2026-09-12 validate: review round 1. Specification compliance approved - the
  candidate is the five files the design named plus the schema inventory test,
  no writer was added, the `serve` gate is untouched per the accepted scope
  refinement, and the table carries no timestamp column. Implementation quality
  requires changes, one finding: `App.Commissioning` was inserted between the
  `AuditWriter` doc comment and its function, leaving that comment attached to
  the wrong declaration and both accessors mis-documented. gofmt does not catch
  it because the result is valid Go. Routing to build for the fix.
- 2026-09-12 build: review round 1 finding fixed. `App.Commissioning` and its
  comment now sit before the `AuditWriter` comment, which is attached to
  `AuditWriter` again. command=`go test ./internal/app/ ./internal/appliance/ ./internal/store/sqlite/ -count=1`;
  exit=0; `gofmt -l internal/` reported nothing.
- 2026-09-12 validate: review round 2 on the fixed candidate. Specification
  compliance approved and implementation quality approved: the round-1 finding
  is closed and no other finding stands.
- 2026-09-12 validate: knowledge reconciled. `.agentic/knowledge/bench-core-foundation.md`
  gains the lifecycle invariant, the two new file seams, the two requirement IDs
  and a current-state section. `.agentic/context/project.md` needs no change:
  this task adds no product responsibility, no interface and no observable
  appliance behavior beyond refusing to start on an unusable recorded value.

- 2026-09-12 validate: final gate run.
  command=`.\02_Tools\env\with-tools.ps1 python .agentic/aegis.py check --task TASK-ac1e9003 --strict --run`;
  exit=1; errors=2 (`run: test` exit 1 after 81.8s, `run: lint` exit 1 after
  3.5s). Both are the pre-existing base-branch failures accepted for this branch
  under TASK-2873801a RULING-003, unchanged and with nothing new joining them:
  `test_build_environment.py::test_reconstruction_lock_is_complete_and_consumed_offline`
  (92 script tests ran, the same 1 failed) and `prettier --check` on the same
  four GUI files. The script-test failure short-circuits the configured runner
  before the Go, Python and GUI suites execute, so those were run individually.
- 2026-09-12 validate: verification completed on the final candidate.
  `go test ./... -count=1` exit=0 over every package in
  `05_Software/nilo_assistant`, and `go vet ./...` exit=0. Python:
  integration_sdk 32 passed, sigrok 28 passed, fake_python 7 passed 1 skipped,
  all exit=0. GUI: `npm run test:ci` 34 files / 459 tests passed exit=0;
  `npm run build` exit=0. Browser end-to-end through the tool wrapper: 34 passed,
  1 failed, 4 did not run - the failure being the same pre-existing
  `e2e/network.spec.ts:115` accepted for this branch, which also blocks the 4
  that did not run.
- 2026-09-12 validate: the end-to-end run is the material evidence for this
  candidate's integration risk. It starts a real Bench Core on a fresh data
  directory, so its 34 passing tests prove that migration 013 applies to a new
  database, seeds the row, and that `app.Open` reads a valid state and continues
  - the paths a schema change is most likely to break.
- 2026-09-12 validate: DEVIATION validate: the configured gate exits non-zero for
  the base-branch failures already accepted for this branch, so this candidate
  also advances on per-suite evidence rather than one green gate command. No new
  failure was introduced; every suite the candidate can affect was run and
  passed.

- 2026-09-13 deliver: delivered. The human committed the whole issue #6
  candidate as `47f4ad6` "Feature: First-boot management provisioning on
  eligible interfaces" on `ddp/nilo-first-boot-provisioning`, together with the
  other work packages and the 0.3.0 version bump. This task contributed the commissioning lifecycle state and its seeding rule.
  A per-package split was no longer clean by then - five files had been edited by
  more than one package, so separate commits would not have built individually -
  and the human chose one commit over hunk-level staging. Per-package
  traceability remains in these journals and their requirements. Repository Git
  authority reserves committing to the human; the agent neither staged,
  committed nor pushed. No push, PR or release was part of this outcome.
- 2026-09-13 deliver: hardware evidence recorded for the feature as a whole on
  the Raspberry Pi `ddp-remote` before the commit. A fresh data directory started
  with no listener option, derived `eth0`/`eth1`/`wlan0`, excluded the WireGuard
  device, recorded `provisioning`, bound to the documented stable adapter
  identities, published `ddp-remote.local` through avahi, and resumed the same
  allowlist after a restart. Upgrading the real installation recorded
  `commissioned` and left its identities and `{nilo0, wlan0}` allowlist
  untouched. The board now runs 0.3.0. Detail is in
  `06_SoftwareDocu/Local_Hardware_Access.md` section 2.8; this is early bench
  evidence from a personal board, not a reproducible acceptance gate.
- 2026-09-13 done: task closed. This close-out line is journal-only and rides
  with the next commit the human makes.

## Rulings


## Handoff


current: done
status: ready
next: none - issue #6 is delivered in commit 47f4ad6 and verified on ddp-remote
must_read: .agentic/tasks/TASK-ac1e9003.md
blockers: none

## Stage report

stage: deliver
result: completed
current: done
changed: delivered in commit 47f4ad6 together with the rest of issue #6 and the 0.3.0 version bump; this task contributed the commissioning lifecycle state and its seeding rule
evidence: automated suites green before delivery, recorded in this journal's validate entries; hardware observations on ddp-remote recorded in the deliver entry above and in 06_SoftwareDocu/Local_Hardware_Access.md section 2.8
issues: the base-branch failures accepted under TASK-2873801a RULING-003 remain open on the branch and still fail CI, as they do on master
next: none
needs_user: no
spec_compliance: approved
implementation_quality: approved
review_round: 1

