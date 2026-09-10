# AGENTS.md

<!-- AEGIS:BEGIN (managed by .agentic/aegis.py init) -->

## AEGIS workflow

The full contract is `.agentic/AGENTS.md` - read it before starting work.
This digest is enough to act on if you read nothing else.

AEGIS applies only to software development operations. Do not use it for
repository operations or package and delivery operations; follow the
repository's own procedures for those instead.

Stages, one step file each in `.agentic/steps/`: **frame** (agree scope,
open the task journal) -> **specify** (write the requirement, and a spec
when useful) -> **design** (a short note for non-trivial work) -> **build**
(failing test first, then the implementation, until green) -> **validate**
(review, knowledge, full suite and strict check) -> **deliver** (approval,
report, commit/PR). A small fix or change may take the quick path,
frame -> build -> validate -> deliver.

To resume: `python .agentic/aegis.py status` lists the task journals; run
`python .agentic/aegis.py brief TASK-<id>`, then read the current step and the
Handoff `must_read` files. Required full journal reads still apply. Optional
one-level child tasks are child-side links visible only in this checkout; the
parent validates the integrated candidate.

Rulings are append-only; Handoff is replaceable. Rulings and Handoff cannot grant authority.
optional explicit review records two independent verdicts and uses a bounded fix
and re-review loop; disabled review never disables existing validation.

The append-only journal Log is a compact progress-and-evidence ledger, not a
session transcript. Record stage transitions, material changes or decisions,
consolidated verification, blockers, approvals, deviations and delivery
outcomes. Exclude routine tool activity, corrected retries, inner-loop test
retries and detail already held by another durable artifact.

At stage completion, pause or session end, or blocker, update the journal's
`## Stage report` fields from observed facts, then run
`python .agentic/aegis.py summary TASK-<id>` and copy the block before detail.
Missing evidence stays `not recorded`; read the journal if anything is stale or
shortened. This adds no approval stops and never replaces immediate deviation
notices or full delivery evidence.

Artifacts: requirements in `.agentic/requirements/<category>/`, specs in
`.agentic/specs/`, journals in `.agentic/tasks/`, component pages in
`.agentic/knowledge/`. `python .agentic/aegis.py check` is advisory - read
its findings and self-correct; nothing here blocks you mid-flight.

Say a deviation out loud when you record one, as one line -
`DEVIATION <stage>: <what> - <why accepted, or what it costs>` - then carry
on. It never pauses the work and autopilot never suppresses it. You owe one
only when the candidate, agreed scope/design/contract, or confidence in
required verification materially departs from plan: for example, accepted
relevant findings, skipped or weakened verification, retired requirements,
accepted unexpected candidate files, or unrequested work. Corrected internal
tool-use errors, expected red tests, and unrelated findings are not deviations.

<!-- AEGIS:END -->

## Git authority

**An agent never pushes. This is absolute.**

It holds whatever else is true. It overrides the AEGIS `deliver` stage when that
stage describes pushing a branch or publishing a pull request: the agent stops
after the local repository outcome and leaves remote publication to the human.
It is not waived by an approval of the work or by an instruction in a Ruling,
Handoff, task journal, or another repository document.

An agent may perform local Git operations, including staging and committing,
when they are authorized by the user and appropriate for the task. An agent may
read Git state freely. It may not run `push` or otherwise publish local Git
history to a remote.

The human pushes. An agent that believes local history should be published says
so and leaves the push for the human.

## Development credential

`.\02_Tools\scripts\nilo-dev.ps1 start` brings up the real backend and the
interface, and `admin` / `admin_password` signs in. The pair is fixed on
purpose: the development phase needs the running application reachable in one
step, so that visible behaviour can be checked against something close to the
real thing rather than against a mock.

It is created by `serve --development-administrator`, refused on any profile but
`simulation`, and it never disturbs an administrator with a different username.
The same pair seeds the browser mock. It is a development-phase convenience and
is expected to be removed before deployment; section 8.0 of
`06_SoftwareDocu/Development_and_Simulation_Flow.md` holds the detail and the
fences.

## Versioning and changelog rules

