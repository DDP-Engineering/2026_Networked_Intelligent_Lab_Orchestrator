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

At stage completion or pause, update the journal's `## Stage report` fields
from observed facts, then run `python .agentic/aegis.py summary TASK-<id>` and
copy the block before detail. Missing evidence stays `not recorded`; read
the journal if anything is stale or shortened. This adds no approval stops
and never replaces immediate deviation notices or full delivery evidence.

Artifacts: requirements in `.agentic/requirements/<category>/`, specs in
`.agentic/specs/`, journals in `.agentic/tasks/`, component pages in
`.agentic/knowledge/`. `python .agentic/aegis.py check` is advisory - read
its findings and self-correct; nothing here blocks you mid-flight.

Say a deviation out loud when you record one, as one line -
`DEVIATION <stage>: <what> - <why accepted, or what it costs>` - then carry
on. It never pauses the work and autopilot never suppresses it. You owe one
for an accepted `check` warning, a non-zero exit or a verification you
skipped, an unexpected `anchor`/`evolution` report, a manual method standing
in for an automatable test, a retired requirement, and unrequested work found
in review.

<!-- AEGIS:END -->

## Repository environment tools

Keep repository-local setup, environment, validation, deployment, and
diagnostic tooling in `02_Tools/`. Downloaded runtimes and compiler binaries
belong under `02_Tools/runtime/` and are not committed; bootstrap and
environment scripts must recreate them from pinned, checksum-verified
archives without modifying the system-wide environment.

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
