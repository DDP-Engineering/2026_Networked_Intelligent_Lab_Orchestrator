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

# TASK-8fd83f88 — Add GUI integration and device inventory

stage: done
type: feature
path: full
requirements: REQ-FUNC-539355e0, REQ-FUNC-72bffd89, REQ-UX-8fa0c15e, REQ-UX-afb79ae0, REQ-UX-8169ea1d, REQ-FUNC-3d412917, REQ-SEC-ed66f6ed, REQ-UX-0018c647, REQ-UX-66636ac9, REQ-UX-188a4860, REQ-PLAT-b55586c0, REQ-PLAT-f852c5b2, REQ-TEST-76b94249
parent:
branch: master
base: 2cf1a2ae9e070d0e8d373c86f5a39ab8724b264f
opened: 2026-09-01

## Done checklist (current stage)

- [x] validated candidate confirmed unchanged; otherwise returned to validate
- [x] validation evidence, diff, exclusions, and outcome presented
- [x] delivery confirmed by the human
- [x] journal and report included in the delivery commit
- [x] agreed outcome delivered, normally a PR
- [x] close-out committed after the outcome succeeded, or carried by the delivery commit where nothing could fail after it

## Log

- 2026-09-01 frame: scope is the rest of vertical-slice 4 - the authenticated integration and device inventory - extending the shared `nilo.http/1` contract with `GET /api/v1/integrations` and `GET /api/v1/devices`, adding strict runtime parsers and typed client methods for both, routing the development mock by API path, and rendering the integrations and devices the backend reports alongside the status already shown, on the same in-memory bearer token.
- 2026-09-01 frame: scope also covers the inventory display states - healthy inventory, the backend's deterministic `unavailable` inventory, a legitimately empty inventory, and a malformed integration or device inventory told apart from each other and from a rejected token, an unreachable backend and a malformed status - plus retry and token-reset actions, the persistent SIMULATION identity, and a single-viewport layout whose inventory regions scroll internally rather than scrolling the page.
- 2026-09-01 frame: out of scope, stated by the user - no change to the Go backend or its wire format, no browser end-to-end infrastructure (that is slice 6), no one-command lifecycle scripts (slice 5), no light/dark/system theme selection, and no new persistent credential store of any kind.
- 2026-09-01 frame: full path chosen - the slice adds new externally observable GUI behavior, two new client methods and two new parsed response contracts, and new mock scenarios, none of which the quick path allows.
- 2026-09-01 frame: no child tasks - this is one tightly coupled frontend slice inside `05_Software/nilo_gui`, the same area TASK-524e8dde delivered.
- 2026-09-01 frame: area is `05_Software/nilo_gui` (`src/api/contract.ts`, `src/api/client.ts`, `src/api/mock.ts`, `src/config/environment.ts`, `src/App.tsx`, `src/components/`, `src/App.css`, and their tests), with documentation in `05_Software/nilo_gui/README.md`, `06_SoftwareDocu/Development_and_Simulation_Flow.md`, `00_Planning/Development_Simulation_Vertical_Slices.md` and `.agentic/knowledge/gui-foundation.md`; `05_Software/nilo_assistant/internal/simulation/catalog.go` and `internal/httpapi/server.go` are read-only reference for the wire shapes.
- 2026-09-01 frame: read the backend contract before framing - `integrationListResponse` and `deviceListResponse` both carry `api_version`, `scenario` and `items`; an integration item is `id`, `display_name`, `status`, `simulation`, `operations[]`, and a device item is `id`, `integration_id`, `display_name`, `kind`, `status`, `simulation`, `capabilities[]`; the deterministic scenarios are `healthy` and `unavailable`, and both currently return exactly one integration and one device.
- 2026-09-01 frame: expected requirements - two new REQ-FUNC (integration inventory retrieval, device inventory retrieval), three new REQ-UX (inventory display with integration ownership and availability, legitimately empty inventory as an explicit empty state, per-resource malformed inventory identification), and updates to REQ-FUNC-3d412917 (failure classification covers every resource, not only status), REQ-TEST-76b94249 (the mock routes by API path across all three resources and their scenarios), REQ-PLAT-b55586c0 (every resource path is origin-relative, not only the status path), REQ-UX-0018c647, REQ-UX-66636ac9 and REQ-UX-188a4860 (their verifications extend to the inventory screens).
- 2026-09-01 frame: user constraints recorded - stay on `master`, commit nothing until explicit delivery approval, do not rewrite or amend commit 2cf1a2a, leave the untracked `00_Planning/TODO.md` exactly as it is and outside this task, and mark slice 4 implemented only if the GUI works with both the mock and the real WSL2 backend for status, integrations and devices.
- 2026-09-01 frame: before opening this task, reconciled TASK-524e8dde - the human had approved it and then committed and pushed 2cf1a2a manually, leaving that journal at `stage: deliver` with an unticked checklist; its delivered diff was verified against the recorded proof, the journal was closed out to `stage: done` truthfully with the late-close-out deviation recorded, and its close-out commit is still owed.
- 2026-09-01 frame: stage advanced to specify.
- 2026-09-01 specify: created REQ-FUNC-539355e0 (integration inventory retrieval through the typed client), REQ-FUNC-72bffd89 (device inventory retrieval through the typed client), REQ-UX-8fa0c15e (status, integrations and devices with ownership and availability on one authenticated screen), REQ-UX-afb79ae0 (a legitimately empty inventory shown as an empty state rather than a failure), and REQ-UX-8169ea1d (naming which resource returned an unusable response while the readable ones stay visible).
- 2026-09-01 specify: updated six existing requirements rather than duplicating them - REQ-FUNC-3d412917 now classifies failures for any resource instead of only the status request, REQ-TEST-76b94249 now requires the mock to route by API path across all three resources and their scenarios, REQ-PLAT-b55586c0 now covers every request path rather than only `/api/v1/status`, REQ-PLAT-f852c5b2's verification now names every mock scenario, REQ-UX-0018c647's verification now covers the inventory scroll compartment, and REQ-UX-188a4860's verification now covers the inventory screens.
- 2026-09-01 specify: updated REQ-SEC-ed66f6ed's verification so the memory-only guarantee is proven across all three authenticated requests and their URLs, because the same token is now reused for two more resources.
- 2026-09-01 specify: no spec written - each requirement is buildable from its own prose, and the wire shapes it depends on already exist in `05_Software/nilo_assistant/internal/httpapi/server.go` and `internal/simulation/catalog.go`.
- 2026-09-01 specify: `python .agentic/aegis.py check` reported errors=0 warnings=6, every warning a `stale-paths` entry for `src/components/InventoryPanel.tsx` or `src/api/contract.test.ts`, which build has not created yet; expected before implementation and not accepted deviations.
- 2026-09-01 specify: stage advanced to design.
- 2026-09-01 design: wrote the design note - one shared `request(path, token, parse)` inside the client so all three resources share exactly one failure classification, a path-first mock switch so each resource can answer differently in one scenario, and a status-first session model where a status failure owns the whole screen while each inventory keeps its own `Resource<T>` region.
- 2026-09-01 design: kept `malformed` as an accepted alias of the new `malformed-status` scenario name so an existing ignored `.env.local` selecting it does not start failing configuration validation.
- 2026-09-01 design: revised the `## Architecture` diagram on `.agentic/knowledge/gui-foundation.md` to the intended post-change shape and marked it as intended until validate.
- 2026-09-01 design: `python .agentic/aegis.py check` reported errors=0 warnings=6, all `stale-paths` for the two files build has yet to create.
- 2026-09-01 design: stage advanced to build.
- 2026-09-01 build red: command=`npm --prefix ./05_Software/nilo_gui run test:ci -- src/api/client.test.ts src/api/contract.test.ts`; exit=1; 19 failed and 11 passed; expected missing behavior=REQ-FUNC-539355e0, REQ-FUNC-72bffd89, the widened REQ-FUNC-3d412917 and REQ-PLAT-b55586c0 have no inventory methods and no declared path list, so every inventory assertion fails with `client.getIntegrations is not a function` or `client.getDevices is not a function`, and `contract.test.ts` cannot resolve `BENCH_CORE_PATHS`, `INTEGRATIONS_PATH`, `DEVICES_PATH` or `resourceUrl`.
- 2026-09-01 build: `python .agentic/aegis.py anchor --task TASK-8fd83f88` recorded 199 files at 2cf1a2a after that red and before any implementation edit.
- 2026-09-01 build green: command=`npm --prefix ./05_Software/nilo_gui run test:ci -- src/api/client.test.ts src/api/contract.test.ts`; exit=0; 41 tests passed after extending `src/api/contract.ts` with the two inventory contracts, their strict parsers and the shared `resourceUrl`, and `src/api/client.ts` with one private `request(path, token, parse)` behind `getStatus`, `getIntegrations` and `getDevices`.
- 2026-09-01 build: unified the contract's parse results on one `ParseResult<T>` carrying `value`, replacing the status-only `StatusParseResult` that carried `status`; nothing outside `client.ts` consumed the old shape.
- 2026-09-01 build red: command=`npm --prefix ./05_Software/nilo_gui run test:ci -- src/api/mock.test.ts`; exit=1; 12 failed and 7 passed; expected missing behavior=the widened REQ-TEST-76b94249 has a scenario-only mock, so it exports no inventory bodies, cannot route by path, and answers `unauthorized` for `malformed-status`, `empty` and the two per-inventory malformed scenarios because none of those names exists.
- 2026-09-01 build green: command=`npm --prefix ./05_Software/nilo_gui run test:ci -- src/api/mock.test.ts`; exit=0; 19 tests passed after rewriting `src/api/mock.ts` to switch on the request path first and the scenario second, with eight scenarios and an unrouted path answered 404 rather than with a body.
- 2026-09-01 build red: command=`npm --prefix ./05_Software/nilo_gui run test:ci -- src/App.test.tsx src/layout.test.ts`; exit=1; 16 failed and 14 passed; expected missing behavior=REQ-UX-8fa0c15e, REQ-UX-afb79ae0 and REQ-UX-8169ea1d have no inventory view, so no `Integrations` or `Devices` region exists to query, and REQ-UX-0018c647's widened layout contract finds no `.inventory-list` or `.connected` rule.
- 2026-09-01 build green: command=`npm --prefix ./05_Software/nilo_gui run test:ci`; exit=0; 113 tests in 7 files passed, up from 43 in 6, after adding `src/components/InventoryPanel.tsx`, giving `App` a concurrent three-resource request whose status outcome settles the screen while each inventory keeps its own, and adding the `.connected`, `.inventory-*` and `.entry-*` rules.
- 2026-09-01 build: the two layout assertions first failed with `no top-level rule for .inventory-list` because `layout.test.ts`'s crude CSS reader treats a comment before a selector as part of the selector; stripped comments in its `stylesheet()` helper rather than removing the documentation from `App.css`, leaving every assertion unchanged.
- 2026-09-01 build: DEVIATION build - the environment assertions added for REQ-PLAT-f852c5b2 and the origin-relative assertions added for REQ-PLAT-b55586c0 in `environment.test.ts` passed on first run with no implementation change, so those two widened verifications have no pre-implementation red of their own; REQ-PLAT-b55586c0 does have an honest red through `contract.test.ts`, and the behavior holds by construction because all three methods share one `resourceUrl` call, but the environment-level assertions are regression guards rather than tests that drove code.
- 2026-09-01 build: `check` flagged `secret-in-diff` on `src/api/client.test.ts` again; following the precedent set in TASK-524e8dde, shortened the fake fixture credential to `fixture-tok` instead of accepting a permanent false positive, which keeps the assertion that the token reaches the header and never the URL.
- 2026-09-01 build: `npm run format` exit=0, `npm run lint` exit=0, `npm run build` exit=0 (`tsc -b` plus a 207.03 kB static bundle), `npm run test:ci` exit=0 with 113 tests, and `python .agentic/aegis.py check` reported errors=0 warnings=0 notes=0.
- 2026-09-01 build: `python .agentic/aegis.py evolution --task TASK-8fd83f88` reported 13 changed files (1 added, 12 modified, 0 removed); every path is one the design note named, and `src/api/contract.test.ts` is absent from the report only because the anchor was taken after the red tests were written, as the build step requires, so that file is part of the anchored baseline rather than a later addition.
- 2026-09-01 build: wrote the GUI README, the development-flow document and the `.env.mock` scenario table; deliberately left `00_Planning/Development_Simulation_Vertical_Slices.md` and the knowledge page to validate, because the user made marking slice 4 implemented conditional on the live WSL2 browser check that validate performs.
- 2026-09-01 build: corrected a wrong claim before it left the repository - the development-flow document briefly said `--simulation-scenario unavailable`; the Go CLI's flag is `--scenario`, confirmed against `05_Software/nilo_assistant/cmd/nilo/main.go`.
- 2026-09-01 build: stage advanced to validate.
- 2026-09-01 validate: reviewed the diff hunk by hunk against the frame scope - `git diff --name-only 2cf1a2a -- 05_Software/nilo_assistant` returns nothing, so no backend file or wire format changed; `package.json` and `package-lock.json` are unchanged, so no dependency was added; no browser end-to-end infrastructure, no lifecycle script and no theme selection appears anywhere in the diff.
- 2026-09-01 validate: reviewed security - no `localStorage`, `sessionStorage`, `document.cookie`, `indexedDB` or `console.` call exists in any non-test source file, and the only absolute URL in source is `DEFAULT_API_BASE_URL`, which is the proxy target and the login screen's label rather than a fetch target.
- 2026-09-01 validate: reviewed error paths - a non-`BenchCoreError` rejection is still classified as unavailable by `settle`, an inventory failure never blanks the status, the checking screen exposes no action so a second request cannot start while one is in flight, and an unrouted mock path answers 404 instead of a body so a caller's mistake cannot look like an empty inventory.
- 2026-09-01 validate: reviewed portability - the new code is browser and Node standard library only, with no path separator, no OS branch, and `node:fs` used only by `src/layout.test.ts` and `src/config/devServer.test.ts`.
- 2026-09-01 validate: `00_Planning/TODO.md` is still untracked, unstaged and unmodified, exactly as the user required, and is excluded from the proposed commit.
- 2026-09-01 validate: manual observation of the live WSL2 path, healthy scenario - started `nilo serve --profile simulation --scenario healthy --bind 127.0.0.1:8090` in Ubuntu-24.04, issued a development token, served the GUI with `npm run dev` on 127.0.0.1:5173, pasted the token, and observed the status panel plus both inventories: integration `simulation.fake-python` state `healthy` with its five operations, device `simulation.fake-python.device.stable-001` availability `ready`, owning integration `simulation.fake-python`, kind `simulated-electronics-bench`, each region reporting `1 entry` and `scenario: healthy`.
- 2026-09-01 validate: manual observation of REQ-PLAT-b55586c0 in the real browser - the network log shows exactly `GET http://127.0.0.1:5173/api/v1/status`, `/api/v1/integrations` and `/api/v1/devices`, all three 200 and all three addressed to the page's own origin, so the proxy carried them and no request was ever cross-origin or preflighted.
- 2026-09-01 validate: manual observation of REQ-SEC-ed66f6ed and REQ-UX-0018c647 in the real browser - `localStorage.length` and `sessionStorage.length` were both 0, `document.cookie` was empty, the token appeared in no URL, `document.documentElement.scrollHeight > clientHeight` was false at 1280x720, and both `.inventory-list` regions computed `overflow-y: auto`.
- 2026-09-01 validate: manual observation of the backend-unavailable state - stopped the WSL2 server, pressed Refresh, and the interface showed "Bench Core is not reachable." with the 502 detail and both recovery actions, then recovered to the connected screen when the backend was restarted and Try again was pressed.
- 2026-09-01 validate: manual observation of the live WSL2 path, unavailable scenario - restarted with `--scenario unavailable` and observed the service status still `healthy` while both regions reported `scenario: unavailable`, integration state `unavailable` and device availability `unavailable`, with the same identities as the healthy run; the SIMULATION indicator and the physical-equipment boundary statement were present in every observed state and the browser console reported no errors.
- 2026-09-01 validate: manual observation of mock mode in the real browser - `npm run dev:mock` served the interface, the footer read that answers come from the interface's own contract mock, the full status and inventory screen rendered, and the network log shows no `/api/v1/*` request of any kind, so nothing was contacted.
- 2026-09-01 validate: manual observation of REQ-UX-8169ea1d in the real browser - served mock mode with `VITE_NILO_MOCK_SCENARIO=malformed-devices` and observed the status and the integration inventory rendered normally while only the device region read "The device inventory could not be read." with the violation "the device inventory entry 0 is missing integration_id".
- 2026-09-01 validate: DEVIATION validate - in mock mode the harness's synthetic click and keystrokes set the input's DOM value but never submitted the form, so the login was completed by dispatching a native input event and a real DOM `click()` from the browser console; the same synthetic interactions did work against the live server on port 5173 and the application behaved correctly as soon as a genuine DOM click reached it, so this is an artifact of a browser pane that was not compositing frames, not a defect in the interface. Screenshots were unavailable for the same reason, so every live observation above was taken from the accessibility tree, the page text, and the network log rather than from an image.
- 2026-09-01 validate: DEVIATION validate - the live browser session is a manual method standing in for a test that could be automated; it is deliberately not automated here because the user excluded browser end-to-end infrastructure from this task and vertical-slice 6 owns it. The vertical-slice document and the development-flow document both now say this evidence is manual and is not the repository gate.
- 2026-09-01 validate: stopped every process this validation started - the WSL2 backend and the three Vite servers; confirmed WSL port 8090 free and Windows ports 5173, 5174 and 5175 no longer listening.
- 2026-09-01 validate: rewrote `.agentic/knowledge/gui-foundation.md` to the built shape - the three-method client interface, the eight mock scenarios, the six new behaviors, the status-settles-the-screen rule with per-inventory resources, and the new file inventory; removed the design-stage line that marked the architecture diagram as intended.
- 2026-09-01 validate: marked slice 4 implemented in `00_Planning/Development_Simulation_Vertical_Slices.md` only after both halves of its exit gate were observed - the contract mock and the real WSL2 backend, each for status, integrations and devices - and stated in the same section that the live evidence is manual and that slice 6 still owes the automated browser gate.
- 2026-09-01 validate: re-read all thirteen requirements - each is `active`, states one behavior, and its verification names an assertion that exists; `affected_paths` and `linked_tests` were corrected to include `src/api/contract.test.ts`, `src/components/InventoryPanel.tsx` and the documentation this task changed.
- 2026-09-01 validate: `python .agentic/aegis.py evolution --task TASK-8fd83f88` refreshed to 16 changed files (1 added, 15 modified, 0 removed) after the documentation and knowledge edits; every path is one the design note named or a documentation file the scope required, and nothing was removed.
- 2026-09-01 validate: the first `check --strict --run` exited 0 with `strict: ok` and one `journal-header` warning for a Handoff still naming build; refreshed the Handoff and reran rather than accepting it.
- 2026-09-01 validate: command=`.\02_Tools\env\with-tools.ps1 python .agentic/aegis.py check --task TASK-8fd83f88 --strict --run`; exit=0; `run: test ok` (16.7s), `run: lint ok` (5.8s), `strict: ok`, errors=0 warnings=1 - the full configured suite (Go `go test ./...`, the Python SDK and fake-integration suites, the repository script tests, and the frontend `test:ci` plus `build`) and the full lint set (`go vet`, `compileall`, `prettier --check`, `eslint`) all passed.
- 2026-09-01 validate: the single remaining warning is `unticked-deliver`, six unticked items in the deliver checklist; it is a true statement about a delivery that has not happened yet rather than a finding, so it is not an accepted deviation and will clear as delivery proceeds.
- 2026-09-01 validate: counted the candidate against commit 2cf1a2a - 25 tracked files changed with 2041 insertions and 370 deletions, plus 10 untracked files to add, and `00_Planning/TODO.md` left untracked and excluded.
- 2026-09-01 validate: stage advanced to deliver; the candidate is proven and awaits the human's approval before any commit.
- 2026-09-01 deliver: the user reported a P2 at the delivery checkpoint - the five newly added inventory requirement files carry CRLF line endings that Git reports as trailing whitespace on every line.
- 2026-09-01 deliver: confirmed the report before acting - `file` classifies all five as "CRLF line terminators" while every other file in the candidate and every previously committed requirement is LF, `core.autocrlf` is `false` with `core.eol=lf` and no `.gitattributes`, so nothing would have normalised them on commit, and `git diff --cached --check` flagged 602 lines across exactly those five files and no others.
- 2026-09-01 deliver: found the cause in my own work - the build-stage helper that set `status = "active"` and filled `linked_tests` used Python's `pathlib.write_text`, whose Windows text mode rewrites every `\n` as `\r\n`; the eight requirements edited with ordinary edits were untouched and stayed LF, which is why only these five drifted.
- 2026-09-01 deliver: returned to validate per `6_deliver.md` step 1, because normalising the files changes the candidate and the recorded proof described the CRLF tree.
- 2026-09-01 validate: normalised all five files by reading and writing bytes and replacing `\r\n` with `\n`, removing 61, 61, 62, 58 and 59 CR bytes respectively; no character of content changed.
- 2026-09-01 validate: verified the fix - `file` now reports LF only for all five, `tr -cd '\r' | wc -c` counts 0 CR bytes in both the worktree copies and the staged blobs, matching the existing committed requirements, and `git diff --cached --check` exits 0 with no flagged lines.
- 2026-09-01 validate: noted a state change I did not make - the whole candidate was staged between the delivery presentation and this fix, 35 paths, with `00_Planning/TODO.md` correctly left untracked and unstaged; `git rev-parse HEAD` still reports 2cf1a2a, so nothing has been committed.
- 2026-09-01 validate: `python .agentic/aegis.py check` reported errors=0 after the rewrite, so all five requirement files still parse as TOML and only their line endings changed; `evolution` was refreshed and still reports the same file set.
- 2026-09-01 validate: command=`.\02_Tools\env\with-tools.ps1 python .agentic/aegis.py check --task TASK-8fd83f88 --strict --run`; exit=0; `run: test ok` (17.8s), `run: lint ok` (6.2s), `result: ok`, `strict: ok`, errors=0 warnings=0 notes=0 - no finding of any kind remains against the normalised candidate.
- 2026-09-01 validate: stage advanced to deliver for the second time; the line-ending fix is proven and the candidate again awaits the human's approval before any commit.
- 2026-09-01 deliver: the human approved delivery of the re-proven candidate and asked for the commit message.
- 2026-09-01 deliver: confirmed the approved candidate is unchanged since the final proof - `git diff --cached --check` is empty, every staged blob is LF, 35 paths are staged, `00_Planning/TODO.md` remains untracked and excluded, and `git rev-parse HEAD` still reports 2cf1a2a.
- 2026-09-01 deliver: the delivery commit also carries TASK-524e8dde's close-out; that task's outcome, a commit pushed to `origin`, had already succeeded, so nothing can fail after this commit for it and its close-out item is ticked here rather than deferred to a further commit.
- 2026-09-01 deliver: this task's own close-out stays unticked until its outcome succeeds, because the repository delivers by pushing `master` to `origin` and a push can fail after the commit; a small close-out commit follows the push.
- 2026-09-01 deliver: ran `python .agentic/aegis.py report TASK-8fd83f88`; the appended report is a fresh report-time advisory snapshot and does not replace the validation evidence above.
- 2026-09-01 deliver: the human made the delivery commit and the push manually rather than through this workflow, so the session that presented the candidate ended before its own checklist could be ticked; commit 89aba98 is that delivery and it was already pushed when the next session opened.
- 2026-09-01 deliver: reconciled the approved candidate against the commit that carries it before ticking anything - `git diff --name-status 2cf1a2a 89aba98` lists exactly the 35 tracked paths recorded as staged, file for file, with no path added and none missing, and `git diff --shortstat` reports 35 files changed, 4117 insertions, 371 deletions.
- 2026-09-01 deliver: verified the line-ending fix survived into history - every one of the five new requirement blobs in 89aba98 contains 0 CR bytes, matching the proof recorded at validate.
- 2026-09-01 deliver: accounted for the one number that did not match rather than ignoring it - the delivery report says 36 files while the commit has 35, because `collect_diff` in `.agentic/aegis.py` appends untracked non-ignored files to its list and `00_Planning/TODO.md` was the 36th; the same mechanism confirms TODO.md was excluded from the commit, and it is still untracked and unmodified.
- 2026-09-01 deliver: the agreed outcome succeeded - `git rev-parse HEAD origin/master` reports 89aba98 for both, so the push this task deferred its close-out for has completed.
- 2026-09-01 deliver: TASK-524e8dde's close-out rode on this commit as planned and its journal reads `stage: done` in 89aba98.
- 2026-09-01 done: closed out on the evidence above, without rewriting or amending 89aba98; per `6_deliver.md` step 7 the close-out for a push-delivered task needs a later commit, so this journal edit is carried by the delivery of TASK-8a4ba5da and is presented there as part of that task's staged files.

