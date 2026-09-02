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

# TASK-8a4ba5da — Add one-command Windows development lifecycle

stage: done
type: feature
path: full
requirements: REQ-FUNC-d05e1128, REQ-FUNC-d161146d, REQ-FUNC-645b9d0c, REQ-UX-4f1d3604, REQ-UX-1f854bdc, REQ-UX-09865d40, REQ-SEC-e0462d41, REQ-REL-8c82d667, REQ-PLAT-5360be1a, REQ-PLAT-022e39b0, REQ-TEST-daa9a99d
parent:
branch: master
base: 89aba98b8f928d2545a8c49a0971f2aa5fab3e20
opened: 2026-09-01

## Done checklist (current stage)

- [x] validated candidate confirmed unchanged; otherwise returned to validate
- [x] validation evidence, diff, exclusions, and outcome presented
- [x] delivery confirmed by the human, who also ran the live topology themselves
- [x] journal and report included in the delivery commit
- [x] agreed outcome delivered — the human committed and pushed 0cb7732, which carries this journal
- [x] close-out written after the outcome succeeded; a later task's commit carries this journal-only edit

## Log

- 2026-09-01 frame: scope is vertical-slice 5 - one obvious PowerShell entry point under `02_Tools` that starts, stops, reports the status of, and shows the logs of the whole local development topology, with the Go Bench Core running under WSL2 on the simulation profile and the Vite GUI running on native Windows.
- 2026-09-01 frame: scope covers explicit options for the WSL2 distribution, the simulation scenario, the backend port and the frontend port, each with a safe default, plus an opt-in browser open that never happens by itself.
- 2026-09-01 frame: scope covers the state split the flow document already requires - Bench Core SQLite, artifacts and product runtime state stay on the WSL Linux filesystem and never under `/mnt/<drive>`, while lifecycle metadata and generated logs live in an ignored, clearly owned development-state location.
- 2026-09-01 frame: scope covers the safety behavior that makes this usable on a developer's own machine - detect port conflicts before launching, never kill a process merely because it owns a wanted port, track only processes this repository lifecycle launched, and validate PID, process identity, command line and instance ownership before stopping anything.
- 2026-09-01 frame: scope covers lifecycle semantics - `start` idempotent when the same instance is already healthy, readiness of both components detected before success is reported, partial startup cleaning up only what that invocation started while preserving its logs, `stop` idempotent and graceful before any narrowly scoped fallback, `status` distinguishing stopped, starting, healthy, partially running, stale metadata and port conflict, and `logs` naming the frontend and backend streams with a bounded recent view.
- 2026-09-01 frame: scope covers what `start` prints - backend status and endpoint, browser URL, selected distribution and scenario, and the exact token-issuance, status, logs and stop commands - with no plaintext bearer token ever persisted or logged.
- 2026-09-01 frame: out of scope, stated by the user - browser end-to-end infrastructure (slice 6), light/dark/system theme selection, MCP operations, hardware access or PLC flashing, Raspberry Pi deployment, packaging, systemd, firewall or WireGuard behavior, and any new product API endpoint or backend wire-format change.
- 2026-09-01 frame: full path chosen - the slice adds a new externally observable command surface with its own option set, exit-code contract and on-disk state format, none of which an existing requirement covers and none of which the quick path allows.
- 2026-09-01 frame: no child tasks - this is one tightly coupled PowerShell component under `02_Tools` plus its Python contract tests, too small and too interdependent to govern separately.
- 2026-09-01 frame: area is `02_Tools` (a new lifecycle entry point under `scripts/`, a new library under `lib/` beside `Wsl.ps1`, and `README.md`), with tests in `05_Software/scripts/tests/`, plus `.gitignore`, `06_SoftwareDocu/Development_and_Simulation_Flow.md`, `00_Planning/Development_Simulation_Vertical_Slices.md` and `.agentic/knowledge/repository-toolchain.md`.
- 2026-09-01 frame: read the existing toolchain before framing - `02_Tools/lib/Wsl.ps1` already centralises checked WSL invocation, distribution discovery, the WSL2 kernel test and `wslpath` translation, and `02_Tools/env/with-tools.ps1` already builds the Windows child-process environment for Go, uv, Python, Node.js and npm, so the lifecycle composes both rather than reimplementing either.
- 2026-09-01 frame: read the backend command surface before framing - `nilo serve --profile simulation [--scenario healthy|unavailable] --data-dir <absolute> --bind <loopback>` and `nilo development issue-token --data-dir <absolute>` are the executable forms in `05_Software/nilo_assistant/cmd/nilo/main.go`, and `serve` prints one `{"status":"listening",...}` JSON line when the listener is up.
- 2026-09-01 frame: read the test precedent before framing - `05_Software/scripts/tests/test_toolchain_bootstrap.py` already drives real PowerShell scripts with a fake `wsl.exe` injected through `-WslExecutable` and asserts on a log of the arguments it received, which is exactly the injected boundary this slice needs for deterministic failure paths.
- 2026-09-01 frame: user constraints recorded - stay on `master`, commit nothing until explicit delivery approval, leave the untracked `00_Planning/TODO.md` exactly as it is and outside this task, preserve existing Windows and WSL bootstrap behavior, and mark slice 5 implemented only when start, status, logs and stop work safely in the real Windows plus WSL2 topology.
- 2026-09-01 frame: before opening this task, reconciled TASK-8fd83f88 - the human had approved it and then committed and pushed 89aba98 manually, leaving that journal at `stage: deliver` with two unticked items; its delivered diff was verified path for path against the recorded proof, the 36-versus-35 file count was traced to `collect_diff` counting untracked `00_Planning/TODO.md`, and the journal was closed out to `stage: done` truthfully without rewriting or amending the commit.
- 2026-09-01 frame: that close-out is a journal-only edit that no commit yet carries, so it is deliberately part of this task's candidate and will be presented in this task's staged files at the delivery checkpoint.
- 2026-09-01 frame: expected requirements - one REQ-FUNC for the single Windows entry point and its four verbs, one REQ-FUNC for idempotent start with verified readiness of both components, one REQ-FUNC for the named log streams and their bounded recent view, one REQ-UX for the six distinguished status classifications, one REQ-UX for what start prints and for the browser opening only on request, one REQ-SEC for never persisting or logging a plaintext bearer token, one REQ-REL for launched-process ownership and validated stopping, one REQ-REL for pre-launch port-conflict detection that never terminates the owner, one REQ-REL for partial-startup cleanup that preserves diagnostics, one REQ-PLAT for WSL-native product runtime state, one REQ-PLAT for ignored development-state metadata and logs, one REQ-PLAT for the automation-usable exit codes, and one REQ-TEST for the injected process and WSL boundaries.
- 2026-09-01 frame: no existing requirement is expected to change - slice 5 adds a command surface beside the bootstrap and doctor entry points rather than altering them, and the user asked that existing bootstrap behavior be preserved.
- 2026-09-01 frame: stage advanced to specify.
- 2026-09-01 specify: created fifteen requirements rather than the thirteen framed, because two of the drafts carried two `shall` clauses each and `2_specify.md` step 6 requires one behavior per requirement.
- 2026-09-01 specify: split readiness from idempotence - REQ-FUNC-d161146d now covers only reporting success after both endpoints answer and timing out by name when one does not, and the new REQ-FUNC-645b9d0c covers leaving an already-healthy instance of the same checkout untouched.
- 2026-09-01 specify: split printing from browser launching - REQ-UX-1f854bdc now covers only what a successful start prints, and the new REQ-UX-09865d40 covers opening a browser only on the explicit option.
- 2026-09-01 specify: created REQ-FUNC-d05e1128 (one entry point with the four commands and the four explicit options), REQ-FUNC-6f0f254f (named component log streams with a bounded recent view), REQ-UX-4f1d3604 (the six status classifications), REQ-SEC-e0462d41 (no plaintext token in metadata, logs or output), REQ-REL-13d73a86 (four ownership checks before any signal), REQ-REL-8c82d667 (port conflict reported without terminating the owner), REQ-REL-dc3c44d5 (partial-start cleanup limited to what that invocation launched, logs preserved), REQ-PLAT-5360be1a (WSL-native data directory, never a Windows drive mount), REQ-PLAT-15cc50fd (all lifecycle state below one Git-ignored directory), REQ-PLAT-022e39b0 (distinct documented exit codes) and REQ-TEST-daa9a99d (five injectable boundaries so no test touches a real process).
- 2026-09-01 specify: no existing requirement was updated - the lifecycle adds a command surface beside the bootstrap and doctor entry points, and re-reading the toolchain knowledge page confirmed none of its eight behaviors describes starting, stopping, or supervising a running topology.
- 2026-09-01 specify: no spec written - each requirement is buildable from its own prose against command surfaces that already exist, namely `nilo serve`/`nilo development issue-token` in `05_Software/nilo_assistant/cmd/nilo/main.go` and the `npm run dev` script in `05_Software/nilo_gui/package.json`.
- 2026-09-01 specify: every one of the fifteen new files was confirmed to contain 0 CR bytes before advancing, because the CRLF defect TASK-8fd83f88 delivered came from a Python text-mode write of exactly these files.
- 2026-09-01 specify: `python .agentic/aegis.py check` reported errors=0 warnings=46 notes=0, every warning a `stale-paths` entry for `02_Tools/scripts/nilo-dev.ps1`, `02_Tools/lib/DevLifecycle.ps1` or `05_Software/scripts/tests/test_development_lifecycle.py`, none of which build has created yet; expected before implementation and not accepted deviations.
- 2026-09-01 specify: stage advanced to design.
- 2026-09-01 design: wrote the design note - one entry point over a decision library whose functions are pure enough to test, one `-BoundaryScript` carrying all four injected boundaries beside the existing `-WslExecutable`, and an instance identity derived from the Windows repository root so two checkouts never share state.
- 2026-09-01 design: chose to build the backend to `<datadir>/bin/nilo` and launch that binary rather than `go run ./cmd/nilo`, because `go run` executes an anonymous temporary binary whose command line proves nothing, and REQ-REL-13d73a86 requires the recorded identity and command line to still match before anything is signalled.
- 2026-09-01 design: chose the backend readiness probe to be a 401 from `/api/v1/status`, which proves Bench Core is listening and answering without the lifecycle ever obtaining or holding a token, keeping REQ-FUNC-d161146d and REQ-SEC-e0462d41 from pulling against each other.
- 2026-09-01 design: added exit code 7 for `starting` beyond the six outcomes REQ-PLAT-022e39b0 enumerates, because a caller that cannot tell "still coming up" from "partially running" would either fail a healthy start or wait on a broken one; the requirement's distinctness rule is satisfied, not weakened.
- 2026-09-01 design: revised the `## Architecture` diagram on `.agentic/knowledge/repository-toolchain.md` to the intended post-change shape and marked it as intended until validate.
- 2026-09-01 design: `affected_paths` on all fifteen requirements already name the three files the note expects to create, plus `.gitignore` on REQ-PLAT-15cc50fd and `02_Tools/README.md` on REQ-PLAT-022e39b0, so no correction was needed.
- 2026-09-01 design: `python .agentic/aegis.py check` reported errors=0 warnings=46 notes=0, all `stale-paths` for the three files build has yet to create.
- 2026-09-01 design: stage advanced to build.
- 2026-09-01 build red: command=`python -m unittest discover -s scripts/tests -p test_development_lifecycle.py -v` from `05_Software`; exit=1; 18 failed, 2 errors and 1 passed of 21; expected missing behavior=all fifteen requirements have no entry point, so every run reports `The argument '02_Tools\scripts\nilo-dev.ps1' to the -File parameter does not exist`, and the only test that passes is the one asserting the existing bootstrap scripts are still present and still do not start the topology.
- 2026-09-01 build: `python .agentic/aegis.py anchor --task TASK-8a4ba5da` recorded 201 files at 89aba98 after that red and before any implementation edit.
- 2026-09-01 build green: command=`python -m unittest discover -s scripts/tests -p test_development_lifecycle.py` from `05_Software`; exit=0; 21 tests passed in 162s after adding `02_Tools/lib/DevLifecycle.ps1`, `02_Tools/scripts/nilo-dev.ps1`, the `/02_Tools/state` ignore rule and the tools README lifecycle section.
- 2026-09-01 build: four PowerShell portability defects were found by the red-to-green loop rather than by inspection, and each would have failed on a developer's machine too, not only in tests.
- 2026-09-01 build: the first was constrained language mode - `[System.Security.Cryptography.SHA256]` cannot be constructed there, so the instance identity is now an arithmetic FNV-1a over the checkout path rather than a digest; the identity only has to be stable per checkout, not cryptographic.
- 2026-09-01 build: the second was that Windows PowerShell 5.1 parses the literal `0xFFFFFFFF` as the signed value -1, which silently turned the FNV mask into a no-op; the mask is now written in decimal with a comment saying why.
- 2026-09-01 build: the third was that `[uint32] * [int]` promotes to `[double]` in PowerShell, silently losing the low bits of every product above 2^53, so the Windows and Python hashes disagreed and every ownership check reported a foreign checkout; the operands are now cast to `[long]`. This one is worth naming because it fails silently and only at scale - the algorithm was correct and the arithmetic was not.
- 2026-09-01 build: the fourth was that `exit (Invoke-Start)` captures the function's whole output stream as its return value, so every `Write-Output` line was swallowed and the command printed nothing at all; user-facing text now goes to the host stream.
- 2026-09-01 build: two of my own test-harness defects were fixed rather than worked around - the boundary recording was appended across two runs in one workspace so the second `stop` appeared to signal what the first had, and the `assertNotRegex` guarding against real process calls matched its own pattern in the file it was scanning.
- 2026-09-01 build: the fake process boundary was changed to report a signalled process as gone on the next lookup, so the graceful path completes instead of always falling through to the bounded force fallback; this made the suite both faster and a truer model of a process that honours SIGTERM.
- 2026-09-01 build: caught and removed a UTF-8 BOM that a PowerShell `Set-Content -Encoding utf8` patch had added to the two new scripts and the new test file, and confirmed 0 CR bytes in all three; this is the same class of defect TASK-8fd83f88 delivered, so every write to a requirement file in this task used explicit byte writes rather than Python text mode.
- 2026-09-01 build: all fifteen requirements set to `status = "active"` with their `linked_tests` filled and `affected_paths` matching the design note, before running the focused set.
- 2026-09-01 build: DEVIATION build - `python .agentic/aegis.py evolution --task TASK-8a4ba5da` reported 7 changed files and one of them is `00_Planning/TODO.md`, which this task was told to leave exactly as it is. I did not write to it: its recorded anchor digest is 721980ff and it is now 6eb840c4, and its mtime is 2026-09-01T14:36:43Z while the anchor was taken at 2026-09-01T14:05:45Z, so it was changed by something outside this task while the task was running - most plausibly the human, who was editing during the session. The file is still untracked, is excluded from the candidate, and was neither read for content nor written by me; it is reported here rather than accepted silently, and the user is asked to confirm the change was theirs.
- 2026-09-01 build: the other six paths in the evolution report are exactly the ones the design note named - `.gitignore`, `02_Tools/README.md`, `06_SoftwareDocu/Development_and_Simulation_Flow.md`, and the three new files - with nothing added and nothing removed.
- 2026-09-01 build: `python .agentic/aegis.py check --task TASK-8a4ba5da` reported errors=0 warnings=2, both `journal-header` staleness in the two task journals rather than findings against the code; refreshed both Handoffs and reran rather than accepting them.
- 2026-09-01 build: stage advanced to validate.
- 2026-09-01 validate: reviewed the diff against the frame scope - `git diff --name-only 89aba98 -- 05_Software/nilo_assistant 05_Software/nilo_gui` returns nothing, so no product source, no API endpoint and no wire format changed; no dependency was added; no browser end-to-end infrastructure, theme selection, MCP, hardware, or Raspberry Pi behavior appears anywhere in the diff.
- 2026-09-01 validate: reviewed the preservation constraint - `02_Tools/bootstrap/*` and `02_Tools/scripts/doctor*.ps1` are untouched, `02_Tools/lib/Wsl.ps1` is untouched and reused rather than modified, and a test asserts the combined bootstrap still does not reference the lifecycle entry point so setup never starts a topology.
- 2026-09-01 validate: the live run found four defects the automated suite could not, because each lives in the real boundaries the tests deliberately replace; all four are fixed and re-proven, and they are recorded here rather than summarised away.
- 2026-09-01 validate: live defect 1 - `Start-Process` cannot redirect a child's standard output and standard error to the same file, which the first real start would have failed on; each component now has a separate `.err.log` and `logs` shows it only when it has content.
- 2026-09-01 validate: live defect 2 - the real boundary script blocks referenced `$wsl` and `$distribution` from the defining function's scope, but a PowerShell script block is not a closure, so the first real call failed with `The variable '$wsl' cannot be retrieved`; the values now live in script scope and are read when a boundary runs. `GetNewClosure` was rejected as the fix because it is unavailable under constrained language mode.
- 2026-09-01 validate: live defect 3 - the first real `stop` refused to stop either component, reporting that neither still matched its recorded identity. The cause was that the launcher recorded a synthetic command line for the frontend while the ownership check compares against what the process boundary observes. The fix records the identity by re-reading it through that same boundary after launch, so recorded and observed are produced by one code path rather than two. The safety property held correctly throughout - the lifecycle refused to signal rather than signalling the wrong thing - but a lifecycle that cannot stop what it started is still a defect.
- 2026-09-01 validate: live defect 4 - stopping the frontend by its tracked identifier orphaned the server, because `npm run dev` makes the real listener a grandchild; the observed tree was four processes deep. The Windows stop boundary now uses `taskkill /PID <id> /T`, which is still narrowly scoped because it is the tree of a process whose ownership was already verified. Its graceful form writes to standard error and exits non-zero when a console child will not stop, which under `$ErrorActionPreference = 'Stop'` aborted the whole command; the graceful attempt is now explicitly best-effort so the caller's own wait decides when to escalate.
- 2026-09-01 validate: also corrected a message that was untrue rather than merely unhelpful - absent metadata was reported as "the lifecycle metadata belongs to a different checkout", which would send a developer looking for another checkout; absent and foreign metadata are now separate statements.
- 2026-09-01 validate: MANUAL INTEGRATION EVIDENCE, real Windows plus WSL2 topology, Ubuntu-24.04 on kernel 6.18.33.2-microsoft-standard-WSL2 - `.\02_Tools\scripts\nilo-dev.ps1 start` exit=0 reported both components answering, backend pid 694 in WSL and frontend pid 38060 on Windows, and printed the endpoint, browser URL, distribution, scenario, data directory, the token-issuance command and the status, logs and stop commands.
- 2026-09-01 validate: manual observation of REQ-PLAT-5360be1a - the backend data directory was `/home/dpasare/.local/state/nilo-development/f62c4bb04ff0`, on the WSL Linux filesystem and not under `/mnt/`, and the recorded backend command line shows `--data-dir` pointing there.
- 2026-09-01 validate: manual observation of REQ-FUNC-645b9d0c - a second `start` against the healthy instance exited 0, reported "This checkout is already running and healthy; nothing was started", and left both process identifiers unchanged.
- 2026-09-01 validate: manual observation of the authenticated path - a token issued with the exact printed command returned `token_id`, and `GET /api/v1/status`, `/api/v1/integrations` and `/api/v1/devices` through the interface's own origin on port 5173 answered with `simulation: true`, integration `simulation.fake-python` state `healthy`, and device `simulation.fake-python.device.stable-001` availability `ready` owned by that integration.
- 2026-09-01 validate: manual observation in the real browser - the interface at `http://127.0.0.1:5173/` showed the login screen, then after connecting showed profile `simulation`, service status `healthy`, SIMULATION `Yes`, the integration inventory with one entry and its five operations, and the device inventory with one entry showing availability, owning integration, kind and capabilities, all against the WSL2 backend this lifecycle started.
- 2026-09-01 validate: manual observation of REQ-FUNC-6f0f254f - `logs -Tail 6` printed a heading naming each stream and its path and showed both, with the backend's `{"status":"listening","address":"127.0.0.1:8090","profile":"simulation"}` line; `logs -Component backend -Tail 3` showed only backend lines, and both logs were still readable after `stop`.
- 2026-09-01 validate: manual observation of REQ-REL-8c82d667, on a genuinely unrelated process rather than a contrived one - a Vite server belonging to the human's other project at `D:\2026_Agentic_Delivery_Platform\...\frontend` was holding port 5173, and `start` exited 3 with "windows port 5173 is already held by process 21244 (node) ... it has been left running and nothing was started"; the holder was confirmed alive and still holding the port afterwards, no metadata was written, and `ps -eo comm` confirmed no backend had been started in WSL. An earlier controlled repetition with a Python listener on 5173 behaved identically and that holder also survived.
- 2026-09-01 validate: manual observation of REQ-REL-13d73a86 and stop idempotence - `stop` exit=0 asked the frontend to shut down, escalated to the bounded force fallback when the console child did not exit, and stopped the backend gracefully on the first signal because the Go server honours SIGTERM; afterwards Windows pid 38060 and WSL pid 694 were both gone, no `nilo` process remained, and port 5173 was free. Running `stop` twice more exited 0 each time with "Nothing is recorded as running for this checkout".
- 2026-09-01 validate: DEVIATION validate - the live topology run is a manual method standing in for a test that could be automated; it is deliberately not automated here because the user excluded browser end-to-end infrastructure from this task and vertical-slice 6 owns it. The vertical-slice and development-flow documents both say this evidence is manual and is not the repository gate.
- 2026-09-01 validate: DEVIATION validate - the browser pane again set the token field's DOM value without React observing it, exactly as recorded in TASK-8fd83f88, so the login was completed by dispatching a native input event and a real DOM click; the interface behaved correctly the moment a genuine event reached it, so this is a harness artifact rather than a defect, and the observations were taken from the accessibility tree and page text rather than a screenshot.
- 2026-09-01 validate: a cosmetic observation, not fixed - the backend log carries some shell function text from `bash -lc` sourcing the login profile before the server's own JSON line. The login shell is what makes the repository tool wrapper resolve, so it is kept; the noise precedes the useful output rather than replacing it.
- 2026-09-01 validate: re-ran the automated suite after every one of the live fixes - `python -m unittest discover -s scripts/tests -p test_development_lifecycle.py` from `05_Software`; exit=0; 21 tests passed, so no live fix regressed an injected-boundary path.
- 2026-09-01 validate: stage advanced to deliver; the candidate was presented for approval.
- 2026-09-01 deliver: the user brought a code review with six findings, three P1 and three P2; every one was checked against the code and every one is valid, so per `6_deliver.md` step 1 the candidate must change and this returned to validate rather than being argued down.
- 2026-09-01 deliver: returned to validate; the recorded proof described a candidate that no longer stands.
- 2026-09-01 validate: confirmed finding 2 in the source before fixing - `Get-NiloDevState` counted `$ownership.Alive`, which is set the moment a process with that identifier exists and before the identity comparison, so a recycled identifier counted as running and `Running` was reported from `Alive` too; a live process that is no longer ours could therefore be reported healthy and could make `start` treat an unrelated process as an already-running instance.
- 2026-09-01 validate: confirmed finding 3 in the source before fixing - `Invoke-Stop` called `Remove-NiloDevMetadata` unconditionally after the component loop, so refusing to signal a mismatched process still deleted the only record of it, printed `state: stopped` and exited 0, leaving a process nothing could manage afterwards.
- 2026-09-01 validate: confirmed finding 1 in the source before fixing - `Undo-PartialStart` sent one graceful signal with no wait, no re-check and no bounded force, which for the Windows frontend is known not to stop a console child, and the WSL launcher threw on discovery timeout without ever returning the wrapper it had already started, so the caller had nothing to clean up.
- 2026-09-01 validate: this review found three real defects that the automated suite and the live run both missed, and the two ownership ones are the most serious in the task - they weaken exactly the safety property this slice exists to provide. The suite passed because its arrangements never had a live-but-unowned process; that gap is now covered by tests rather than by argument.
- 2026-09-01 validate: wrote nine new failing tests before fixing anything, one per finding plus the complements that pin the correct behavior - a live-but-unowned process is not running, start does not adopt a stranger, stop keeps metadata when it refuses, stop removes it only once everything is stopped, partial cleanup escalates, printed commands are translated and carry the selection, status reports the running instance's scenario, follow accepts one component with a non-empty error log, and unreadable metadata is stale.
- 2026-09-01 validate: fixed finding 2 - `Get-NiloDevState` now counts `Owned` rather than `Alive`, `Running` is reported from `Owned`, and a recorded identifier that is alive but no longer ours classifies as stale metadata instead of contributing to healthy.
- 2026-09-01 validate: fixed finding 3 - `Stop-OwnedComponent` returns `stopped`, `absent` or `retained`, and `Invoke-Stop` removes the metadata only when nothing was retained; a refusal or a failed stop keeps the record, reports `partially running`, names what is still running, and exits 5 instead of claiming success.
- 2026-09-01 validate: fixed finding 1 - the graceful-then-bounded-force sequence moved into one `Stop-TrackedProcess` used by both the stop command and the cleanup of a failed start, so partial cleanup now waits, re-checks and escalates instead of sending a single signal a console child ignores; and the WSL launcher no longer throws while leaving a wrapper behind, it stops the wrapper and asks anything matching the instance-unique binary path to terminate first.
- 2026-09-01 validate: fixed finding 4 - the token command's checkout path now comes from `ConvertTo-NiloWslPath` rather than a hardcoded `/mnt/f/...`, the printed status, logs and stop commands carry `-Distribution` and any non-default ports so they run exactly as printed, and `status` now reports the scenario, ports and distribution recorded for the running instance rather than the option defaults, which previously reported `scenario: healthy` for an instance started as `unavailable`.
- 2026-09-01 validate: fixed finding 5 - `-Follow` now requires one component rather than one file, and selects that component's primary stream, so a non-empty error log no longer makes `logs -Component backend -Follow` fail as ambiguous.
- 2026-09-01 validate: fixed finding 6 - `Read-NiloDevMetadata` returns an `unreadable` marker instead of `$null`, `status` classifies it as stale metadata, and both `start` and `stop` refuse and explain rather than discarding a record that may still describe a running instance.
- 2026-09-01 validate: a fifth live defect surfaced while re-verifying the fixes - the real Windows `GetProcess` read `StartTime` on a process caught mid-exit, which throws and aborted a stop after the force signal. Ownership never compares `StartTime`, so it is now read defensively and left blank on failure. The retention fix proved itself in the same run: the aborted stop kept its metadata and the next stop cleaned up the backend it had not reached.
- 2026-09-01 validate: also corrected two defects in my own fixture rather than the code - the stopped-process recording used a JSON round-trip that silently failed to match, which made every graceful stop look like it needed the force fallback and inflated the suite from 88s to 324s, and the portability assertion could not distinguish translation from hardcoding because the stand-in returned the real checkout path; it now returns `/mnt/z/elsewhere/nilo-checkout`, which no clone would have.
- 2026-09-01 validate: command=`python -m unittest discover -s scripts/tests -p test_development_lifecycle.py` from `05_Software`; exit=0; 30 tests passed in 88s, up from 21.
- 2026-09-01 validate: MANUAL INTEGRATION EVIDENCE after the fixes, on alternate ports so the human's unrelated Vite server was left undisturbed - `start -Scenario unavailable -BackendPort 8095 -FrontendPort 5188` exit=0 with backend pid 700 and frontend pid 28452; the printed token command carried the translated checkout path and the three follow-up commands each carried `-Distribution Ubuntu-24.04` and both ports; `status` run with no options at all reported `scenario: unavailable`, `backend port: 8095` and `frontend port: 5188` from the metadata rather than the defaults.
- 2026-09-01 validate: manual observation of the repaired stop - a later cycle on the same ports stopped both components, the frontend through the bounded force fallback and the backend gracefully, left no `nilo` process, freed port 5188 and removed the metadata; exit 0.
- 2026-09-01 validate: command=`.\02_Tools\env\with-tools.ps1 python .agentic/aegis.py check --task TASK-8a4ba5da --strict --run`; exit=0; `run: test ok` (102.1s), `run: lint ok` (3.8s), `strict: ok`, errors=0 - the full configured suite and lint set pass against the corrected candidate.
- 2026-09-01 deliver: a second user code review brought six more findings, five P1; every one was checked against the code and every one is valid, so the candidate returned to validate a second time.
- 2026-09-01 validate: the first finding was the most serious and was not about the code at all - the index had been staged at an earlier point by something outside this task, and five files had been edited since, so committing that index would have delivered the pre-review implementation and silently dropped every fix from the first review round. `git status` showed them as `AM`. The index has been reset so nothing stale can be committed, and nothing will be staged again until delivery is approved.
- 2026-09-01 validate: fixed the process-identity gap - `Stop-TrackedProcess` now calls a new `Test-NiloDevProcessIdentity` immediately before the graceful signal and again immediately before the force signal, so an identifier released during the fifteen-second wait and reissued by the operating system is never force-killed; both callers pass the expected executable and command line, including the partial-start cleanup, which previously passed none at all.
- 2026-09-01 validate: the real Windows launcher was still returning the synthetic command line it had been handed, which the new identity check would have rejected; it now reports what `Win32_Process` observes, so the launch record, the metadata and the ownership comparison are all the same string.
- 2026-09-01 validate: fixed the checkout-wide discovery - the launch marker is now a pattern that pins the bind address as well as the instance-unique binary path, so two backends from one checkout on different ports are distinguishable; discovery refuses to adopt anything when more than one process matches, and the timeout path no longer uses a pattern-wide kill - it signals only an unambiguous single match, by its own identifier.
- 2026-09-01 validate: fixed the distribution split - `Sync-BoundariesWithRecordedInstance` rebuilds the boundaries against the distribution recorded in the metadata before `start`, `stop` or `status` inspects anything, so `stop -Distribution B` against an instance recorded in A no longer reports the backend absent, deletes the record and orphans it. The injected boundary override is re-applied when the boundaries are rebuilt, so the test seam survives the switch.
- 2026-09-01 validate: fixed the empty-metadata hole - a zero-byte or whitespace-only file is now `unreadable` rather than absent, and the metadata write is atomic through a pending sibling and a move, so an interrupted write cannot produce the empty file that caused it.
- 2026-09-01 validate: fixed the quoting - the distribution, the translated checkout path and the data directory are now quoted for their respective shells, so a distribution named `Ubuntu Dev` or a checkout under a path with spaces still prints commands that run exactly as printed.
- 2026-09-01 validate: added nine more tests for this round, including a fixture that models an identifier being answered by a different process once it has been signalled, which is what reuse looks like in life; two of my own assertions had to be corrected because they matched their own needles in the file they were scanning.
- 2026-09-01 validate: command=`python -m unittest discover -s scripts/tests -p test_development_lifecycle.py` from `05_Software`; exit=0; 38 tests passed in 135s, up from 30.
- 2026-09-01 validate: MANUAL INTEGRATION EVIDENCE for this round - an empty `instance.json` made `start` exit 6 with `state: stale metadata` and launch nothing; a real start printed `-d 'Ubuntu-24.04'`, `--cd '/mnt/f/...'` and `-Distribution 'Ubuntu-24.04'` all quoted; and `stop -Distribution Ubuntu-24.04-WRONG` reported "recorded in distribution 'Ubuntu-24.04' ... using the recorded one" and stopped both components, where the previous build would have orphaned the backend. Afterwards no `nilo` process remained, port 5188 was free and the metadata was removed.
- 2026-09-01 validate: command=`.\02_Tools\env\with-tools.ps1 python .agentic/aegis.py check --task TASK-8a4ba5da --strict --run`; exit=0; `run: test ok` (156.4s), `run: lint ok` (5.7s), `strict: ok`, errors=0 warnings=1, the single warning being the expected `unticked-deliver`.
- 2026-09-01 deliver: a third review round, run as an eight-angle recall review, confirmed ten more defects in the supervised design, five of them ending with a real process running and nothing recording it; presented with the cost, the human chose to drop process supervision entirely rather than keep repairing it, which is RULING-006.
- 2026-09-01 validate: returned to validate for the third time and rebuilt the component to the simplified design; every claim recorded above about `stop`, `logs`, ownership metadata, PID validation, cleanup, six lifecycle states and the 38-test suite describes code that no longer exists, and is kept only as the history of how this arrived here.
- 2026-09-01 validate: `02_Tools/lib/DevLifecycle.ps1` fell from 780 to about 330 lines and `02_Tools/scripts/nilo-dev.ps1` from 750 to about 300; the supervision machinery, the metadata format, the state directory and its `.gitignore` rule are gone, and `.gitignore` is back to its committed content because the command now writes nothing.
- 2026-09-01 validate: deleted four requirements outright rather than retiring them, per RULING-007, and rewrote the eleven that survive so each specifies only the `start`/`status` design; the journal `requirements:` line now lists exactly those eleven.
- 2026-09-01 validate: the human reviewed the first cut of the simplified design and found six further defects, all valid and all fixed - `Start-Backend` created `<datadir>/bin/nilo` as a directory before `go build` tried to write the executable there, `-OpenBrowser` did nothing when the topology was already answering, one test sat out the full 180-second default while three others arranged an already-healthy topology and then expected launches, the requirements and documents still described the removed design, and the leftover `02_Tools/state/` directory, a missing CHANGELOG entry and a README timeout of 90 seconds against an implementation of 180 were all still outstanding.
- 2026-09-01 validate: added a `_plan_becoming_ready` arrangement so every launch test begins with both endpoints unavailable and then transitions to ready, and made the harness supply a five-second readiness bound whenever a test does not set its own, so no test can ever wait out the production default.
- 2026-09-01 validate: corrected one assertion rather than the code - the data-directory test forbade `/mnt/` anywhere in the launch arguments, but the checkout itself legitimately lives on the Windows mount; it now reads the `--data-dir` argument specifically.
- 2026-09-01 validate: DEVIATION validate - while rewriting the tools README I ran a PowerShell script whose failure path wrote an empty array over the file, truncating `02_Tools/README.md` to zero bytes. It was a tracked file, so `git checkout --` restored it from 89aba98 and the section was re-added with an ordinary edit. Nothing was lost, but a generated-write step destroyed a committed file and only version control made that recoverable; shell-quoted heredocs were abandoned for this kind of edit as a result.
- 2026-09-01 validate: command=`python -m unittest discover -s scripts/tests -p test_development_lifecycle.py` from `05_Software`; exit=0; 24 tests passed in 29.4s, down from 38 tests in 135s.
- 2026-09-01 validate: removed the obsolete generated directory `02_Tools/state/` after confirming its exact path; nothing outside it was touched.
- 2026-09-01 validate: reconciled every artifact with RULING-006 - the tools README lifecycle and exit-code sections, section 8.0 of the development-flow document, the vertical-slice entry, the toolchain knowledge page and its architecture diagram, and a new CHANGELOG `Unreleased` entry; every description of `stop`, `logs`, ownership metadata, PID validation, signalling, cleanup, six lifecycle states, the development-state directory and the 38-test result is gone, and the documented readiness timeout now matches the implemented 180 seconds.
- 2026-09-01 validate: command=`git diff --check`; exit=0; no whitespace or line-ending damage in the candidate.
- 2026-09-01 validate: `python .agentic/aegis.py evolution --task TASK-8a4ba5da` refreshed to 8 changed files (2 added, 6 modified, 0 removed); every path is one the rewritten design names, except `00_Planning/TODO.md`, which this task has never written.
- 2026-09-01 validate: command=`.\02_Tools\env\with-tools.ps1 python .agentic/aegis.py check --task TASK-8a4ba5da --strict --run`; exit=0; `run: test ok` (42.4s), `run: lint ok` (5.8s), `strict: ok`, errors=0 warnings=1 - the full configured suite and lint set pass, and the suite time fell from 156s to 42s because the lifecycle tests fell from 135s to 29s.
- 2026-09-01 validate: DEVIATION validate - the one remaining `check` warning is `outside-layout` for `CHANGELOG.md`, because `aegis.toml`'s `[layout] docs` globs list `README.md` but not `CHANGELOG.md`. The file is a legitimate repository document and the entry was explicitly requested, so the warning is accepted rather than silenced; the honest fix is one line in `aegis.toml`, which is a configuration change outside this task's scope and is offered to the user rather than taken.
- 2026-09-01 validate: NOT VERIFIED - no live Windows plus WSL2 run of the simplified command has been performed. Every live observation recorded earlier in this journal belongs to the removed supervised design and does not carry over, so this candidate's real-topology behavior is unproven and the vertical-slice entry deliberately no longer claims a met exit gate.
- 2026-09-01 build: the human's fourth review found two functional defects; both were reproduced by measurement before anything was changed.
- 2026-09-01 build: P1 confirmed - `Start-Process -ArgumentList` joins an array with spaces and quotes nothing, so passing three elements including `mkdir -p a && cd b` delivered nine arguments to the child; the backend's `bash -lc` script was therefore truncated to `mkdir` and Bench Core could never have started. I had refuted this claim in an earlier round on the strength of a live run of the supervised design; that refutation was wrong and is withdrawn.
- 2026-09-01 build: fixed P1 by adding `ConvertTo-NiloWindowsArgument` and `ConvertTo-NiloWindowsCommandLine`, which apply the quoting rules `CommandLineToArgvW` reverses, and by having the real launcher pass one quoted command line instead of the array; a round trip through a real child now returns all six arguments intact, including `mkdir -p a && cd b`.
- 2026-09-01 build: P2 confirmed - the WSL port-owner parser read `$Matches[1]` after a second `-match` had overwritten it, so a realistic `ss` line raised `Cannot convert value "nilo" to type "System.Int32"` instead of reporting the conflict; fixed by extracting `Get-NiloDevListener`, which reads the identifier before matching the name, and which is now unit-testable without a distribution.
- 2026-09-01 build: fixed the already-answering `start` path, which printed no token command; the data directory is now resolved before that short circuit, so every successful start prints the token-issuance command as REQ-UX-1f854bdc requires. The port-conflict check stays ahead of the home probe, because a blocked port means there is nothing to start.
- 2026-09-01 validate: added three tests - the launcher argument round trip, the listening-socket parse, and the token command on the already-answering path - and confirmed the first two fail against the previous code (nine arguments instead of six; an integer-conversion error instead of `716|nilo`), so they discriminate rather than merely pass.
- 2026-09-01 validate: DEVIATION validate - REQ-TEST-daa9a99d claimed the suite starts no process, which the new argument-boundary test makes false. The requirement and its verification were amended to permit exactly one hidden, short-lived helper that prints its arguments and exits, rather than leaving a requirement the suite contradicts; the no-signalling property is unchanged and still asserted.
- 2026-09-01 validate: reconciled the four items the human raised - the development-flow document no longer says the entry point stops the topology, the CHANGELOG entry is cut to the repository's one-or-two-line shape, REQ-UX-1f854bdc and REQ-REL-8c82d667 record their new verifications and linked tests, and this journal's header and Stage report now agree.
- 2026-09-01 validate: command=`python -m unittest discover -s scripts/tests -p test_development_lifecycle.py` from `05_Software`; exit=0; 27 tests passed in 36.3s.
- 2026-09-01 validate: command=`.\02_Tools\env\with-tools.ps1 python .agentic/aegis.py check --task TASK-8a4ba5da --strict --run`; exit=0; `run: test ok` (44.1s), `run: lint ok` (4.0s), `strict: ok`, errors=0 warnings=1 - the same accepted `outside-layout` warning for `CHANGELOG.md`, unchanged.
- 2026-09-01 validate: command=`git diff --check`; exit=0; index empty, HEAD still 89aba98, and `00_Planning/TODO.md` still untracked and outside this task.
- 2026-09-01 validate: corrected the candidate's file count - it is 23 files (6 modified tracked plus 17 new), not the 21 reported to the human in the previous round; the earlier figure under-counted the new files and the set itself never changed.
- 2026-09-01 deliver: the human ran the command against the real Windows plus WSL2 topology and reported it tested; they did not relay their specific observations, so this line records their verdict and not a measurement of my own, and the automated evidence above remains the only evidence I observed directly.
- 2026-09-01 deliver: the human approved delivery and chose to make the commit themselves; the candidate was staged for them unchanged, with `00_Planning/TODO.md` deliberately left unstaged and untracked as required throughout this task.
- 2026-09-01 deliver: the delivery carries TASK-8fd83f88's close-out edit under RULING-003; this journal's own close-out cannot be carried by the same commit, because the outcome - the human's commit and any push that follows - is not observable to me beforehand, so `stage: deliver` stands until it succeeds.
- 2026-09-01 deliver: the outcome succeeded - commit 0cb7732 carries this journal and the whole approved candidate, and `git rev-list --left-right --count origin/master...master` reports 0 0, so it is also pushed; this task is closed out to `stage: done` as a journal-only edit, which a later task's delivery commit will carry, exactly as RULING-003 handled TASK-8fd83f88.

