<!--
Copyright (c) 2026 DDP Engineering
This software is licensed under the PolyForm Noncommercial License 1.0.0.
Commercial use requires a separate commercial license.
See the LICENSE.txt in the .agentic folder for full license details.
SPDX-License-Identifier: LicenseRef-PolyForm-Noncommercial-1.0.0
-->


# TASK-c99c640b - Separate software components and retire the fake Python reference integration

stage: done
type: change
path: quick
interaction: autopilot
requirements: REQ-TEST-2bbda8a9, REQ-TEST-b52051b8
parent:
branch: master
base: b83555684d11c359c41676a962dc0f0860e1335d
opened: 2026-09-25

## Done checklist (current stage)

- [x] validated candidate confirmed unchanged; otherwise returned to validate
- [x] validation evidence, diff, exclusions, and outcome presented
- [x] delivery confirmed by the human
- [x] journal and report included in the delivery commit
- [x] agreed outcome delivered, normally a PR
- [x] close-out committed after the outcome succeeded, or carried by the delivery commit where nothing could fail after it

## Frame

Problem: The software tree groups product components, contracts, fixtures, and tooling under historical names.
Desired outcome: Distinct component directories, repository tooling in 02_Tools, and no fake Python reference integration in product integrations.
Actors: NILO maintainers, test runners, packaging and development scripts.
Primary scenario: A maintainer opens 05_Software and finds frontend, core, integrations, contracts, and cross-component tests; existing verification and development commands resolve their new paths.
Examples: nilo_gui becomes frontend; nilo_assistant becomes core; public schemas become contracts/integration/v1.
Counterexamples: Removing Sigrok hardware or decoder source, changing the integration wire protocol, or shipping a test fixture.
In scope: Relocate component projects; replace fake_python with a test-only protocol fixture; update path references, requirements, knowledge, tooling, tests, and changelog.
Out of scope: PulseView implementation, Sigrok retirement, Go package redesign, API changes, version assignment, release packaging or publication.
Constraints: Preserve runtime behavior and contract coverage. Do not change Git state without a named Git instruction. Keep generated artifacts ignored and outside tracked source.
Success signals: Component paths are coherent; no fake Python reference package remains; focused and full checks pass or limitations are reported accurately.
Evidence: STATED by user: approved proposed plan and later PulseView migration. OBSERVED: 05_Software/README.md, .agentic/context/project.md, 05_Software/tests/contract/test_integration_contract.py, .agentic/aegis.toml, 02_Tools/scripts. INFERRED: relocation has broad path effects. UNKNOWN: future PulseView internal architecture.
Assumptions: A test-only deterministic process fixture is still needed to preserve contract and acceptance coverage.
Unknowns: Future integration languages after Sigrok retirement.
Decision-blocking questions: None for the authorized layout and fixture replacement.
Proposed behaviors: Clear top-level component ownership; external runner contract tests remain executable; development, build, and packaging references follow new paths.
Path and reason: Quick path. No new user-facing behavior, wire/data format, dependency, or security boundary; existing test requirements cover the fixture. The user explicitly requested end-to-end implementation, so proceed through stages without intermediate checkpoint pauses. Stay on master because AGENTS.md Git authority prohibits branch creation absent a named instruction.

## Requirement coverage


## Design


## Log