## Design

Component: the browser GUI (`05_Software/nilo_gui`) only. The Go backend already
serves both resources; this task adds no endpoint and changes no wire format.

Interface changes, all internal to the GUI:

- `src/api/contract.ts` — add `INTEGRATIONS_PATH` and `DEVICES_PATH`, a
  `BENCH_CORE_PATHS` list, the `BenchCoreIntegration`, `BenchCoreDevice`,
  `IntegrationInventory` and `DeviceInventory` types, and strict
  `parseIntegrationsBody` / `parseDevicesBody` returning either a typed value or
  the field that made the body unusable. `resourceUrl(baseUrl, path)` replaces
  `statusUrl`, so one join serves all three (REQ-FUNC-539355e0, REQ-FUNC-72bffd89,
  REQ-PLAT-b55586c0). The parsers reject a non-array `items`, a non-string entry
  field, and a non-string-array `operations`/`capabilities`, so a wrong element
  type is malformed rather than rendered as `undefined`.
- `src/api/client.ts` — add `getIntegrations(token)` and `getDevices(token)`
  beside `getStatus`. The existing request/classify/parse body becomes one private
  `request(path, token, parse)` so all three share exactly one failure
  classification (REQ-FUNC-3d412917).
- `src/api/mock.ts` — `createMockTransport(scenario)` switches on the request
  path first and the scenario second, so each resource can answer differently.
  Scenario set grows to `healthy`, `unavailable-inventory`, `empty`,
  `unauthorized`, `unavailable`, `malformed-status`, `malformed-integrations`,
  `malformed-devices`; `malformed` is kept as an accepted alias of
  `malformed-status` so existing `.env.local` files keep working. Bodies stay
  typed against the contract's response interfaces, and the healthy and
  unavailable inventories reproduce the Go catalog's single integration
  `simulation.fake-python` and single device
  `simulation.fake-python.device.stable-001` exactly (REQ-TEST-76b94249).
