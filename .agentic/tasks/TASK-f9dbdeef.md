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

# TASK-f9dbdeef — Add cross-component browser verification

stage: deliver
type: feature
path: full
requirements: REQ-TEST-0a4f49af, REQ-TEST-0f02f4b8, REQ-TEST-7ff6a75d, REQ-TEST-3c4487d6, REQ-REL-f04744fa, REQ-REL-f0c8b07c, REQ-SEC-4884bc91, REQ-PLAT-38014f08, REQ-PLAT-59993b78, REQ-UX-f479d8b9
parent:
branch: master
base: 0cb77320e22f6762625694c814120d86cf253477
opened: 2026-09-01

## Done checklist (current stage)

- [x] validated candidate confirmed unchanged; otherwise returned to validate
- [x] validation evidence, diff, exclusions, and outcome presented — twice, the
      second time after the human's review found two flaws that sent it back
- [x] delivery confirmed by the human
- [x] journal and report included in the delivery commit
- [ ] agreed outcome delivered — the human makes the commit themselves; this
      journal is staged with it
- [ ] close-out committed after the outcome succeeded

## Log

- 2026-09-01 frame: scope is vertical-slice 6 - a deterministic browser end-to-end path that drives the real native Windows Vite GUI in a real browser against a real Go Bench Core running under WSL2 on the simulation profile, so that the browser-to-WSL2 path becomes repository-gated evidence instead of a manual browser check recorded in a journal.
- 2026-09-01 frame: scope covers the pinned browser test dependency (Playwright, exact version in `package.json` and `package-lock.json`), its browser binaries kept under the untracked `02_Tools/runtime/windows-x86_64/` area, a bootstrap step that reproduces them without touching the persistent host environment, and the guarantee that an ordinary test run downloads nothing.
- 2026-09-01 frame: scope covers one obvious headless command for the integrated browser test plus an optional headed mode for diagnosis, both routed through the existing repository-local tool wrapper.
- 2026-09-01 frame: scope covers the harness itself - choose isolated free ports, create WSL-native temporary runtime state, build and start Bench Core under WSL2 on the simulation profile, start the native Windows Vite server pointed at that backend port, issue one development token and keep its plaintext in process memory only, wait for both endpoints with bounded timeouts, and tear down exactly what that invocation created, including on failure.
- 2026-09-01 frame: scope covers the seven browser scenarios the slice owes - development-token login, authenticated status loading, prominent simulation indication, healthy service status, the deterministic integration inventory, the deterministic device inventory, the backend-unavailable presentation, and recovery through the interface's own retry/refresh control once the backend answers again.
- 2026-09-01 frame: scope covers integrating the browser test into the repository's full AEGIS gate on the supported Windows plus WSL2 host, where a missing prerequisite fails with an actionable message rather than being skipped and then presented as browser-to-WSL proof.
- 2026-09-01 frame: scope covers the documentation the user named - `00_Planning/Development_Simulation_Vertical_Slices.md`, `06_SoftwareDocu/Development_and_Simulation_Flow.md`, `05_Software/nilo_gui/README.md`, `02_Tools/README.md`, the affected AEGIS requirements and knowledge pages, and `CHANGELOG.md` under Unreleased.
- 2026-09-01 frame: out of scope, stated by the user - any supervision, metadata, `stop` or `logs` behavior added to `02_Tools/scripts/nilo-dev.ps1`, which stays a visible manual development entry point that owns no processes and is deliberately not the automated test supervisor; also out of scope are MCP operations, hardware or PLC behavior, Raspberry Pi deployment, packaging, and any new product API endpoint or backend wire-format change.
- 2026-09-01 frame: out of scope by exclusion - killing an unrelated process or forcing a port free, retaining traces, screenshots, videos, tokens or databases after a successful run, and changing the existing unit/component tests, mock mode, bootstrap behavior or manual `nilo-dev.ps1` workflow.
- 2026-09-01 frame: full path chosen - the slice adds a new dependency, a new command surface, a new runtime-state layout under `02_Tools/runtime`, and new security-relevant token-handling behavior, none of which an existing requirement covers and each of which the quick path forbids.
- 2026-09-01 frame: no child tasks - the harness, its configuration, its bootstrap step and its one spec file are a single tightly coupled unit inside `05_Software/nilo_gui` plus two `02_Tools` scripts, too small and too interdependent to govern separately.
- 2026-09-01 frame: area located and skimmed - `05_Software/nilo_gui` (new `playwright.config.ts` and `e2e/`, plus `package.json`, `package-lock.json`, `README.md`, `.prettierignore`, `eslint.config.js`), `02_Tools` (`env/with-tools.ps1`, a new browser bootstrap script, `bootstrap-development.ps1`, `README.md`), `05_Software/scripts/aegis_test.py` for the gate, `.gitignore` for the generated test output directory, and the three documents plus `.agentic/knowledge/gui-foundation.md` and `repository-toolchain.md`.
- 2026-09-01 frame: read the backend command surface before framing - `nilo serve --profile simulation [--scenario healthy|unavailable] --data-dir <absolute> --bind <loopback>` prints one `{"status":"listening",...}` line when the listener is up, and `nilo development issue-token --data-dir <absolute>` prints a JSON document whose `token` field is the plaintext; both are in `05_Software/nilo_assistant/cmd/nilo/main.go`.
- 2026-09-01 frame: read the interface before framing - the browser only ever requests origin-relative `/api/v1/*` paths and the Vite server forwards `/api` to `VITE_NILO_API_BASE_URL`, so the harness must give Vite the chosen backend port at start; a backend that is down turns into a non-`ok` proxy response or a fetch error, which `src/api/client.ts` classifies as `unavailable`, which is exactly the presentation this slice must observe.
- 2026-09-01 frame: read the deterministic catalog before framing - `05_Software/nilo_assistant/internal/simulation/catalog.go` serves exactly one integration (`simulation.fake-python`) and one device (`simulation.fake-python.device.stable-001`) with `SIMULATION - ` display names, so the browser assertions can be exact rather than approximate.
- 2026-09-01 frame: user constraints recorded - stay on `master`, do not use Superpowers, commit nothing until explicit delivery approval, leave the untracked `00_Planning/TODO.md` exactly as it is and outside this task, add no supervision to `nilo-dev.ps1`, and do not mark slice 6 implemented until the browser test has actually passed against the real WSL2 backend.
- 2026-09-01 frame: before opening this task, reconciled TASK-8a4ba5da - the human had approved it and then committed and pushed 0cb7732, leaving that journal at `stage: deliver` with two unticked items; the commit was verified to carry the journal and `origin/master` was verified level with `master`, so the journal was closed out to `stage: done` truthfully without amending anything.
- 2026-09-01 frame: that close-out is a journal-only edit that no commit yet carries, so it is deliberately part of this task's candidate and will be presented in this task's staged files at the delivery checkpoint, exactly as TASK-8a4ba5da carried TASK-8fd83f88's.
- 2026-09-01 frame: expected requirements - one REQ-TEST for the integrated browser journey itself, one REQ-TEST for the harness owning only what it started and cleaning it up on every path, one REQ-TEST for the gate integration and its actionable missing-prerequisite failure, one REQ-SEC for the development token never being printed, persisted or exposed in a failure artifact, one REQ-PLAT for the pinned, repository-local browser runtime that no test run downloads, and one REQ-UX for the recovery-through-the-interface behavior if the existing screens do not already cover it.
- 2026-09-01 specify: wrote ten requirements, splitting the frame proposal where a single line held two behaviors - REQ-TEST-0a4f49af (the authenticated journey), REQ-TEST-0f02f4b8 (unavailable then recovery), REQ-TEST-7ff6a75d (the gate runs it), REQ-TEST-3c4487d6 (a named missing prerequisite fails the run), REQ-REL-f04744fa (bounded readiness naming the silent endpoint), REQ-REL-f0c8b07c (release only what the run created), REQ-SEC-4884bc91 (the token stays in memory and out of every artifact), REQ-PLAT-38014f08 (pinned repository-local browser runtime), REQ-PLAT-59993b78 (free ports and WSL-native temporary state), and REQ-UX-f479d8b9 (one headless command plus a headed variant).
- 2026-09-01 specify: no requirement was reused - the existing GUI requirements describe what the interface does, and the existing toolchain requirements describe bootstrap and the manual lifecycle; none of them describes a browser exercising the real WSL2 backend, so all ten are new and none is retired.
- 2026-09-01 specify: no spec written - each requirement is buildable from its own prose plus the design note, and no new wire format, protocol or non-obvious algorithm is introduced.
- 2026-09-01 specify: command=`python .agentic/aegis.py check`; exit=0; errors=0 warnings=19 notes=0 - every warning is `stale-paths` for the ten new requirements, because their `affected_paths` name files build has not created yet; they are expected to clear at build and are not accepted as standing warnings.
- 2026-09-01 design: wrote the design note - `@playwright/test` pinned at 1.62.1 (the version `npm view` reports as current), browser binaries under `02_Tools/runtime/windows-x86_64/playwright` through a `PLAYWRIGHT_BROWSERS_PATH` set by `with-tools.ps1` for its child process only, a new `bootstrap-browser.ps1` wired into the combined bootstrap, and a harness under `05_Software/nilo_gui/e2e/` that owns its own short-lived processes.
- 2026-09-01 design: rejected driving the topology from `nilo-dev.ps1` - it owns no process and signals nothing by design, so a test could neither stop the backend mid-run to observe the unavailable screen nor prove it cleaned up only its own work; the user also required it not be the automated supervisor.
- 2026-09-01 design: chose a file-suffix split, `*.test.ts` for Vitest and `*.spec.ts` for Playwright, because Vitest's default include would otherwise collect the browser spec and run it in jsdom; `vite.config.ts` gains an explicit Vitest `include` and `tsconfig.node.json` gains the harness so `tsc -b` type-checks it.
- 2026-09-01 build red: command=`.\02_Tools\env\with-tools.ps1 npm.cmd --prefix .\05_Software\nilo_gui run test:ci`; exit=1; 3 of 10 test files failed - `e2e/topology.test.ts` and `e2e/prerequisites.test.ts` cannot resolve `./topology` and `./prerequisites`, and `e2e/commands.test.ts` cannot read `playwright.config.ts`; expected missing behavior=the end-to-end harness, its prerequisite checks and its browser configuration do not exist yet. The 113 existing component tests all passed, so the baseline is intact (REQ-TEST-3c4487d6, REQ-REL-f04744fa, REQ-REL-f0c8b07c, REQ-SEC-4884bc91, REQ-PLAT-59993b78, REQ-UX-f479d8b9).
- 2026-09-01 build red: command=`python -m unittest discover -s scripts/tests -p test_aegis_test.py` from `05_Software`; exit=1; 2 failures - the Windows full run stops after `build` instead of running `test:e2e`, and a failing browser run returns 0 rather than its own exit code; expected missing behavior=the verification router does not run the browser suite at all (REQ-TEST-7ff6a75d).
- 2026-09-01 build red: command=`python -m unittest discover -s scripts/tests -p test_toolchain_bootstrap.py` from `05_Software`; exit=1; 3 failures - the interface manifest declares no `test:e2e` scripts, `02_Tools/bootstrap/bootstrap-browser.ps1` does not exist, and the combined bootstrap does not call it; expected missing behavior=there is no pinned, repository-local browser runtime (REQ-PLAT-38014f08).
- 2026-09-01 design: revised the `## Architecture` diagrams on `.agentic/knowledge/gui-foundation.md` and `.agentic/knowledge/repository-toolchain.md` to the shape they will have after this change, each marked as an intended shape naming this task; both markers are removed at validate once the code matches.
- 2026-09-01 build: `python .agentic/aegis.py anchor --task TASK-f9dbdeef` recorded 206 files at 0cb7732 after the three red records above and before the first implementation edit.
- 2026-09-01 build: pinned `@playwright/test` at 1.62.1 and installed Chromium into `02_Tools/runtime/windows-x86_64/playwright/`; the per-user cache at `%LOCALAPPDATA%\ms-playwright` was checked before and after and every entry in it predates this session, so the repository-local install wrote nothing outside the checkout.
- 2026-09-01 build: the first live run failed on a real defect the harness itself surfaced - a Windows path handed to `wsl.exe` as a bare argument arrives with its backslashes stripped, so `wslpath -a F:\checkout` became `wslpath: F:checkout`; the translation now goes through `sh -c` with a shell-quoted forward-slash path. The prerequisite check reported it as an actionable failure naming `doctor-wsl.ps1`, which is the behavior REQ-TEST-3c4487d6 asks for.
- 2026-09-01 build: command=`.\02_Tools\env\with-tools.ps1 npm.cmd --prefix .\05_Software\nilo_gui run test:e2e`; exit=0; 2 browser tests passed in 9.9s against a Bench Core the suite compiled and ran inside `Ubuntu-24.04` on the simulation profile - this is the first automated browser-to-WSL2 proof in this repository.
- 2026-09-01 build: added a source assertion that the browser spec names the token exactly once and only inside `fill`, because Playwright still writes an error-context note on an assertion failure; that note carries the failing locator's aria snapshot and the spec source, neither of which can hold a password field's value.
- 2026-09-01 build: command=`.\02_Tools\env\with-tools.ps1 npm.cmd --prefix .\05_Software\nilo_gui run test:ci`; exit=0; 146 tests in 10 files, up from 113 in 7 - every pre-existing component test still passes.
- 2026-09-01 build: DEVIATION build - `python .agentic/aegis.py evolution --task TASK-f9dbdeef` reported 23 changed files, one of which, `05_Software/nilo_gui/.prettierignore`, the design note's expected-file list did not name. It was needed because Playwright's generated `test-results/` output would otherwise be reformatted by `format` and then fail `format:check`; the frame log did name the file, the design note dropped it.
- 2026-09-01 build: the evolution report is anchor-relative, so `05_Software/scripts/tests/test_aegis_test.py` and `test_toolchain_bootstrap.py` are absent from it - both were written before the anchor, as the red-first order requires, and are in its baseline. `00_Planning/TODO.md` does not appear at all: it is untracked, unread and unmodified by this task.
- 2026-09-01 validate: reviewed the diff hunk by hunk against the frame scope - `02_Tools/scripts/nilo-dev.ps1` and `02_Tools/lib/DevLifecycle.ps1` are untouched and unreferenced by the harness, as RULING-004 requires; no product API, wire format or backend behavior changed; no supervision, metadata, stop or logs behavior was added anywhere.
- 2026-09-01 validate: reviewed the diff for unrequested work - none found. `05_Software/nilo_gui/.prettierignore` and `tsconfig.node.json` are the two edits no requirement first named; both are consequences of adding the suite (keeping generated `test-results/` out of the format gate, and letting `tsc -b` type-check the harness) and both are now on a requirement's `affected_paths`.
- 2026-09-01 validate: corrected an untruthful comment found in review - `bootstrap-frontend.ps1` claimed the browser test package fetches browsers from an install hook during restore. The pinned lockfile records `hasInstallScript: false` for `@playwright/test`, `playwright` and `playwright-core` at 1.62.1, so it does not. The guard is kept as protection against a future version reintroducing one, and the comment and `02_Tools/README.md` now say exactly that.
- 2026-09-01 validate: manual observation for REQ-UX-f479d8b9 - command=`.\02_Tools\env\with-tools.ps1 npm.cmd --prefix .\05_Software\nilo_gui run test:e2e:headed`; exit=0; 2 passed in 10.4s with the browser window visible, so the headed variant differs from the headless one only by showing the browser.
- 2026-09-01 validate: manual observation for REQ-PLAT-59993b78 - a helper process was made to hold both default development ports, 8090 and 5173, on 127.0.0.1; the browser suite then ran to `exit=0` with 2 passed in 8.6s, and the holder was still alive afterwards. The suite neither collided with the held ports nor touched the process holding them.
- 2026-09-01 validate: manual observation for REQ-REL-f0c8b07c and REQ-SEC-4884bc91 - after a passing run, `ls -A "$HOME/.local/state/nilo-e2e"` inside `Ubuntu-24.04` reports 0 entries and `ps -eo pid,args | grep "[n]ilo serve"` reports none, so the run's temporary directory and its backend are both gone; a repository-wide search for a bearer-token or `"token": "<value>"` pattern outside `node_modules`, `runtime` and `cache` returns nothing.
- 2026-09-01 validate: DEVIATION validate - a successful run leaves one 45-byte file, `05_Software/nilo_gui/test-results/.last-run.json`, holding `{"status":"passed","failedTests":[]}`. Playwright writes it after the run has finished, so the suite cannot remove it, and chaining a cleanup onto the npm script would mask the command's exit code. It is ignored by Git and carries no trace, screenshot, video, token or database, but it is not literally nothing; it is reported rather than described as an empty directory.
- 2026-09-01 validate: command=`.\02_Tools\env\with-tools.ps1 python scripts/aegis_test.py full` from `05_Software`; exit=0; the Windows frontend suite ran `npm.cmd run test:ci` (146 tests), then `npm.cmd run build`, then `npm.cmd run test:e2e` with both browser tests passing in 10.2s - direct evidence that the repository gate itself performs the browser-to-WSL2 proof, which is what REQ-TEST-7ff6a75d claims.
- 2026-09-01 validate: command=`.\02_Tools\env\with-tools.ps1 python .agentic/aegis.py check --task TASK-f9dbdeef --strict --run`; exit=0; `run: test ok` (54.5s), `run: lint ok` (3.9s), `strict: ok`, errors=0 warnings=1. The first strict run reported 4 warnings; the stale Handoff and the two `uncovered-source` warnings were fixed rather than accepted, leaving only the accepted `outside-layout` warning for `CHANGELOG.md`.
- 2026-09-01 validate: command=`git diff --check`; exit=0; `00_Planning/TODO.md` is still untracked, unread and outside this candidate, and no generated `test-results/` or browser runtime appears in `git status`.
- 2026-09-01 validate: reconciled both knowledge pages against the built code and removed the intended-shape markers design added; `gui-foundation.md` gained the harness, prerequisite and spec nodes plus their files and behaviors, and `repository-toolchain.md` gained the browser bootstrap, the browser runtime and the gate step.
- 2026-09-01 deliver: the human's review of the presented candidate found two implementation flaws, both valid, so the candidate returns to build under `5_validate.md` step 2 rather than being patched at deliver; the delivery presentation is withdrawn and no commit was made.
- 2026-09-01 build: review finding 1 reproduced and confirmed - a failing `fill()` can repeat its argument in `error.message`. A throwaway probe with a dummy marker over six failure modes showed the leak is conditional on the locator resolving first: `locator-never-resolves`, `element-is-not-fillable`, `strict-mode-violation` and `page-closed` do not leak, while `element-disabled` and `element-detached-mid-action` both print `fill("DUMMY-MARKER-9f3a2b7c")` in the call log. The interface's token field always exists, so ours is the leaking shape, and that message reaches both the console reporter and `test-results/.../error-context.md`. My earlier claim that turning off trace, video and screenshot was sufficient was wrong.
- 2026-09-01 build: review finding 2 confirmed by reading the code - `releaseOwned` discarded the release command's exit code entirely and swallowed every exception, so a `rm -rf` that failed inside the distribution was indistinguishable from one that succeeded and the run still passed. REQ-REL-f0c8b07c's verification asserted the removal was attempted, not that it succeeded, so the requirement was too weak as written and is amended rather than merely re-tested.
- 2026-09-02 build red: command=`.\02_Tools\env\with-tools.ps1 npm.cmd --prefix .\05_Software\nilo_gui run test:ci`; exit=1; 10 of 23 tests in `e2e/topology.test.ts` failed with `TypeError: withRedactedSecret is not a function` and on the release assertions; expected missing behavior=there is no redaction wrapper, `releaseOwned` returns successfully whatever the release did, and the release script does not check the directory is gone (REQ-SEC-4884bc91, REQ-REL-f0c8b07c).
- 2026-09-02 build: fixed finding 1 structurally rather than by filtering strings - `DevelopmentTopology` no longer exposes a `token` accessor at all. The value stays in one private field and reaches the browser only through `enterTokenInto(field)`, which wraps the action in `withRedactedSecret` so the message and the stack of anything thrown have the value replaced by `<redacted development token>`. The browser spec now names neither the token nor `fill`.
- 2026-09-02 build: added the forced-failure regression the review asked for, in `e2e/token-redaction.spec.ts` - one test drives a real browser `fill` against a deliberately disabled field, which is the failure shape that leaks, and asserts the surfaced message and stack carry the marker and not the value; a companion test asserts the same unwrapped call still does leak it, so the premise is guarded and the wrapper cannot be dropped on a guess if Playwright changes.
- 2026-09-02 build: fixed finding 2 - `releaseOwned` now attempts every step, collects failures instead of swallowing them, checks the release command's exit code, and raises a `CleanupError` naming what failed and the directory that may survive. `buildReleaseScript` no longer trusts `rm -rf`: it tests the path afterwards and exits 1 if anything survived. `stopBackend` checks its own exit code, and the start-up failure path appends a cleanup failure to the original error rather than replacing it.
- 2026-09-02 build: verified the premise of that fix rather than assuming it - a throwaway probe spec whose `afterAll` throws was run and reported as `1 failed` with exit 1, so a `CleanupError` from `dispose()` genuinely fails the run. The probe was deleted.
- 2026-09-02 build: amended REQ-SEC-4884bc91 and REQ-REL-f0c8b07c rather than only adding tests - the first now requires no accessor and no leak through a failing action's error, the second requires the removal to be confirmed and the run to fail when the release did not succeed. Both were too weak as written, which is why the flaws passed validation the first time.
- 2026-09-02 build: command=`.\02_Tools\env\with-tools.ps1 npm.cmd --prefix .\05_Software\nilo_gui run test:ci`; exit=0; 155 tests in 10 files, up from 146.
- 2026-09-02 build: command=`.\02_Tools\env\with-tools.ps1 npm.cmd --prefix .\05_Software\nilo_gui run test:e2e`; exit=0; 4 browser tests passed in 14.7s - the two journey tests and the two forced-failure redaction tests.
- 2026-09-02 validate: re-reviewed the changed hunks; `format:check`, `lint` and `build` each exit=0; `python .agentic/aegis.py evolution --task TASK-f9dbdeef` reports 24 files, the previous 23 plus `e2e/token-redaction.spec.ts`, which is the file the review asked for.
- 2026-09-02 validate: command=`.\02_Tools\env\with-tools.ps1 python .agentic/aegis.py check --task TASK-f9dbdeef --strict --run`; exit=0; `run: test ok` (59.5s), `run: lint ok` (4.0s), `strict: ok`, errors=0 warnings=1 after refreshing the stale Handoff - the accepted `outside-layout` warning for `CHANGELOG.md` again.
- 2026-09-02 validate: re-verified cleanup after the fixes - `ls -A "$HOME/.local/state/nilo-e2e"` reports 0 entries, no `nilo serve` process remains, the token scan finds nothing, and `git diff --check` exits 0.
- 2026-09-02 deliver: the human approved delivery of the re-proved candidate and chose to make the commit themselves; the candidate was staged for them unchanged, with `00_Planning/TODO.md` deliberately left unstaged and untracked as required throughout this task.
- 2026-09-02 deliver: the appended report says 43 changed files against 0cb7732 while this journal says 42; the difference is `00_Planning/TODO.md`, which `collect_diff` counts because it is untracked in the working tree. It is not staged and is not part of this candidate, exactly as in the previous task.
- 2026-09-02 deliver: the delivery carries TASK-8a4ba5da's close-out edit under RULING-003; this journal's own close-out cannot be carried by the same commit, because the outcome - the human's commit and any push that follows - is not observable to me beforehand, so `stage: deliver` stands until it succeeds.
- 2026-09-01 build: DEVIATION build - `python .agentic/aegis.py check` exits 0 with errors=0 warnings=1, the `outside-layout` warning for `CHANGELOG.md`. All 19 `stale-paths` warnings from specify have cleared. The remaining one is accepted rather than silenced: `aegis.toml`'s `[layout] docs` globs list `README.md` but not `CHANGELOG.md`, the entry was explicitly requested by the user, and the honest fix is one line of configuration outside this task's scope. The previous task accepted the same warning for the same reason.