## Design

Superseded once, by RULING-006. The supervised design this section first
described - process ownership, lifecycle metadata, `stop` and `logs` - was
removed in full. What follows is the design that is actually built.

Component: `02_Tools` only. No product source changes; the backend and the
interface are launched through the command surfaces they already publish.

Interface - one entry point, `02_Tools/scripts/nilo-dev.ps1 <start|status>`,
over a small library `02_Tools/lib/DevLifecycle.ps1` (REQ-FUNC-d05e1128):

- options `-Distribution` (default: the sole installed distribution),
  `-Scenario` (`healthy`), `-BackendPort` (8090), `-FrontendPort` (5173),
  `-OpenBrowser` (REQ-UX-09865d40) and `-ReadyTimeoutSeconds` (180, because a
  first start compiles the Go backend);
- injected boundaries (REQ-TEST-daa9a99d): the existing `-WslExecutable`, plus
  one `-BoundaryScript` that is dot-sourced and returns a hashtable overriding
  any subset of the endpoint, port, launcher and browser boundaries;
- exit codes (REQ-PLAT-022e39b0): 0 ok/healthy, 1 unexpected, 2 usage,
  3 port conflict, 4 stopped, 5 partially running.

The central decision: **the command supervises nothing.** Each component is
launched into its own visible console window and the developer stops it by
closing that window or pressing Ctrl+C. There is no process tracking, no
ownership metadata, no state directory, no `stop` and no `logs`. Owning
long-lived processes across Windows and a WSL2 distribution means identifier
reuse, process trees and two namespaces, and that machinery produced every
defect this component had across three review rounds. A command that terminates
nothing cannot terminate the wrong thing, and a window a developer can see is a
better log than a file they have to be told about.