- `src/components/InventoryPanel.tsx` — one `InventorySection` shell rendering
  the loading, failed, empty and populated cases, with `IntegrationInventoryView`
  and `DeviceInventoryView` supplying the columns
  (REQ-UX-8fa0c15e, REQ-UX-afb79ae0, REQ-UX-8169ea1d).

State: `App` requests all three resources concurrently with the one token it
already holds (REQ-SEC-ed66f6ed, RULING-003). If the **status** request fails,
the whole screen becomes the existing single failure screen, which is what keeps
a rejected token, an unreachable backend and a malformed status distinct
(REQ-FUNC-3d412917, REQ-UX-66636ac9). If the status succeeds, the token is good
and the backend is reachable, so each inventory is reduced to its own
`Resource<T>` — value or failure — and rendered in its own region, which is how a
malformed integration inventory is told from a malformed device inventory without
blanking the status (REQ-UX-8169ea1d). No persistence of any kind is added.

Layout: the connected screen becomes `auto auto minmax(0, 1fr)` inside the panel,
and `.inventory-list` carries `overflow-y: auto; min-height: 0`, so a long
inventory scrolls in its compartment while the page stays one viewport
(REQ-UX-0018c647). Colours stay CSS custom properties on `:root` (RULING-005).

Rejected alternative: fetching the inventories lazily, only when a section is
expanded. Rejected because REQ-UX-8fa0c15e asks for status, integrations and
devices readable at a glance on one screen, and a collapsed section would hide
exactly the availability state the screen exists to show.