## Design

Components: the browser GUI project `05_Software/nilo_gui` (new `playwright.config.ts`
and `e2e/`), the repository toolchain `02_Tools` (wrapper and a new browser bootstrap),
and the verification router `05_Software/scripts/aegis_test.py`. `nilo-dev.ps1` and
`DevLifecycle.ps1` are untouched (RULING-004).

Interface: `@playwright/test` pinned at 1.62.1 in `package.json` and the lockfile
(REQ-PLAT-38014f08); two new manifest scripts, `test:e2e` headless and
`test:e2e:headed` (REQ-UX-f479d8b9); `bootstrap-browser.ps1` installs one browser,
Chromium, and `bootstrap-development.ps1` calls it after the frontend restore.
`with-tools.ps1` exports `PLAYWRIGHT_BROWSERS_PATH` under
`02_Tools/runtime/windows-x86_64/playwright` for its child only, and
`bootstrap-frontend.ps1` sets `PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD` around `npm ci` so a
locked restore can never populate a user-wide cache (REQ-PLAT-38014f08).

Data: no persisted state. Each run picks two free loopback ports by binding
`127.0.0.1:0` and releasing, and creates one `mktemp -d` directory under
`$HOME/.local/state/nilo-e2e/` inside the distribution, refusing a home that resolves
under `/mnt/` (REQ-PLAT-59993b78). The Go binary, the SQLite database and the recorded
backend PID live there and the whole directory is removed at teardown
(REQ-REL-f0c8b07c). The token is parsed out of `issue-token` stdout into one in-memory
field; `playwright.config.ts` sets trace, video and screenshot to `off` on every
outcome, so no artifact can capture it (REQ-SEC-4884bc91).