State: none in the repository. Product runtime state is unaffected - the backend
is given `$HOME/.local/state/nilo-development/<instance>` resolved inside the
distribution, and a home directory resolving under `/mnt/` is refused rather
than used (REQ-PLAT-5360be1a). `<instance>` is a short hash of the Windows
checkout path, so two checkouts never share a database. Only the last non-empty
line of the home probe is taken, because the WSL helper merges standard error
into its output.

Health is what answers, not what exists (REQ-UX-4f1d3604). `status` probes both
endpoints and reports healthy, partially running, stopped, or port conflict.
Bench Core is recognised by a 401 to an unauthenticated status request, which
proves it is Bench Core without the command ever holding a token
(REQ-SEC-e0462d41). A port that is held while its component does not answer is
reported with its holder and nothing is started (REQ-REL-8c82d667); since the
command has no signalling code at all, leaving that process alone is structural
rather than a rule to remember. `start` launches only what is not already
answering, so a rerun is harmless and a half-up topology is completed rather
than duplicated (REQ-FUNC-645b9d0c), and it reports success only once both
endpoints answer (REQ-FUNC-d161146d).

Every path interpolated into the WSL script and every value printed in a
follow-up command is quoted for its shell (REQ-UX-1f854bdc), so a checkout or
distribution containing a space or an apostrophe still produces runnable text.
There are three quoters, not one, because there are three consumers: a POSIX
shell, PowerShell, and the Windows command line. The third exists because
`Start-Process -ArgumentList` joins an array without quoting it, which split
the backend's `bash -lc` script into separate arguments; the launcher builds
one correctly quoted command line instead of handing over the array.

