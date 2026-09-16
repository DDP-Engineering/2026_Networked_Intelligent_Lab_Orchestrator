<!--
Copyright (c) 2026 DDP Engineering
This software is licensed under the PolyForm Noncommercial License 1.0.0.
Commercial use requires a separate commercial license.
See the LICENSE.txt in the .agentic folder for full license details.
SPDX-License-Identifier: LicenseRef-PolyForm-Noncommercial-1.0.0
-->


# TASK-a3dda009 - Derive the GUI lockfile digest in the toolchain manifest test

stage: done
type: change
path: quick
interaction: autopilot
requirements: REQ-PLAT-978855a5
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
`test_manifest_pins_supported_tool_archives` pinned `PLAYWRIGHT_LOCK_SHA256` as
a literal beside the digests of remote tool archives. Every other digest there
names bytes fetched from the internet, where a literal is the only way to state
which bytes were approved. That one is the digest of a file in this repository,
`05_Software/nilo_gui/package-lock.json`, which changes with any dependency or
version change - so the literal was a fourth copy of the same value, alongside
`toolchain.versions`, `build-environment.lock.json` and the lockfile itself.
This branch has already paid for it twice: the 0.7.0 version bump rewrote the
lockfile and the suite failed on the stale copies rather than on anything about
the toolchain.

Desired outcome:
The test asserts what the manifest must actually be right about - that it
records the digest of the lockfile in this tree - and stops carrying a copy
somebody must hand-synchronise.

Actors: a developer changing a GUI dependency or the project version; the
repository gates that read the manifest.

Primary scenario: trigger -> system action -> observable result
A developer changes `package-lock.json` and regenerates the manifest ->
the test recomputes the lockfile digest and compares -> it passes, with no
literal to update; if the manifest was not regenerated, it fails naming exactly
that.

In scope: the one assertion in
`05_Software/scripts/tests/test_toolchain_bootstrap.py`.

Out of scope: the digest recorded in `02_Tools/toolchain.versions` and
`02_Tools/containers/build/build-environment.lock.json`, and the runtime
comparison in `02_Tools/scripts/build_environment.py`. Those are the manifest
and its verifier, not copies, and they stay.

Constraints:
- OBSERVED `test_toolchain_bootstrap.py:117`: the expected key set is built from
  `*expected_digests`, so removing the entry also removes the key from the set
  the manifest is checked against. The key has to be named explicitly instead.
- STATED by the user: derive it rather than pin it.

Evidence:
- STATED (2026-09-16): "derive the playwright-lock digest instead of pinning it".
- OBSERVED: the same value appears in `toolchain.versions`,
  `build-environment.lock.json` and this test; `build_environment.py:162`
  compares the real file against the manifest at runtime.
- INFERRED: the literal was a consistency check between copies, not a
  supply-chain control over npm packages - the lockfile carries per-package
  integrity hashes of its own, and it is reviewed like any other tracked file.

Assumptions: none.
Unknowns: none.
Decision-blocking questions: none; the user decided.

Proposed behaviors:
- B1. The test computes the SHA-256 of `05_Software/nilo_gui/package-lock.json`
  and asserts the manifest records it, still checking the 64-hex shape and still
  requiring the key to be present in the manifest's key set.

Path and reason: quick.

| criterion | yes | no | evidence |
|---|---|---|---|
| no new externally observable behavior | yes | | a test's assertion strategy only |
| no new interface or data format | yes | | none |
| no new dependency | yes | | hashlib is in the standard library |
| an existing requirement already covers the behavior | yes | | REQ-PLAT-978855a5 covers the version/checksum manifest and its validation |
| no security or data-handling behavior changes | yes | | the manifest, its runtime verifier and the lockfile's own integrity hashes are untouched; what goes is a duplicate copy of a value, and the test still fails when the manifest and the lockfile disagree |

Every criterion answers yes, so the quick path applies.

## Requirement coverage


## Design


## Log

- 2026-09-16 frame: quick path. One assertion in the toolchain manifest test
  derives the GUI lockfile digest instead of repeating it; REQ-PLAT-978855a5
  already covers the manifest contract, so no requirement changes.
- 2026-09-16 build: the assertion now recomputes the digest from
  05_Software/nilo_gui/package-lock.json. Removing the entry from
  `expected_digests` also removed the key from the set the manifest is checked
  against, because that set is built with `*expected_digests`; the key is named
  explicitly now, so the manifest still has to carry it.
- 2026-09-16 build: the derived assertion was proved non-vacuous rather than
  assumed - `PLAYWRIGHT_LOCK_SHA256` in 02_Tools/toolchain.versions was
  temporarily perturbed and the test failed with "toolchain.versions does not
  record the digest of the GUI lockfile in this tree"; the manifest was then
  restored and `git diff` on it is empty. `python -m unittest discover -s
  scripts/tests`; exit=0; 118 tests OK.
- 2026-09-16 build: the repository anchor was taken after the edit rather than
  before it, so this task's evolution report reads 0 changed files and describes
  nothing. The candidate is one file and is visible in the staged diff; the
  anchor record is noted as uninformative rather than presented as evidence.
- 2026-09-16 validate: reviewed the candidate against REQ-PLAT-978855a5. The
  manifest contract is unchanged: `toolchain.versions` and
  `build-environment.lock.json` still record the digest and
  02_Tools/scripts/build_environment.py still compares the real lockfile against
  it at reconstruction time. What the test lost is a duplicate of that value;
  what it keeps is that the manifest carries the key and that the key is right.
  No requirement prose needed changing and no knowledge page describes the
  retired literal.
- 2026-09-16 validate final proof:
  `python .agentic/aegis.py check --task TASK-a3dda009 --strict --run`; exit 0;
  `test ok (379.2s)` and `lint ok (8.0s)`; `strict: ok`; 0 errors, 63 warnings,
  2 notes. The warning count is higher than the previous run only because this
  task's own journal and the closed TASK-60bcc914 are both counted by the
  advisory catalog; none is an error and none belongs to this change.
- 2026-09-16 validate review round 1: specification compliance approved and
  implementation quality approved on the current candidate.
- 2026-09-16 deliver: the user asked for this change and for the tree to be
  staged; they make the commit. The outcome is that one repository-local commit,
  so close-out is carried by it. The agent staged only.

## Rulings


## Handoff


current: done
status: ready
next: none - the human makes the staged commit
must_read: .agentic/tasks/TASK-a3dda009.md
blockers: none

## Stage report

stage: deliver
result: completed
current: done
changed: 05_Software/scripts/tests/test_toolchain_bootstrap.py, .agentic/tasks/TASK-a3dda009.md and its anchor/evolution records
evidence: python .agentic/aegis.py check --task TASK-a3dda009 --strict --run - exit 0, test ok 379.2s, lint ok 8.0s, strict: ok, 0 errors; the assertion was proved non-vacuous by perturbing the manifest and observing the named failure
issues: none open; the anchor for this task was taken after the edit, so its evolution report is uninformative and is recorded as such
next: none - the human commits the staged tree
needs_user: no
spec_compliance: approved
implementation_quality: approved
review_round: 1


## Delivery report (2026-09-16)

- stage: done; type: change; path: quick; branch: ddp/nilo-password-policy
- requirements:
  - REQ-PLAT-978855a5 - Bootstrap repository-local development toolchain
- deviations: none recorded
- explicit review: enabled; spec_compliance=approved; implementation_quality=approved; review_round=1
- diff vs task base 0c59286: 46 file(s) changed, 9289 line(s) added
- checks: errors=0 warnings=62 notes=2
- checklist: 6/6 done