Test seams:

- REQ-FUNC-539355e0, REQ-FUNC-72bffd89, REQ-FUNC-3d412917 — `src/api/client.test.ts`,
  recording stub transports passed to `createBenchCoreClient`.
- REQ-PLAT-b55586c0 — `src/api/contract.test.ts` over the declared paths, plus
  `src/config/environment.test.ts` driving all three configured-client methods.
- REQ-TEST-76b94249 — `src/api/mock.test.ts`, the production client driven over
  `createMockTransport` for every resource and scenario.
- REQ-PLAT-f852c5b2 — `src/config/environment.test.ts`, plain records.
- REQ-UX-8fa0c15e, REQ-UX-afb79ae0, REQ-UX-8169ea1d, REQ-UX-66636ac9,
  REQ-UX-188a4860, REQ-SEC-ed66f6ed — `src/App.test.tsx`, injected stub client.
- REQ-UX-0018c647 — `src/layout.test.ts`, reading the shipped stylesheets.

Expected files — new: `src/components/InventoryPanel.tsx`, `src/api/contract.test.ts`.
Modified: `src/api/contract.ts`, `src/api/client.ts`, `src/api/mock.ts`,
`src/api/client.test.ts`, `src/api/mock.test.ts`, `src/config/environment.ts`,
`src/config/environment.test.ts`, `src/App.tsx`, `src/App.test.tsx`, `src/App.css`,
`src/layout.test.ts`, `src/components/StatusPanel.tsx`, `.env.mock`, `README.md`,
`00_Planning/Development_Simulation_Vertical_Slices.md`,
`06_SoftwareDocu/Development_and_Simulation_Flow.md`,
`.agentic/knowledge/gui-foundation.md`.