Rejected alternative: keeping a supervised `stop`, fixed rather than removed.
Rejected because three review rounds found twelve defects in it, eight of them
capable of signalling a process the command did not own, and the convenience it
bought over closing a window did not justify that risk in a development-only
tool.

Test seams - one Python suite,
`05_Software/scripts/tests/test_development_lifecycle.py`, driving the real
entry point with a stand-in WSL launcher and a boundary script written into a
temporary directory. Every test supplies a bounded readiness deadline, so none
can wait out the production default, and the suite asserts that neither it nor
the command contains any process-signalling call.

Expected files - new: `02_Tools/scripts/nilo-dev.ps1`,
`02_Tools/lib/DevLifecycle.ps1`,
`05_Software/scripts/tests/test_development_lifecycle.py`. Modified:
`02_Tools/README.md`, `06_SoftwareDocu/Development_and_Simulation_Flow.md`,
`00_Planning/Development_Simulation_Vertical_Slices.md`,
`.agentic/knowledge/repository-toolchain.md`, `CHANGELOG.md`. `.gitignore` is
deliberately **not** changed: the simplified command writes nothing.

## Rulings

- RULING-001 | date=2026-09-01 | decision=The frame checkpoint stop is suppressed for this task; the pre-delivery stop remains the only stop. | reason=The user's prompt states the scope, the exclusions, the constraints, the test obligations, the live-validation obligations and the delivery presentation in full, and names approval before committing as the only checkpoint it wants. | source=user | supersedes=none
- RULING-002 | date=2026-09-01 | decision=This task is delivered on `master` rather than on a task branch, so the frame step's branch instruction is deliberately not followed. | reason=The user asked to work on the same branch, which is where TASK-8fd83f88 was delivered. | source=user | supersedes=none
- RULING-003 | date=2026-09-01 | decision=TASK-8fd83f88's close-out journal edit is carried by this task's delivery commit and is presented as part of this task's staged files. | reason=`6_deliver.md` step 7 requires a later commit to close out a task delivered by a push, that push has already succeeded, and the user asked for any required close-out commit to appear transparently in this task's proposed delivery. | source=user | supersedes=none
- RULING-004 | date=2026-09-01 | decision=The lifecycle prints the exact `nilo development issue-token` command for the developer to run; it never issues, captures, stores or echoes a token itself. | reason=The user required that no plaintext bearer token is ever persisted or logged, and printing the command satisfies the usability goal without the lifecycle ever holding a credential. | source=user | supersedes=none
- RULING-005 | date=2026-09-01 | decision=A live run of the real Windows plus WSL2 topology through the new entry point is recorded at validate as manual integration evidence, explicitly labelled as not the automated browser end-to-end gate that vertical-slice 6 still owes. | reason=The user asked for the live check and for that separation to be stated; this continues the separation TASK-8fd83f88 established. | source=user | supersedes=none
- RULING-007 | date=2026-09-01 | decision=Four requirements written for the supervised design are deleted rather than retired: REQ-FUNC-6f0f254f (component log streams), REQ-REL-13d73a86 (ownership checks before signalling), REQ-REL-dc3c44d5 (partial-start cleanup) and REQ-PLAT-15cc50fd (ignored development-state directory). | reason=Retirement records a behavior that shipped and was then withdrawn; none of these ever left this task's working tree, and the behaviors they describe no longer exist anywhere in the design. Keeping them as retired files would imply the product once had them. | source=technical | supersedes=none
- RULING-006 | date=2026-09-01 | decision=The lifecycle drops process supervision entirely. It no longer tracks process identifiers, keeps ownership metadata, or terminates anything. Each component is launched into its own visible console window, the developer stops it by closing that window or pressing Ctrl+C, `status` is an endpoint probe rather than a process query, and the `stop` and `logs` commands are removed along with the development-state directory that existed to serve them. | reason=The supervision machinery was the entire source of cost and risk: roughly 2,700 lines to replace three commands, and every one of the twelve defects found across three review rounds lived in the ownership, signalling, cleanup or metadata paths rather than in starting the topology. The convenience the slice exists to provide survives without any of it, and a command that terminates nothing cannot terminate the wrong thing. The user chose this after being shown the cost, the alternatives and the recommendation. | source=user | supersedes=none