File-suffix split so the two runners never collide: `*.test.ts` is Vitest, `*.spec.ts`
is Playwright. `vite.config.ts` gains an explicit Vitest `include`; `tsconfig.node.json`
gains `e2e` and `playwright.config.ts` so `tsc -b` type-checks the harness.

Rejected alternative: driving the topology from `nilo-dev.ps1`. It deliberately owns no
process, records no identifier and signals nothing, so a test could not stop the backend
mid-run to observe the unavailable screen (REQ-TEST-0f02f4b8) and could not guarantee it
cleaned up only its own work (REQ-REL-f0c8b07c). The harness owns its own short-lived
processes instead, which is also what the user required.

Test seams: `e2e/bench-core.spec.ts` for REQ-TEST-0a4f49af and REQ-TEST-0f02f4b8;
`e2e/topology.test.ts` for REQ-REL-f04744fa, REQ-REL-f0c8b07c, REQ-SEC-4884bc91 and
REQ-PLAT-59993b78; `e2e/prerequisites.test.ts` for REQ-TEST-3c4487d6;
`e2e/commands.test.ts` for REQ-UX-f479d8b9 and the artifact half of REQ-SEC-4884bc91;
`05_Software/scripts/tests/test_aegis_test.py` for REQ-TEST-7ff6a75d; and
`05_Software/scripts/tests/test_toolchain_bootstrap.py` for REQ-PLAT-38014f08.