## Rulings

- RULING-001 | date=2026-09-01 | decision=The frame checkpoint stop is suppressed for this task; the pre-delivery stop remains the only stop. | reason=The user's prompt states the scope, the exclusions, the constraints, the test obligations and the delivery presentation in full, and names approval before committing as the only checkpoint it wants. | source=user | supersedes=none
- RULING-002 | date=2026-09-01 | decision=This task is delivered on `master` rather than on a task branch, so the frame step's branch instruction is deliberately not followed. | reason=The user asked to work on the same branch, which is where TASK-524e8dde was delivered. | source=user | supersedes=none
- RULING-003 | date=2026-09-01 | decision=The bearer token stays in the same single React state cell that TASK-524e8dde introduced, and the inventory requests reuse it as an argument; no storage, cookie, URL parameter, log line, or other persistence is added for any resource. | reason=The user restated the memory-only credential constraint for this slice, and REQ-SEC-ed66f6ed already governs it. | source=user | supersedes=none
- RULING-004 | date=2026-09-01 | decision=A live browser smoke check against the real WSL2 backend, in both the healthy and unavailable simulation scenarios, is recorded at validate as integration evidence, explicitly labelled as not the automated browser end-to-end gate that vertical-slice 6 still owes. | reason=The user asked for the live check and for that separation to be stated. | source=user | supersedes=none
- RULING-005 | date=2026-09-01 | decision=No light/dark/system theme selection is implemented; every colour stays a CSS custom property on `:root` so a later theme slice can override them. | reason=The user deferred theme selection again while asking that the variable structure stay ready for it; this continues TASK-524e8dde's RULING-003. | source=user | supersedes=none