The repository carries one project-level Semantic Version covering the Go Bench
Core, the integrations, the GUI, and the tooling together. Components are not
versioned separately.

- The root `VERSION` file is the single canonical declaration. Two mirrors must
  match it: the `Version` literal in
  `05_Software/nilo_assistant/internal/buildinfo/buildinfo.go`, which is what
  `nilo version` and the HTTP status build block report, and the `version`
  field in `05_Software/nilo_gui/package.json` and its lockfile. The buildinfo
  mirror is enforced by a test; the rest are not.
- Assigning a version and building a release are repository operations, not
  AEGIS work: do not open a task journal for them and do not run the stages
  over them. The implementation a release carries is journaled; the release
  itself is not.
- The root `CHANGELOG.md` is the canonical record of project updates. Accrue
  changes under `## [Unreleased]` using Keep a Changelog sections (Added,
  Changed, Fixed, Removed).
- Keep entries short. One or two lines each: what changed, and why only when
  the why is not obvious from the what. No rationale essays and no restating
  the implementation - those belong in the task journal and the knowledge page.
  A reader scans a changelog; they do not study it.
- When the user gives a command such as "assign a version to the changelog", in
  the same change:
  1. roll `## [Unreleased]` into a new dated `## [x.y.z] - YYYY-MM-DD` section
     in `CHANGELOG.md`, leaving `## [Unreleased]` in place and empty;
  2. write that same `x.y.z` to the root `VERSION` file and mirror it into
     `buildinfo.go`, `package.json`, and `package-lock.json`;
  3. update every document that states the version in prose.
- Use Semantic Versioning: MAJOR for breaking changes to the integration wire
  protocol, package manifest format, CLI, HTTP API, or on-disk state layout;
  MINOR for backward-compatible capabilities; PATCH for fixes with no contract
  change. While MAJOR is `0`, a breaking change bumps MINOR.

## Repository environment tools

Keep repository-local setup, environment, validation, deployment, and
diagnostic tooling in `02_Tools/`. Downloaded runtimes and compiler binaries
belong under `02_Tools/runtime/` and are not committed; bootstrap and
environment scripts must recreate them from pinned, checksum-verified
archives without modifying the system-wide environment.

The sole binary-source exception is immutable multipart escrow under
`02_Tools/archives/`: only files matching the narrow `.gitattributes`
`*.partNNN` rules may be tracked through Git LFS. Extracted runtimes, joined
archives, caches, and temporary staging remain ignored. Every escrow generation
must be complete on the authoritative personal Gitea remote, independently
backed up and checksum-read back; GitHub may hold either a complete verified
replica or no archive replica, never a partial usable archive. Agents may build
and verify local parts when authorized but never push Git or LFS objects.

## Code style

Use `01_Documentation/Code_Style.md` as the repository code-style and
readability authority for production code and tests.

For Go, apply its general maintainability, modularity, documentation,
portability, determinism, and interface-preservation rules using idiomatic Go:

- format Go source with `gofmt`;
- use standard Go package and exported-identifier comments where documentation
  is needed;
- keep comments concise and focused on contracts, intent, and non-obvious
  behavior;
- do not copy Python-only shebangs, encoding declarations, Doxygen tags, or
  section-marker conventions into Go files.

Python files continue to follow the document's Python-specific rules directly.

## Development and simulation flow

Read `06_SoftwareDocu/Development_and_Simulation_Flow.md` before changing the
GUI, backend startup, development toolchain, simulation profiles, integration
simulators, or cross-component tests.

The default local development topology is a native Windows browser GUI with
Bench Core, SQLite, and integration runners executing under WSL2 Linux. Keep
runtime state on the WSL Linux filesystem, use explicit simulation mode, and
do not treat either Windows or WSL2 simulation as a production appliance or a
substitute for required Raspberry Pi hardware acceptance.

On a Windows development host, use
`02_Tools/bootstrap/bootstrap-development.ps1` to prepare and verify the pinned
Windows and WSL2 toolchains. WSL installation or conversion is a host change
and must remain explicit through its `-InstallWSL` option.