Expected files - new: `05_Software/nilo_gui/playwright.config.ts`, `e2e/topology.ts`,
`e2e/prerequisites.ts`, `e2e/bench-core.spec.ts`, `e2e/topology.test.ts`,
`e2e/prerequisites.test.ts`, `e2e/commands.test.ts`,
`02_Tools/bootstrap/bootstrap-browser.ps1`. Modified: the GUI `package.json`,
`package-lock.json`, `vite.config.ts`, `tsconfig.node.json`, `README.md`,
`02_Tools/env/with-tools.ps1`, `bootstrap/bootstrap-frontend.ps1`,
`bootstrap/bootstrap-development.ps1`, `02_Tools/README.md`,
`05_Software/scripts/aegis_test.py`, its two test suites, `.gitignore`,
`00_Planning/Development_Simulation_Vertical_Slices.md`,
`06_SoftwareDocu/Development_and_Simulation_Flow.md`, `CHANGELOG.md`, and the
`gui-foundation` and `repository-toolchain` knowledge pages.

## Rulings

- RULING-001 | date=2026-09-01 | decision=The frame checkpoint stop is suppressed for this task; the pre-delivery stop remains the only stop. | reason=The user's prompt states the scope, the exclusions, the constraints, the ten requirements, the validation set and the report contents, and instructs the task to run frame through validate without an intermediate approval stop and to halt before the commit. | source=user | supersedes=none
- RULING-002 | date=2026-09-01 | decision=This task is delivered on `master` rather than on a task branch, so the frame step's branch instruction is deliberately not followed. | reason=The user asked to work on the same branch, which is `master` here; the journal `branch:` header records `master` so later sessions still reconcile against the real checkout. | source=user | supersedes=none
- RULING-003 | date=2026-09-01 | decision=TASK-8a4ba5da's close-out journal edit is carried by this task's delivery commit and is presented as part of this task's staged files. | reason=`6_deliver.md` step 7 requires a later commit to record a close-out that could only be written after the delivery outcome succeeded, and that task's outcome - the human's commit and push of 0cb7732 - had already succeeded when this task opened. | source=repository | supersedes=none
- RULING-004 | date=2026-09-01 | decision=`02_Tools/scripts/nilo-dev.ps1` and `02_Tools/lib/DevLifecycle.ps1` are not modified by this task, and the end-to-end harness does not call them. | reason=The user requires that the manual entry point gain no supervision, metadata, stop or logs behavior, and that it not be used as the automated test supervisor; the harness therefore manages only its own short-lived test processes. | source=user | supersedes=none
- RULING-005 | date=2026-09-02 | decision=The harness exposes no accessor for the development token; it leaves the topology object only through `enterTokenInto`, which redacts the value out of anything the action throws. | reason=A browser action that fails after its locator has resolved repeats its argument in the error message, which reaches the console reporter and the error-context note whatever the trace, video and screenshot settings are; this was reproduced with a dummy marker across six failure modes. Containment is structural rather than dependent on the runner's internals. | source=user | supersedes=none
- RULING-006 | date=2026-09-02 | decision=A browser end-to-end run whose release did not fully succeed fails, and the release script confirms the directory is absent rather than trusting the removal command's exit status. | reason=The human's review established that a WSL failure does not mean the temporary files disappeared, and that a run leaving its database behind must not be reported as clean. REQ-REL-f0c8b07c was amended, not just re-tested, because its verification asked only that removal be attempted. | source=user | supersedes=none