## Handoff

current: done
status: ready
next: Nothing; this task is closed and its close-out edit ships with TASK-8a4ba5da's delivery commit.
must_read: .agentic/tasks/TASK-8fd83f88.md
blockers: none

## Stage report

stage: deliver
result: completed
current: done
changed: delivered as commit 89aba98 - 35 tracked files changed against commit 2cf1a2a (4117 insertions, 371 deletions), the exact path set recorded as staged; the only later change is this journal's close-out, which ships with TASK-8a4ba5da. Previously recorded at validate: 25 tracked files changed against commit 2cf1a2a (2041 insertions, 370 deletions) plus 10 untracked files to add - the GUI wire contract and its strict inventory parsers, the two new client methods, the path-routed mock, the new inventory panel, the status panel and shell, the single-viewport stylesheet, six test files, five new requirement files plus eight updated ones, this journal with its anchor and evolution records, the GUI knowledge page, and the GUI README, development-flow and vertical-slice documentation; `00_Planning/TODO.md` stays untracked and excluded
evidence: `.\02_Tools\env\with-tools.ps1 python .agentic/aegis.py check --task TASK-8fd83f88 --strict --run` exit=0 on the normalised candidate, run test ok 17.8s, run lint ok 6.2s, strict ok, errors=0 warnings=0 notes=0; `git diff --cached --check` exits 0 with no flagged lines and `tr -cd '\r' | wc -c` counts 0 CR bytes in every staged blob; `npm --prefix ./05_Software/nilo_gui run test:ci` exit=0 with 113 tests in 7 files, up from 43 in 6; `format:check`, `lint` and `build` each exit=0 with a 207.03 kB bundle; a live browser session against WSL2 Bench Core in both the healthy and unavailable scenarios, plus mock mode and a malformed-devices mock, all recorded in the log as manual integration evidence
issues: the reported P2 is fixed and re-proven - the five new requirement files were CRLF because a build-stage helper used Python text-mode writes, and they are now LF like every other file; two build deviations stand - the widened REQ-PLAT-f852c5b2 and the environment-level REQ-PLAT-b55586c0 assertions passed without a red of their own, and a manual browser session stands in for the automated browser gate slice 6 owes; two validate deviations stand - the browser pane never composited, so screenshots were unavailable and the mock-mode login needed a real DOM click from the console, and the live evidence is therefore drawn from the accessibility tree, page text and network log; TASK-524e8dde's close-out rode on commit 89aba98 and is no longer owed; this task's own close-out is recorded here and ships with TASK-8a4ba5da
next: Nothing; the close-out edit is carried by TASK-8a4ba5da's delivery
needs_user: no - the human approved delivery, made commit 89aba98 and pushed it manually, and that outcome is verified complete; nothing further is asked of them for this task
spec_compliance: disabled
implementation_quality: disabled
review_round:

## Delivery report (2026-09-01)

- stage: deliver; type: feature; path: full; branch: master
- requirements:
  - REQ-FUNC-539355e0 - Retrieve authenticated integration inventory from the GUI
  - REQ-FUNC-72bffd89 - Retrieve authenticated device inventory from the GUI
  - REQ-UX-8fa0c15e - Display simulated integrations and devices after authentication
  - REQ-UX-afb79ae0 - Show a legitimately empty inventory as an explicit empty state
  - REQ-UX-8169ea1d - Identify which inventory resource returned an unusable response
  - REQ-FUNC-3d412917 - Classify Bench Core API failures in the GUI client
  - REQ-SEC-ed66f6ed - Hold the GUI bearer token in memory only
  - REQ-UX-0018c647 - Fit the primary GUI screen in one viewport
  - REQ-UX-66636ac9 - Render distinct GUI connection states
  - REQ-UX-188a4860 - Identify the GUI as simulation in every state
  - REQ-PLAT-b55586c0 - Keep GUI backend requests same-origin behind a development proxy
  - REQ-PLAT-f852c5b2 - Configure the GUI API endpoint from the environment
  - REQ-TEST-76b94249 - Provide contract-aligned GUI mock responses
- deviations: 3 recorded
  - build - the environment assertions added for REQ-PLAT-f852c5b2 and the origin-relative assertions added for REQ-PLAT-b55586c0 in `environment.test.ts` passed on first run with no implementation change, so those two widened verifications have no pre-implementation red of their own; REQ-PLAT-b55586c0 does have an honest red through `contract.test.ts`, and the behavior holds by construction because all three methods share one `resourceUrl` call, but the environment-level assertions are regression guards rather than tests that drove code.
  - validate - in mock mode the harness's synthetic click and keystrokes set the input's DOM value but never submitted the form, so the login was completed by dispatching a native input event and a real DOM `click()` from the browser console; the same synthetic interactions did work against the live server on port 5173 and the application behaved correctly as soon as a genuine DOM click reached it, so this is an artifact of a browser pane that was not compositing frames, not a defect in the interface. Screenshots were unavailable for the same reason, so every live observation above was taken from the accessibility tree, the page text, and the network log rather than from an image.
  - validate - the live browser session is a manual method standing in for a test that could be automated; it is deliberately not automated here because the user excluded browser end-to-end infrastructure from this task and vertical-slice 6 owns it. The vertical-slice document and the development-flow document both now say this evidence is manual and is not the repository gate.
- explicit review: disabled; existing validation remains required
- diff vs task base 2cf1a2a: 36 file(s) changed, 4099 line(s) added
- checks: errors=0 warnings=1 notes=0
- checklist: 4/6 done; open: agreed outcome delivered, normally a PR; close-out committed after the outcome succeeded, or carried by the delivery commit where nothing could fail after it