- 2026-09-25 frame: Approved component-layout plan; quick path because product behavior and contracts remain unchanged. End-to-end implementation requested; Git branch creation is outside granted authority.
- 2026-09-25 build red: REQ-TEST-2bbda8a9, REQ-TEST-b52051b8; command=`.\02_Tools\env\with-tools.ps1 uv run --project 05_Software/integrations/fake_python --all-groups pytest -q 05_Software/tests/contract/test_integration_contract.py::test_contract_runner_is_a_test_only_go_fixture`; exit=1; expected missing behavior=test-only Go fixture absent and fake Python reference package still present.
- 2026-09-25 build: Relocated 614 tracked files into component roots and `02_Tools`; 586 are byte-identical after newline normalization, 28 carry intentional path, fixture, simulation-catalog, or test updates, and none are missing. Removed the fake Python package and added a test-only Go contract runner. Preserved Sigrok hardware and decoder sources; the future PulseView feature remains out of scope.
- 2026-09-25 build green: Contract suite 11/11, focused Go acceptance and manifest tests, frontend 506/506, and SDK/Sigrok tests passed with the new paths and fixture. The SDK dev lock includes the already escrowed `jsonschema` wheel used by the contract harness. `git diff --check` passed. The saved evolution report covers the final executable candidate.
- 2026-09-25 build to validate: Two linked active test requirements and their paths remain current; updated component knowledge, project/environment context, repository tooling and documentation. The advisory check has no errors; relocation appears as new files until staged, creating coverage and secret-pattern warnings for existing code.
- 2026-09-25 validate review round 1: Specification compliance approved after checking the new component roots, fixture-only runner, active requirement paths, and exclusions; Sigrok hardware and decoders remain intact, with PulseView work deferred. Implementation quality approved in a separate pass over changed relocations, process-fixture failure paths, portable scripts, release exclusions, and test results. No manual or inspection method is specified by the linked requirements. The touched Increment 1 knowledge page was brought to the current template; this was prose-only after runtime proof.
- 2026-09-25 validate proof: `.\02_Tools\env\with-tools.ps1 python .agentic/aegis.py check --task TASK-c99c640b --strict --run` exited 0: test command passed in 647.8s and lint passed in 10.3s, covering 187 tooling tests, Go suites, SDK and Sigrok Python suites, 11 runner-contract tests, 506 frontend tests and build, and the WSL appliance browser stage. The final doc-only knowledge edit required no runtime rerun; evolution was refreshed and `check --strict` still exits 0 with no errors. Advisory warnings after that edit: 598 uncovered-source (unstaged moves presented as new paths), 40 secret-pattern findings on relocated existing code/tests, one pre-existing tracked-fingerprint warning, and two unrelated open delivery checklists; three unrelated manual-verification notes. No verification failure is accepted.
- 2026-09-25 validate to deliver: Candidate is locally complete and validated. Repository Git authority bars staging, committing, or opening a PR without a named user instruction; present the exact delivery action and leave the journal at deliver.
- 2026-09-25 deliver to validate: The subsequently authorized project-version assignment changed Go build info, frontend manifests, toolchain lock hashes, changelog, and current Pi documentation after the original full proof. The Pi development deployment passed live smoke checks. The user has now asked for a commit and task close-out; revalidate the integrated 0.9.0 tree before Git delivery. No push or PR was requested.
- 2026-09-25 validate to build: The full strict run of the staged 0.9.0 tree exited 1 after 187 tooling tests (one failure, four errors); the repository lint command passed. All five packaging failures came from reproducibility and build-receipt fixtures that still named 0.8.0 while the release builder correctly bound artifacts to the root 0.9.0 version. The remaining suites did not run in this attempt.
- 2026-09-25 build to validate: Current-release packaging fixtures now read root `VERSION`, and the receipt helper derives each artifact version from its filename; explicit historical-version cases remain unchanged. Focused command=`.\02_Tools\env\with-tools.ps1 python -m unittest discover -s 02_Tools/tests/software -p test_release_packaging.py -q`; exit=0; 48 tests passed, five platform-specific skips. Staged whitespace check passed and evolution was refreshed.
- 2026-09-25 validate review round 2: Specification compliance approved for the integrated 0.9.0 candidate: component boundaries and the two linked active requirements still hold, the shared protocol is unchanged, Sigrok hardware/decoders remain, and PulseView is deferred. Implementation quality approved independently after checking the version mirrors and lock hash, receipt-fixture fix, 624 detected renames, eight intended deletions, three new files, path consumers, portable execution, and clean staged whitespace. No linked requirement specifies manual or inspection verification.
- 2026-09-25 validate final proof: `.\02_Tools\env\with-tools.ps1 python .agentic/aegis.py check --task TASK-c99c640b --strict --run`; exit=0; test passed in 546.9s, lint in 10.1s. The full router ran tooling, Go, SDK/Sigrok and contract Python, 506 frontend tests/build, and WSL appliance browser checks. Advisory output had zero errors, 601 warnings and three unrelated manual-verification notes. Warnings comprise relocated source paths lacking per-file affected-path links, one pre-existing tracked-fingerprint notice, two unrelated open delivery checklists, and this task's review-state notice before these round-2 verdicts were recorded. No current-task verification failure remains.
- 2026-09-25 validate to deliver: Final evolution describes the integrated tree after the packaging fixture fix. The user explicitly requested a repository-local commit and task close-out; no push or PR was requested.
- 2026-09-25 deliver decision: The user requested "commit files" and task close-out. The local outcome is one commit on `master` containing the component relocation, 0.9.0 version assignment, and Pi deployment record; generated binaries/runtime files and unrelated open task journals are excluded. Staged review shows 624 renames, eight deletions, three additions and 425 modified paths. No push or PR is authorized.
- 2026-09-25 deliver close-out: The AEGIS delivery report was appended after the approved local-commit decision. The validated candidate is unchanged except for journal-only delivery records. Because the repository-local commit is the entire authorized outcome, the done state and close-out are carried by that single commit; no push or PR follows.

## Rulings


## Handoff


current: done
status: ready
next: none - local delivery commit carries the task close-out
must_read: .agentic/tasks/TASK-c99c640b.md
blockers: none

## Stage report

stage: deliver
result: completed
current: done
changed: component roots, test fixture, tooling, path consumers, requirements, knowledge, and documentation
evidence: integrated strict gate exit 0 (test 546.9s, lint 10.1s); staged diff check clean; live Pi smoke passed
issues: advisory source-coverage warnings from relocation and unrelated repository notices; no validation failure
next: none after the local commit
needs_user: no
spec_compliance: approved
implementation_quality: approved
review_round: 2

## Delivery report (2026-09-25)

- stage: deliver; type: change; path: quick; branch: master
- requirements:
  - REQ-TEST-2bbda8a9 - Verify integrations with a common contract
  - REQ-TEST-b52051b8 - Prove the integration lifecycle with a test runner
- deviations: none recorded
- explicit review: enabled; spec_compliance=approved; implementation_quality=approved; review_round=2
- diff vs task base b835556: 1060 file(s) changed, 6435 line(s) added
- checks: errors=0 warnings=601 notes=3
- checklist: 3/6 done; open: journal and report included in the delivery commit; agreed outcome delivered, normally a PR; close-out committed after the outcome succeeded, or carried by the delivery commit where nothing could fail after it