## Handoff

current: deliver
status: waiting
next: Re-present the candidate at the delivery checkpoint after the two review findings were fixed and re-proved; commit nothing before approval.
must_read: .agentic/tasks/TASK-f9dbdeef.md, .agentic/steps/6_deliver.md, 05_Software/nilo_gui/e2e/topology.ts, 05_Software/nilo_gui/e2e/token-redaction.spec.ts
blockers: none

## Stage report

stage: validate
result: completed
current: deliver
changed: 42 files against 0cb7732, excluding the untracked `00_Planning/TODO.md` - 20 modified tracked and 22 new. New: 7 files under `05_Software/nilo_gui/e2e/` (the harness `topology.ts`, the prerequisite checks `prerequisites.ts`, the browser specs `bench-core.spec.ts` and `token-redaction.spec.ts`, and three Vitest suites), `05_Software/nilo_gui/playwright.config.ts`, `02_Tools/bootstrap/bootstrap-browser.ps1`, 10 requirement files, and this journal with its anchor and evolution records. Modified: the GUI `package.json`, `package-lock.json`, `vite.config.ts`, `tsconfig.node.json`, `.prettierignore` and `README.md`; `02_Tools/env/with-tools.ps1`, `bootstrap/bootstrap-frontend.ps1`, `bootstrap/bootstrap-development.ps1` and `README.md`; `05_Software/scripts/aegis_test.py` and its two test suites; `.gitignore`, `00_Planning/Development_Simulation_Vertical_Slices.md`, `06_SoftwareDocu/Development_and_Simulation_Flow.md`, `CHANGELOG.md`, both knowledge pages, and TASK-8a4ba5da's close-out
evidence: `npm run test:e2e` exit=0, 4 browser tests passed in 14.7s - two against a Bench Core the suite compiled and ran inside `Ubuntu-24.04`, two forced-failure redaction regressions; `npm run test:e2e:headed` exit=0; `npm run test:ci` exit=0 with 155 tests in 10 files, up from 113 in 7 at task start; `npm run build`, `format:check` and `lint` each exit=0; `go test ./...` exit=0 across 19 packages; `python -m unittest discover -s scripts/tests` exit=0 with 41 tests; `python scripts/aegis_test.py full` exit=0 and observed to run `npm.cmd run test:e2e` after `test:ci` and `build`; `.\02_Tools\env\with-tools.ps1 python .agentic/aegis.py check --task TASK-f9dbdeef --strict --run` exit=0 with `run: test ok` (59.5s), `run: lint ok` (4.0s), `strict: ok`, errors=0 warnings=1; `git diff --check` exit=0. After a passing run the WSL state root holds 0 entries, no `nilo serve` process remains, and no plaintext token exists anywhere in the tree
issues: three deviations stand - the design note's expected-file list omitted `.prettierignore`; the accepted `outside-layout` warning for `CHANGELOG.md`, whose honest fix is one line in `aegis.toml` outside this task's scope; and the 45-byte `test-results/.last-run.json` a passing Playwright run writes after it finishes, now stated plainly in the interface README rather than described as nothing. The human's review of the first presentation found two implementation flaws, both confirmed by measurement and both fixed: a failing browser action could print the token, and a failed cleanup could still pass. Two requirements were amended because they were too weak to have caught either. No unresolved defect and no failing verification
next: Re-present the candidate at the delivery checkpoint and wait for approval; commit nothing before it
needs_user: yes - the delivery checkpoint is the one remaining stop, and the user asked to stop before the commit
spec_compliance: disabled
implementation_quality: disabled
review_round:

## Delivery report (2026-09-02)

- stage: deliver; type: feature; path: full; branch: master
- requirements:
  - REQ-TEST-0a4f49af - Prove the authenticated browser-to-WSL2 simulation journey
  - REQ-TEST-0f02f4b8 - Prove backend-unavailable presentation and browser recovery
  - REQ-TEST-7ff6a75d - Run the browser end-to-end test in the repository gate
  - REQ-TEST-3c4487d6 - Name a missing browser end-to-end prerequisite
  - REQ-REL-f04744fa - Bound the browser end-to-end readiness wait
  - REQ-REL-f0c8b07c - Release only what a browser end-to-end run created
  - REQ-SEC-4884bc91 - Keep the end-to-end development token out of output and artifacts
  - REQ-PLAT-38014f08 - Keep the browser test runtime pinned and repository-local
  - REQ-PLAT-59993b78 - Isolate each browser end-to-end run from the developer topology
  - REQ-UX-f479d8b9 - Offer one headless browser test command and an opt-in headed mode
- deviations: 3 recorded
  - build - `python .agentic/aegis.py evolution --task TASK-f9dbdeef` reported 23 changed files, one of which, `05_Software/nilo_gui/.prettierignore`, the design note's expected-file list did not name. It was needed because Playwright's generated `test-results/` output would otherwise be reformatted by `format` and then fail `format:check`; the frame log did name the file, the design note dropped it.
  - validate - a successful run leaves one 45-byte file, `05_Software/nilo_gui/test-results/.last-run.json`, holding `{"status":"passed","failedTests":[]}`. Playwright writes it after the run has finished, so the suite cannot remove it, and chaining a cleanup onto the npm script would mask the command's exit code. It is ignored by Git and carries no trace, screenshot, video, token or database, but it is not literally nothing; it is reported rather than described as an empty directory.
  - build - `python .agentic/aegis.py check` exits 0 with errors=0 warnings=1, the `outside-layout` warning for `CHANGELOG.md`. All 19 `stale-paths` warnings from specify have cleared. The remaining one is accepted rather than silenced: `aegis.toml`'s `[layout] docs` globs list `README.md` but not `CHANGELOG.md`, the entry was explicitly requested by the user, and the honest fix is one line of configuration outside this task's scope. The previous task accepted the same warning for the same reason.
- explicit review: disabled; existing validation remains required
- diff vs task base 0cb7732: 43 file(s) changed, 4859 line(s) added
- checks: errors=0 warnings=2 notes=0
- checklist: 4/6 done; open: agreed outcome delivered — the human makes the commit themselves; this journal is staged with it; close-out committed after the outcome succeeded