## Handoff

<!-- Replace these fields with the latest minimal resume snapshot.
status: ready | waiting | blocked
-->

current: done
status: ready
next: Nothing. The task is closed; its close-out edit rides in the next task's delivery commit.
must_read: .agentic/tasks/TASK-8a4ba5da.md
blockers: none

## Stage report

stage: deliver
result: completed
current: done
changed: 23 files against 89aba98 (6 modified tracked, 17 new; an earlier snapshot said 21 and under-counted the new files) - 2 new PowerShell files (`02_Tools/scripts/nilo-dev.ps1` about 310 lines, `02_Tools/lib/DevLifecycle.ps1` about 380), 1 new Python suite, 11 new requirement files, this journal with its anchor and evolution records, TASK-8fd83f88's close-out edit, and the tools README, development-flow, vertical-slice, knowledge-page and CHANGELOG updates; `.gitignore` is unchanged because the command writes nothing, and `00_Planning/TODO.md` stays untracked and excluded
evidence: `python -m unittest discover -s scripts/tests -p test_development_lifecycle.py` from `05_Software` exit=0 with 27 tests in 36.3s; `.\02_Tools\env\with-tools.ps1 python .agentic/aegis.py check --task TASK-8a4ba5da --strict --run` exit=0 with `run: test ok` (44.1s), `run: lint ok` (4.0s), `strict: ok`, errors=0 warnings=1; `git diff --check` exit=0. The two defects fixed this round were each reproduced by measurement first and their tests confirmed to fail against the previous code. No live Windows plus WSL2 run of the simplified command has been performed - the live evidence recorded earlier in this journal belongs to the removed supervised design and does not carry over
issues: three review rounds found twenty-two valid findings in the supervised design, after which RULING-006 removed process supervision entirely rather than continuing to repair it; the human's review of the first simplified cut found six more and the fourth review found two functional defects, both of which would have stopped a real start, all fixed; two deviations stand - the accepted `outside-layout` warning for `CHANGELOG.md`, and REQ-TEST-daa9a99d amended because the new argument-boundary test starts one hidden helper process; an earlier round of mine wrongly refuted the argument-quoting defect, which is now measured, recorded and withdrawn; `evolution` still reports `00_Planning/TODO.md` as modified by something outside this task, which is reported rather than accepted and the file remains untracked and excluded
next: Nothing is owed. Commit 0cb7732 delivered and pushed the approved candidate; this journal is closed out and its close-out edit is carried by the next task's delivery commit
needs_user: no - the delivery outcome succeeded and was observed in Git, so the close-out needed no further decision
spec_compliance: disabled
implementation_quality: disabled
review_round:

## Delivery report (2026-09-01)

- stage: deliver; type: feature; path: full; branch: master
- requirements:
  - REQ-FUNC-d05e1128 - Drive the local development topology from one Windows entry point
  - REQ-FUNC-d161146d - Start the development topology idempotently once both components are ready
  - REQ-FUNC-645b9d0c - Leave an already-healthy development instance untouched on start
  - REQ-UX-4f1d3604 - Classify the development topology lifecycle state
  - REQ-UX-1f854bdc - Print the development endpoints and follow-up commands after start
  - REQ-UX-09865d40 - Open a browser only when explicitly requested
  - REQ-SEC-e0462d41 - Keep the development bearer token out of lifecycle state and logs
  - REQ-REL-8c82d667 - Detect a development port conflict without terminating its owner
  - REQ-PLAT-5360be1a - Keep Bench Core runtime state on the WSL Linux filesystem
  - REQ-PLAT-022e39b0 - Report development lifecycle outcomes through distinct exit codes
  - REQ-TEST-daa9a99d - Exercise the development lifecycle through injected process boundaries
- deviations: 6 recorded
  - build - `python .agentic/aegis.py evolution --task TASK-8a4ba5da` reported 7 changed files and one of them is `00_Planning/TODO.md`, which this task was told to leave exactly as it is. I did not write to it: its recorded anchor digest is 721980ff and it is now 6eb840c4, and its mtime is 2026-09-01T14:36:43Z while the anchor was taken at 2026-09-01T14:05:45Z, so it was changed by something outside this task while the task was running - most plausibly the human, who was editing during the session. The file is still untracked, is excluded from the candidate, and was neither read for content nor written by me; it is reported here rather than accepted silently, and the user is asked to confirm the change was theirs.
  - validate - the live topology run is a manual method standing in for a test that could be automated; it is deliberately not automated here because the user excluded browser end-to-end infrastructure from this task and vertical-slice 6 owns it. The vertical-slice and development-flow documents both say this evidence is manual and is not the repository gate.
  - validate - the browser pane again set the token field's DOM value without React observing it, exactly as recorded in TASK-8fd83f88, so the login was completed by dispatching a native input event and a real DOM click; the interface behaved correctly the moment a genuine event reached it, so this is a harness artifact rather than a defect, and the observations were taken from the accessibility tree and page text rather than a screenshot.
  - validate - while rewriting the tools README I ran a PowerShell script whose failure path wrote an empty array over the file, truncating `02_Tools/README.md` to zero bytes. It was a tracked file, so `git checkout --` restored it from 89aba98 and the section was re-added with an ordinary edit. Nothing was lost, but a generated-write step destroyed a committed file and only version control made that recoverable; shell-quoted heredocs were abandoned for this kind of edit as a result.
  - validate - the one remaining `check` warning is `outside-layout` for `CHANGELOG.md`, because `aegis.toml`'s `[layout] docs` globs list `README.md` but not `CHANGELOG.md`. The file is a legitimate repository document and the entry was explicitly requested, so the warning is accepted rather than silenced; the honest fix is one line in `aegis.toml`, which is a configuration change outside this task's scope and is offered to the user rather than taken.
  - validate - REQ-TEST-daa9a99d claimed the suite starts no process, which the new argument-boundary test makes false. The requirement and its verification were amended to permit exactly one hidden, short-lived helper that prints its arguments and exits, rather than leaving a requirement the suite contradicts; the no-signalling property is unchanged and still asserted.
- explicit review: disabled; existing validation remains required
- diff vs task base 89aba98: 24 file(s) changed, 3907 line(s) added
- checks: errors=0 warnings=2 notes=0
- checklist: 4/6 done; open: agreed outcome delivered — the human makes the commit themselves; this journal is staged with it; close-out committed after the outcome succeeded
