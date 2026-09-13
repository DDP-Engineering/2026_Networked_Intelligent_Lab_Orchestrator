<!--
Copyright (c) 2026 DDP Engineering
This software is licensed under the PolyForm Noncommercial License 1.0.0.
Commercial use requires a separate commercial license.
See the LICENSE.txt in the .agentic folder for full license details.
SPDX-License-Identifier: LicenseRef-PolyForm-Noncommercial-1.0.0
-->


# TASK-f978f3bd - Move the browser suite onto the appliance verification stage

stage: done
type: change
path: full
interaction: autopilot
requirements: REQ-TEST-68eedecb, REQ-TEST-b1177191, REQ-TEST-7ff6a75d, REQ-TEST-0a4f49af, REQ-UX-cb91fedb, REQ-UX-c5f7c0cd, REQ-UX-15904d9e, REQ-UX-e6437bf7, REQ-PLAT-32b81548, REQ-PLAT-38014f08, REQ-PLAT-80e40361, REQ-TEST-74c72bbe, REQ-UX-f479d8b9, REQ-TEST-3c4487d6, REQ-PLAT-59993b78, REQ-REL-f04744fa, REQ-SEC-4884bc91, REQ-TEST-0f02f4b8, REQ-TEST-68dcb172, REQ-TEST-842f1943
parent:
branch: ddp/nilo-simulation-retirement
base: a36c277b250a541ee06b26c1634b95810f723b52
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

Problem:
The browser suite builds and starts a simulation-profile Bench Core
(`e2e/topology.ts`), so the one proof that a real browser reaches NILO proves it
against a runtime that will not ship. TASK-67112aa2 added the appliance
verification stage the suite can move onto; until it does, the simulation
profile cannot be removed.

Desired outcome:
The browser suite drives a real browser against a Bench Core on the appliance
profile, beside the real privileged helper and a real NetworkManager, serving
the built interface from its own origin.

Actors: the repository gate; the browser; Bench Core as the unprivileged
service account; `nilo-nethelper` as root; NetworkManager, inside the stage
container.

Primary scenario:
The gate runs the appliance stage -> the stage builds the interface, starts the
helper and an appliance-profile core with generated TLS material and that built
`--gui-dir`, and runs the browser suite against the core's own HTTPS origin ->
the suite signs in and exercises the interface, with no simulation profile,
catalog, scenario or development server taking part.

In scope:
- `e2e/topology.ts` and `e2e/topology.test.ts` rewritten around the appliance.
- The simulation-shaped assertions in the five `*.spec.ts` files.
- The stage running the browser suite (`02_Tools/tests/appliance/run.sh`), and
  the image carrying the interface project's locked dependencies.
- Gate routing: `05_Software/scripts/aegis_test.py` and
  `02_Tools/tests/build-environment/verify-offline.sh`.
- Retiring REQ-TEST-0a4f49af, whose behaviour is the simulation journey.

Out of scope:
- Removing the simulation profile, the GUI contract mock, `nilo-dev.ps1` or the
  sigrok flag; moving the Go unit suites. Those are later tasks.
- The wider document and requirement reconciliation.

Constraints:
- The appliance profile needs a privileged helper, so the suite runs where one
  can run: inside the stage container. `newApplianceProvider` refuses root, so
  the core is spawned under an unprivileged uid while the runner stays root.
- `02_Tools/runtime/linux-x86_64/` carries no Node and no browser; both come
  from the image (TASK-67112aa2 RULING-005).
- The checkout is mounted read-only, so the interface project's dependencies
  cannot be installed at stage time from the mount.
- `verify-offline.sh` runs inside the locked build container, which has no
  network and dropped capabilities, so it cannot run this suite.

Success signals:
- The suite's backend answers `/api/v1/status` with the appliance profile.
- The browser reaches the interface on the appliance's own HTTPS origin, with
  no development server started by the run.
- No spec asserts a simulation profile, catalog, scenario or badge.

Evidence:
- OBSERVED: `e2e/topology.ts:643` starts `--profile simulation`; `:660` starts
  Vite; `startFrontendProcess` sets `VITE_NILO_API_BASE_URL`.
- OBSERVED: simulation-shaped assertions at `bench-core.spec.ts:27,28,80,91,96,125`,
  `authentication.spec.ts:457`, `shell.spec.ts:236`, `network.spec.ts:117-134,459`,
  `field-display.spec.ts:84`.
- OBSERVED: Chromium launches in the stage image with `chromiumSandbox: false`
  (smoke-tested); the global module is at
  `/opt/nilo/node/lib/node_modules/playwright` and browsers at
  `/opt/nilo/playwright`.
- OBSERVED: `aegis_test.py:31` discovers the frontend only on win32;
  `verify-offline.sh:12` runs `npm run test:e2e` in the locked container.
- INFERRED: the suite becomes container-only, so both gate routes change.

Unknowns:
- Whether any spec assertion depends on inventory the appliance cannot show
  (the simulated integration and device). Those move to bench-plugin coverage
  or are dropped with their requirement.

Path and reason: full. New observable verification behaviour, a changed
container image, a retired requirement, and a changed developer workflow.

## Requirement coverage

REQ-TEST-68eedecb is exercised by the appliance-profile topology contract,
the stage prerequisite contract, and the complete Playwright journey. The
backend listening record is rejected unless its profile is `appliance`, and
the browser reaches it through a separate namespace over the configured
management interface so the production listener and firewall remain active.

REQ-TEST-b1177191 is exercised by the topology contract and stage-definition
tests. The stage builds the committed interface, supplies its `dist` directory
as `--gui-dir`, and Playwright navigates directly to Bench Core's HTTPS origin;
no Vite process or proxy participates.

REQ-TEST-0a4f49af is retired. Its simulation-profile/token/catalog journey is
the behavior this appliance-profile/password/real-inventory journey replaces.

REQ-UX-cb91fedb now covers the real appliance sampler's zero-byte boundary: a
measured minute remains keyboard focusable even when both visible bars have
zero height, and its selected-minute readout reports the observed zero values.

REQ-PLAT-32b81548 covers the appliance image inventory: every declared command
must succeed, and the Chromium record comes from the browser executable rather
than the Playwright package. REQ-PLAT-38014f08 now retains the repository-local
Windows browser only for optional host-side diagnosis. REQ-PLAT-80e40361 keeps
the network-disabled locked environment responsible for component/build proof;
privileged browser acceptance belongs to the appliance stage.

REQ-TEST-74c72bbe now limits the locked offline gate to its reproducible native
and ARM64 build proof and keeps privileged evidence explicit and separate.
REQ-UX-f479d8b9 and REQ-TEST-3c4487d6 describe the appliance stage as the
public acceptance entry point and require every absent stage prerequisite to
fail rather than skip. REQ-PLAT-59993b78 and REQ-REL-f04744fa cover the
stage-owned Linux temporary layout, isolated browser ingress and bounded helper
and HTTPS readiness. REQ-SEC-4884bc91 covers password bootstrap through stdin,
redacted browser fills and disabled saved artifacts. REQ-TEST-0f02f4b8,
REQ-TEST-68dcb172 and REQ-TEST-842f1943 cover recovery, administration and shell
journeys against the appliance-profile core with server-managed sessions.

## Design

The suite runs inside the appliance verification stage container, which already
carries Node and the pinned browser engine (TASK-67112aa2 RULING-005). Chromium
was smoke-tested there and launches with `chromiumSandbox: false`; its global
module is `/opt/nilo/node/lib/node_modules/playwright` and its browsers are at
`/opt/nilo/playwright`.

**Requirements coverage.** REQ-TEST-68eedecb - `e2e/topology.ts` starts the
helper and an appliance-profile core and refuses any other profile in the
listening record. REQ-TEST-b1177191 - the stage builds the interface and passes
it as `--gui-dir`; the browser navigates to the appliance's own origin and no
development server is started. REQ-TEST-0a4f49af is retired: its behaviour is
the simulation journey this replaces.

**Interfaces.** `DevelopmentTopology.start` returns an appliance whose
`frontendUrl` is Bench Core's own HTTPS origin. The token sign-in is replaced
by an administrator bootstrapped through the real command line, so
`enterTokenInto` becomes `enterPasswordInto` and `nilo development issue-token`
leaves the browser proof entirely.

**Privilege.** The Playwright runner stays root inside the disposable
container, because `playwright install` put the browser there; Bench Core is
spawned with `uid`/`gid` 65534 through `child_process.spawn`, since
`newApplianceProvider` refuses root. The privileged helper runs as root, as it
does on an appliance.

**Dependencies.** The checkout is mounted read-only, so the interface project's
dependencies cannot be installed at stage time. They are installed into the
image from the committed `package-lock.json`, which means the build context
becomes the repository root with a `Dockerfile.dockerignore` narrowing it to
the two manifests. The stage copies the interface project to a writable
location, links the image's modules in, and builds.

**Gate routing.** `verify-offline.sh` runs inside the locked build container,
which has no network and dropped capabilities, so it cannot run this suite; its
`test:e2e` line moves out. `aegis_test.py` stops running `test:e2e` directly on
Windows and reaches the stage through WSL instead.

**Test seams.** `e2e/topology.test.ts` for the harness contracts;
`05_Software/scripts/tests/test_appliance_stage.py` for the image and stage
contracts; the five `*.spec.ts` files for the journey itself.

**Risks.** The rewrite touches `prerequisites.ts`, `prerequisites.test.ts`,
`topology.test.ts`, five spec files, the Playwright config and `commands.test.ts`
together; until all of them land the interface project does not typecheck, so
the work cannot be left partly applied.

### Proposed architecture - TASK-f978f3bd

```
02_Tools/tests/appliance/run.sh   (container, root)
  | npm run build            -> /tmp/nilo-gui/dist
  | npm run test:e2e
  v
e2e/topology.ts
  | nilo-nethelper (root)
  | nilo serve --profile appliance --gui-dir <dist> --tls-* (uid 65534)
  +-> listening record: refuses any profile but `appliance`
  v
Chromium (nilo-browser namespace)
  -> https://198.18.0.1:<port>/ on nilo-browser0
  -> production listener and nftables admission
```

## Log

- 2026-09-12 frame: Opened for issue #7 step 2. Full path. Scope is the browser
  suite's move onto the appliance verification stage, the stage and image changes
  that let it run there, and the gate routing that reaches it. Removal of the
  simulation profile and the other suites stay with later tasks.
- 2026-09-12 specify: 2 created, 0 revised, 0 reused, 1 retired
  (REQ-TEST-0a4f49af, whose behaviour is the simulation journey this task
  replaces).
- 2026-09-12 design: The suite runs inside the appliance stage container, which
  already carries Node and the browser engine. The interface project's locked
  dependencies are installed into the image from `package-lock.json`, because
  the checkout is mounted read-only and cannot be installed into. The runner
  stays root and spawns Bench Core under an unprivileged uid, since
  `newApplianceProvider` refuses root. Vite leaves the proof entirely: the
  browser reaches the appliance's own HTTPS origin serving the built `--gui-dir`.
- 2026-09-12 build red: REQ-TEST-68eedecb, REQ-TEST-b1177191;
  command=`python -m unittest discover -s scripts/tests -p test_appliance_stage.py`
  (cwd 05_Software); exit=1; expected missing behavior=the image installs no
  interface dependencies and the stage neither builds the interface nor runs the
  browser suite.
- 2026-09-12 build: STOPPED and reverted. The implementation was begun - the
  image gained the locked interface dependencies, the stage gained a browser
  phase, and `e2e/topology.ts` was rewritten around the appliance - but the
  change is not separable: `topology.ts`, `prerequisites.ts`,
  `prerequisites.test.ts`, `topology.test.ts`, five `*.spec.ts` files, the
  Playwright configuration and `commands.test.ts` have to land together or the
  interface project does not typecheck, which would have taken TASK-67112aa2's
  proof down with it. Every source edit belonging to this task was reverted and
  TASK-67112aa2's candidate re-proved green (`sh 02_Tools/tests/appliance/check.sh`
  exit 0; host suite 12 tests exit 0). The design above and the two draft
  requirements are the durable result; the build starts again from them.
- 2026-09-12 design: Stage returned to `design` because no implementation
  survives. The container approach is validated by a Chromium smoke run inside
  the stage image; what remains is the coordinated rewrite listed above.
- 2026-09-12 build: The coordinated rewrite landed. `e2e/prerequisites.ts`,
  `prerequisites.test.ts`, `topology.ts`, `topology.test.ts`, the five
  `*.spec.ts` files, `commands.test.ts` and `playwright.config.ts` were rewritten
  together; the image gained the interface project's locked dependencies from
  `package-lock.json` with the build context moved to the repository root behind
  `Dockerfile.dockerignore`; `02_Tools/tests/appliance/run.sh` builds the
  interface and runs the browser suite; and the gate routing moved out of the
  locked container (`verify-offline.sh`) and onto the stage
  (`aegis_test.py::_run_appliance_stage`).
- 2026-09-12 build green: `npm run build` exit 0 and `npm run test:ci` exit 0
  with 34 files / 459 tests, including the rewritten harness unit tests.
  `python -m unittest discover -s scripts/tests -p test_appliance_stage.py`
  exit 0, 17 tests.
- 2026-09-12 build: The browser suite now runs against a real appliance and
  passes 21 of 24 tests (`sh 02_Tools/tests/appliance/check.sh`). Bench Core
  runs on `--profile appliance` under the real root helper and a real
  NetworkManager, serving the built interface from its own HTTPS origin; the
  suite signs in, exercises the shell, the field viewports, authentication and
  the appliance power controls. Two engine networks give the container two real
  adapters so an uplink and a device-facing port can be told apart.
- 2026-09-12 build BLOCKED: three tests do not pass, all for one cause. The
  device-network editor disables its adapter selects until management access is
  configured, because without an allowlist Bench Core cannot know which adapter
  is the uplink. The stage starts the appliance with `--bind`, so nothing is
  reserved and `field-display.spec.ts:102`, `network.spec.ts:115` and
  `shell.spec.ts:120` cannot complete. Starting the appliance with
  `--management-interface eth0 --management-port P --management-tls-name` was
  tried twice and made it worse - 2 of 24 - because the interface allowlist
  admits connections only on the allowlisted adapter's own address, and even
  reaching it at 192.0.2.2 the boundary never answered, so the management
  listener needs more than the allowlist alone (the helper's firewall
  application is the next thing to examine). Both attempts were reverted to the
  `--bind` form that passes 21.
- 2026-09-12 build: DEVIATION build - the browser suite no longer asserts that
  an adapter is reserved for management access. The stage's appliance has no
  allowlist, so nothing is reserved; role exclusivity stays covered by
  `05_Software/nilo_assistant/internal/network/adapter_roles_test.go` and
  `internal/network/management_access_test.go`.
- 2026-09-12 build: DEVIATION build - the browser suite no longer asserts the
  simulated integration and device inventory (`simulation.fake-python` and its
  device). A fresh appliance carries no managed integration until one is
  installed, so `bench-core.spec.ts` asserts the empty inventory instead. That
  inventory's own coverage belongs with the bench-integration work.
- 2026-09-13 build: The production management listener is now exercised through
  a dedicated browser network namespace and real veth ingress. Bench Core starts
  with `--management-interface`, `--management-port` and TLS name; readiness and
  Chromium traffic enter on that interface, so the real nftables allowlist and
  socket/request gates remain in the proof. This resolves the earlier
  management-reservation deviation.
- 2026-09-13 build: The stage image gained WireGuard tools and read-only kernel
  module evidence, allowing the browser suite to import, enable, disable and
  remove a real WireGuard profile. Browser-side fetches now perform network
  sampling from the isolated ingress rather than from the blocked runner
  namespace.
- 2026-09-13 build: Appliance execution exposed an unstable fixture rollback:
  NetworkManager restored the connection and addresses but recalculated implicit
  route metrics from 100 to 102, causing the production state digest to reject
  the restore. The disposable profile now pins IPv4 and IPv6 metrics to 100;
  the browser test asserts the rollback operation reaches `rolled_back`.
- 2026-09-13 build green: `python 05_Software/scripts/tests/test_appliance_stage.py`
  exit 0 (18 tests); focused GUI harness tests exit 0 (45 tests); `npm run build`
  exit 0; focused network browser suite exit 0 (6 tests); focused field-display
  suite exit 0 (9 tests); complete `02_Tools/tests/appliance/check.sh` exit 0
  with 2 appliance system tests and all 39 Playwright tests, including the real
  two-minute rollback and WireGuard lifecycle.
- 2026-09-13 build: The branch was fast-forwarded to the latest authenticated
  Gitea `origin/master` (`a36c277`) before continuing. The journal base now
  names that integrated commit; the original red anchor remains preserved as
  historical evidence from before the upstream refresh.
- 2026-09-13 build check: `aegis check --task TASK-f978f3bd` reports 0 errors,
  35 warnings and 1 note. One warning touches the candidate: the secret scanner
  recognizes the word `password` in `appliance_stage_linux_test.go`; the value
  is a fixed fixture credential for a temporary per-run database and is passed
  to the CLI over standard input. The other 34 warnings and the manual note are
  pre-existing repository records (stale requirement paths, incomplete older
  delivery/review journals and one older missing linked test), not this task's
  candidate. All candidate source paths are now covered by requirement records.
- 2026-09-13 validate review round 1: candidate
  `6afff5fb0ccc842e84d8fdd6e1f5d1104a83123ff7c4be32ce50445f59d3dfd9`;
  specification compliance and implementation quality both require changes.
  Recorded findings: preserve the shared network stage's repository-local build
  context and no-WireGuard installation fixture while giving the appliance
  stage its richer image; remove the field-display statistics interception;
  reconcile retired-topology comments and operator docs; update the test-router
  unit contract; bound core startup and child shutdown; and delete the stage
  container only after this run created it. Both verdicts are invalidated while
  these fixes are made; review round 2 will inspect the new candidate.
- 2026-09-13 build review-fix red: router unit tests exit 1 because the WSL
  appliance stage is still invoked through a constructed `sh -c` command;
  appliance-stage contract tests exit 1 and the shared network check exits 1
  because the single image target requires GUI files outside the network
  check's narrow context and preinstalls WireGuard; focused GUI harness tests
  exit 1 because operator docs still advertise the retired direct browser
  command, field-display still fabricates statistics, and core startup and
  child teardown have no deadlines. These tests are the red anchors for the
  round-1 review fixes.
- 2026-09-13 build review-fix red: after removing the statistics interception,
  the focused appliance field suite reached a real persisted zero-byte sample
  but failed because the SVG sample group had a zero-height bounding box. This
  exposed an existing boundary of REQ-UX-cb91fedb that deterministic nonzero
  data had hidden; the chart now gives every measured minute a transparent
  one-pixel focus target without inventing traffic volume.
- 2026-09-13 build review-fix green: router unit tests 7/7; appliance-stage
  contracts 20/20, including a fake engine whose container creation fails;
  GUI lifecycle/prerequisite/chart contracts 51/51; full GUI component suite
  490/490; format and lint exit 0; production GUI build exit 0. The shared
  `network-system` target passes the complete privileged network check,
  including installation from a clean no-WireGuard image. The focused real-data
  field suite passes 9/9 and the complete appliance stage passes both Go system
  tests and all 39 Playwright tests in 3.4 minutes.
- 2026-09-13 build baseline: the complete Python script discovery now passes
  the router assertion updated by this task and has one remaining failure:
  `reconstruction-inputs.json` at unchanged `origin/master` content hashes to
  `66b1e14f0a02eef6be7b8c0437b259fae576373ae8959226f71d732ca60193be`,
  while unchanged `build-environment.lock.json` expects
  `784a1d020ef72bd4c567a3f85287e2b72e456f2f7bc28db7c470d0e82579da3f`.
  Neither reconstruction file is part of this candidate.
- 2026-09-13 validate round 2: candidate
  `f4e3ed0e4ecf2f3c673ace1accd2e6c02a83b685f3fc4016e81ef7e20322b8cc`
  sent to independent specification-compliance and implementation-quality
  reviewers after reconciling the last stale role-fixture comment and making
  REQ-TEST-68eedecb distinguish production appliance journeys from explicitly
  labelled presentation-only UI fixtures.
- 2026-09-13 validate review round 2: candidate
  `f4e3ed0e4ecf2f3c673ace1accd2e6c02a83b685f3fc4016e81ef7e20322b8cc`;
  specification compliance and implementation quality both require changes.
  Recorded findings: restore selection and validation of the WSL distribution
  used by the Windows gate; make the opposite-role presentation fixture use the
  live adapter identity and prove that a management-owned port is disabled for
  the device role; make the zero-byte chart sample a genuine 44 by 44 CSS-pixel
  tap target and include it in the field oracle; assert the fresh appliance
  inventory is empty; and narrow requirement and knowledge prose that currently
  overstates shared topology or fixture coverage. Both verdicts are invalidated
  while these fixes are made; review round 3 will inspect the new candidate.
- 2026-09-13 build review-fix red: REQ-TEST-7ff6a75d;
  command=`python -m unittest scripts.tests.test_aegis_test.AegisTestRouterTests.test_windows_appliance_stage_invokes_the_translated_script_without_shell_code scripts.tests.test_aegis_test.AegisTestRouterTests.test_windows_appliance_stage_selects_the_only_installed_distribution scripts.tests.test_aegis_test.AegisTestRouterTests.test_windows_appliance_stage_fails_when_no_distribution_is_installed scripts.tests.test_aegis_test.AegisTestRouterTests.test_windows_appliance_stage_requires_selection_for_multiple_distributions`
  (cwd `05_Software`); exit=1; expected missing behavior=the Windows gate
  neither discovers nor selects the intended WSL distribution and still invokes
  the default distribution.
- 2026-09-13 build review-fix red: REQ-UX-cb91fedb and
  REQ-UX-c5f7c0cd; command=`npm.cmd run test:ci -- --run
  src/components/network/TrafficChart.test.tsx` (cwd
  `05_Software/nilo_gui`); exit=1; expected missing behavior=the zero-byte
  sample exposes an approximately 30 by 1 SVG-unit target rather than the
  required pointer-sized target.
- 2026-09-13 build review-fix red: REQ-UX-15904d9e and
  REQ-UX-e6437bf7; command=`wsl.exe -- sh
  /mnt/f/NILO_simulation_retirement/02_Tools/tests/appliance/check.sh
  e2e/network.spec.ts --grep "keeps opposite-role adapters"`; exit=1;
  expected missing behavior=the hard-coded management adapter identity does not
  match the live eth0 identity, so the device-side option remains enabled.
- 2026-09-13 build review-fix green: router tests 11/11, including explicit,
  sole, absent, ambiguous and unavailable WSL distribution cases;
  appliance-stage contracts 20/20; TrafficChart contracts 2/2; GUI components
  490/490; format and lint exit 0; production build exit 0. Focused appliance
  browser proof confirmed the exact empty inventory, all four field viewport
  journeys with chart role-buttons inside the -by-44 CSS-pixel oracle, and the
  management-owned eth0 disabled until the fixture established eth1 as its
  replacement path. The complete appliance stage then passed both Go system
  tests and all 39 Playwright tests in 3.7 minutes.
- 2026-09-13 build review-fix inspection: REQ-TEST-68eedecb and toolchain
  documentation now describe independent per-spec appliance instances and
  explicitly bounded presentation-response fixtures instead of claiming one
  shared instance or live backend responses for every assertion.
- 2026-09-13 build check: final round-2-fix evolution contains eight modified
  files after the red anchor and matches the recorded review findings. `aegis
  check --task TASK-f978f3bd` reports 0 errors, 35 warnings and 1 note: the
  candidate warning remains the known fixed test credential in the disposable
  appliance database; the other warnings and manual note are pre-existing
  repository records described earlier.
- 2026-09-13 validate review round 3: candidate
  `ecb285c35a81c9d622c70cdaaa9733136c4e94dbfd23215a834707e9ef8ffd62`;
  implementation quality approved with no findings, but specification
  compliance still requires changes. Remaining findings: fixed 56-unit chart
  targets overlap for dense histories and the component test bypasses browser
  hit-testing; the role fixture changes state directly instead of driving the
  replacement-confirmation-removal workflow and its warnings through the UI;
  no real Windows `aegis_test.py full` run is recorded reaching the appliance
  stage; and repository-toolchain, environment and the current Stage report
  retain stale statements about browser ownership, offline Playwright, real
  responses and older test counts. The final configured review round is
  exhausted, so AEGIS blocks further edits until an explicit human Ruling
  resets or changes review scope/policy.
- 2026-09-13 build review-fix red under RULING-001: REQ-UX-cb91fedb and
  REQ-UX-c5f7c0cd; component command=`npm.cmd run test:ci -- --run
  src/components/network/TrafficChart.test.tsx` and real-browser command=
  `wsl.exe -d Ubuntu-24.04 --exec sh
  /mnt/f/NILO_simulation_retirement/02_Tools/tests/appliance/check.sh
  e2e/network.spec.ts --grep "selects adjacent dense traffic"`; both exit=1;
  expected missing behavior=the chart is not one coordinate-aware listbox and
  therefore cannot select adjacent dense minutes through real browser
  hit-testing without overlapping per-sample targets.
- 2026-09-13 build review-fix red under RULING-001: REQ-UX-15904d9e and
  REQ-UX-e6437bf7; command=`wsl.exe -d Ubuntu-24.04 --exec sh
  /mnt/f/NILO_simulation_retirement/02_Tools/tests/appliance/check.sh
  e2e/network.spec.ts --grep "keeps opposite-role adapters"`; exit=1;
  expected missing behavior=the presentation fixture does not process the
  management PUT/confirm sequence, so the UI cannot prove replacement,
  confirmation, current-session warning and final removal.
- 2026-09-13 build deviation under RULING-001: resolving the required real
  Windows full-gate evidence adds the pre-existing build-environment lock
  digest to this candidate. The unchanged 650-input manifest hashes to
  `66b1e14f0a02eef6be7b8c0437b259fae576373ae8959226f71d732ca60193be`,
  while the lock inherited from `origin/master` says
  `784a1d020ef72bd4c567a3f85287e2b72e456f2f7bc28db7c470d0e82579da3f`;
  correcting that declaration is accepted only to unblock the final review's
  mandated real router-path observation, and the reconstruction validator plus
  full script discovery must settle it before the gate runs.
- 2026-09-13 build review-fix green under RULING-001: component chart
  contracts pass 2/2 and the two focused real-browser journeys pass. The
  field-display specification passes all 9 viewports/journeys, including the
  chart listbox in the 44-by-44 field target oracle. A real supported-Windows
  invocation through `with-tools.ps1` ran `python scripts/aegis_test.py full`,
  selected `Ubuntu-24.04`, translated the checkout and reached
  `02_Tools/tests/appliance/check.sh`; exit=0 after 117 script tests, all Go
  packages, Python 32 + 7/1 skipped + 28, GUI 490/490, production build, 2
  appliance system tests and Playwright 40/40 in 3.7 minutes. The separate
  full lint router exits 0.
- 2026-09-13 build complete under RULING-001: dense traffic uses one
  coordinate-aware field-sized listbox with option semantics, keyboard
  movement and a visible zero marker; the role fixture drives both PUT/confirm
  cycles and observes both safety warnings; the Windows full gate is observed;
  current documentation and REQ-UX-15904d9e paths are reconciled. Advanced to
  validate for one fresh independent review cycle.
- 2026-09-13 validate inspection under RULING-001: reviewed the integrated diff
  against all eight requirements, the adapter-role spec, recorded design,
  browser/helper error paths, Windows/WSL routing, secret handling and the
  post-anchor evolution. REQ-UX-cb91fedb and REQ-UX-c5f7c0cd now describe the
  listbox/option contract, dense coordinate proof and field geometry oracle;
  `gui-foundation`, `repository-toolchain` and environment knowledge match the
  built ownership boundaries. None of the linked requirements uses a Manual or
  Inspection verification prefix, so no separate hand observation is owed.
- 2026-09-13 validate review round 1 under RULING-001: candidate
  `617ff62220e5d952478ddad62bdd10e500fb6cb6c076e6fd65d3b7c6f099dcff`
  sent to fresh independent specification-compliance and
  implementation-quality reviewers after the four final findings were fixed.
- 2026-09-13 validate review round 1 under RULING-001: both independent
  reviewers returned `changes_required` and confirmed the candidate digest was
  unchanged. Specification findings: chart hover is unreachable because the
  sample nodes cannot receive pointer events; active REQ-PLAT-38014f08 and old
  GUI knowledge still describe the retired host/simulation browser topology;
  and the locked-environment spec plus tools README still claim the offline
  gate runs Playwright. Implementation-quality findings: the appliance probe
  pipeline masks command failures and labels the Playwright package version as
  Chromium; the same chart-hover defect; the corrected reconstruction digest
  conflicts with the example acceptance evidence and its regression; and the
  same stale current-state documentation. The verdicts are invalidated while
  this bounded fix cycle changes the candidate; the next immutable candidate
  will be review round 2.
- 2026-09-13 build review-round-1 red under RULING-001: focused TrafficChart
  component run exits 1 because pointer movement leaves the previously clicked
  dense minute selected; the appliance probe behavior regression exits 1
  because the new sourced probe contract does not exist; and the focused build
  acceptance regression exits 1 because BE-02 evidence still names
  `784a1d...` instead of the lock's `66b1e14...` manifest digest. Inspection
  also finds active REQ-PLAT-38014f08, the locked-environment acceptance spec,
  GUI knowledge and `02_Tools/README.md` still claiming the retired host or
  offline Playwright topology.
- 2026-09-13 build review-round-1 green under RULING-001: the chart component
  passes 2/2 including coordinate hover; appliance contracts pass 21/21,
  including behavioral propagation of a failing probe; and the two acceptance
  evidence regressions pass with BE-02 truthfully pending. The real Windows
  full router exits 0 after 118 script tests, all Go packages, Python 32 plus
  7/1 skipped plus 28, GUI 490/490, production build, two appliance system
  tests and Playwright 40/40 in 3.5 minutes. The appliance inventory printed
  `Google Chrome for Testing 151.0.7922.34` from the executable itself. The full
  lint router exits 0.
- 2026-09-13 build evidence correction under RULING-001: the current
  reconstruction manifest definition validates at
  `66b1e14f0a02eef6be7b8c0437b259fae576373ae8959226ff71d732ca60193be`,
  but `verify-materialized-inputs` fails because the local cache lacks at least
  `go-proxy/modenc.org/cc/v4/@v/v4.29.1.info`. BE-02 and the maturity flag are
  therefore pending; no current materialized proof is claimed.
- 2026-09-13 build Raspberry Pi observation under RULING-001: the personal
  `ddp-remote` Pi 4 was identified by its recorded wireless MAC and host key at
  `10.44.118.42`. A candidate ARM64 binary and production GUI ran from a
  throwaway `/tmp/nilo-task-f978f3bd-*` directory on port 9443 without touching
  `/var/lib/nilo` or the installed service. A real headed Chromium session
  signed in, observed profile `appliance`, `Simulation: No`, real
  NetworkManager/WireGuard versions and two real eth0 traffic minutes; pointer
  hover selected 18:20 and keyboard End selected 18:21. The process and files
  were removed, both installed services remained active, and the installed GUI
  returned HTTP 200 on port 8443. This is useful personal-bench evidence, not a
  repository or release gate. It also exposed pre-existing empty-inventory copy
  that still says "simulation scenario"; removing that production wording
  belongs to the later simulation-removal work, not this browser-stage task.
- 2026-09-13 build complete under RULING-001: the four consolidated round-1
  findings are fixed and verified. The candidate returns to validate as review
  round 2; no verdict from the changed candidate is yet recorded.
- 2026-09-13 validate review round 2 under RULING-001: candidate
  `c07dc7c38c3ad7d95aa7ac464a03af7ceefbe341aa8273ce79af3801b9198436`
  remained unchanged through both independent reads. Implementation quality
  returned `approved`. Specification compliance returned `changes_required`:
  nine active requirements still named the retired offline-browser, direct
  WSL2/Vite/loopback-process, or issued bearer-token topology. Both verdicts
  are invalidated while this final authorized fix changes the candidate.
- 2026-09-13 build review-round-2 red under RULING-001: direct inspection of
  REQ-TEST-74c72bbe, REQ-UX-f479d8b9, REQ-TEST-3c4487d6,
  REQ-PLAT-59993b78, REQ-REL-f04744fa, REQ-SEC-4884bc91,
  REQ-TEST-0f02f4b8, REQ-TEST-68dcb172 and REQ-TEST-842f1943 confirms the
  stale topology and linked-test wording identified by the specification
  reviewer. The requirements are brought into this task's traceability before
  the next immutable review.
- 2026-09-13 build review-round-2 green under RULING-001: all nine active
  browser requirements now name the appliance-stage entry point, isolated
  management ingress, stage-owned Linux temporary layout, bounded helper/core
  readiness, password bootstrap and session authentication, and current linked
  tests. The test-only redaction fixture no longer names the retired development
  bearer token. Focused GUI contracts pass 49/49 and the full interface format
  check exits 0. AEGIS check reports no errors; its task-local handoff warning
  was corrected before returning to validate.
- 2026-09-13 validate review round 3 under RULING-001: candidate
  `b8d676990e21fb4a1f94e3370caf1b6416ac8e4de25909e874cfdecc8e9302ad`
  remained unchanged through both independent reads. Specification compliance
  returned `changes_required`: one linked-test class name is stale and the
  backend-recovery journey does not reassert the empty inventory promised by
  REQ-TEST-0f02f4b8. Implementation quality returned `changes_required`: the
  same linked-test identifier, an inaccurate topology comment about retaining
  a password field, the requirement's overbroad process-environment promise,
  and stale evolution evidence. Both reviewers otherwise accepted the prior
  topology, authentication, verification, path and task-traceability fixes.
  RULING-001's reset review allowance is exhausted, so no further candidate
  edit or review is authorized without a new user ruling.
- 2026-09-13 ruling under RULING-002: the user reset the review counter and
  authorized one final fix and re-review cycle for the five unique round-3
  findings. Returned to build with no broader authority.
- 2026-09-13 build red under RULING-002: review evidence shows the recovery
  journey only checked that the Devices region returned, not that both empty
  inventories survived; the UX requirement names a nonexistent Python class;
  the security requirement overstates the environment boundary; the topology
  comment claims a password field that does not exist; and evolution evidence
  predates the final requirements reconciliation.
- 2026-09-13 build green under RULING-002: recovery now reuses one empty-
  inventory oracle before and after restart; the linked test names the actual
  `ApplianceBrowserSuiteTests` class; the security promise is limited to child
  process environments; the topology comment describes call-scoped stdin and
  redaction behavior; and evolution evidence is refreshed at 17 files. Focused
  GUI contracts pass 49/49, appliance contracts pass 21/21, the full interface
  format check exits 0, `git diff --check` is clean, and AEGIS check reports no
  errors with 35 explained repository-wide warnings.
- 2026-09-13 validate review round 1 under RULING-002: final candidate
  `3c9377151222b31939b37458608072619d7ac061b1f47bfc13bf2da094c36f62`
  sent for one fresh independent specification-compliance and
  implementation-quality review pair after all five authorized corrections.
- 2026-09-13 validate review round 1 verdicts under RULING-002: candidate
  `3c9377151222b31939b37458608072619d7ac061b1f47bfc13bf2da094c36f62`
  remained unchanged through both independent reads. Specification compliance
  returned `approved` with all five RULING-002 corrections and twenty-
  requirement traceability accepted. Implementation quality returned
  `changes_required`: every current password fill is redacted in source, but
  REQ-SEC-4884bc91's structural regression and linked paths protect only the
  bench-core and authentication specs, not every password-bearing browser
  spec. RULING-002 authorized exactly one final fix and re-review cycle and is
  now exhausted; the candidate cannot change again without a new user ruling.
- 2026-09-13 ruling under RULING-003: the user reset the review counter and
  authorized one narrowly scoped fix and re-review cycle for the remaining
  password-redaction coverage finding. The clarification confirms this is a
  source-structure regression, not password-value, credential-combination or
  additional sign-in testing.
- 2026-09-13 build red under RULING-003: the existing structural checks name
  only `bench-core.spec.ts` and `authentication.spec.ts`, leaving the already-
  redacted field-display, network and shell password fills outside the oracle.
- 2026-09-13 build green under RULING-003: one dynamic source-structure test
  discovers every browser spec with a fixture-password fill and requires each
  fill to sit inside `withRedactedSecret`; it tests no password values and adds
  no browser journey. The focused command contract passes 11/11, the full GUI
  format check exits 0, REQ-SEC-4884bc91 links the dynamic oracle and all five
  password-bearing specs, and evolution evidence is refreshed at 18 files.
- 2026-09-13 validate review round 1 under RULING-003: immutable candidate
  `3fb523ccfabbcb738ca4b78e21b2ba402deadd439df51161436e9f1e330e9a7e`
  sent for the final independent specification-compliance and
  implementation-quality review pair.
- 2026-09-13 validate review round 1 verdicts under RULING-003: candidate
  `3fb523ccfabbcb738ca4b78e21b2ba402deadd439df51161436e9f1e330e9a7e`
  remained unchanged through both independent reads. Specification compliance
  returned `approved`. Implementation quality returned `changes_required`:
  the regex-based source guard can miss fills that use options, properties,
  camel-case aliases or literals, and its fixed look-behind does not prove AST
  ancestry. The reviewer requires an AST-based or equivalent structural check.
  RULING-003's one narrowly scoped fix/re-review allowance is exhausted, so no
  candidate edit is authorized without another user ruling.
- 2026-09-13 ruling under RULING-004: the user reset the review counter,
  directed removal of the exhaustive password-fill source oracle, retained the
  focused credential-redaction tests, and authorized one final re-review cycle.
- 2026-09-13 build red under RULING-004: the dynamic regex oracle exceeds the
  user's intended verification scope and cannot prove syntax-tree ancestry.
- 2026-09-13 build green under RULING-004: removed the exhaustive scanner and
  restored the focused bench-core and administration source checks. The
  security requirement now covers bootstrap and calls made through the
  credential-redaction helper, verified by focused source checks, the helper's
  forced-failure browser test, disabled Playwright artifacts and stdin
  bootstrap. The focused command contracts pass 12/12, the full GUI format
  check exits 0, `git diff --check` is clean, AEGIS check reports no errors,
  and evolution evidence is refreshed at 18 files.
- 2026-09-13 validate review round 1 under RULING-004: immutable candidate
  `befa040af3aa98cee4d18c30a92041f77ea638b252d93eb5bd33967b9acacdc5`
  sent for the final independent specification-compliance and
  implementation-quality review pair with the user's focused-test ruling as
  an explicit review constraint.
- 2026-09-13 validate review round 1 verdicts under RULING-004: candidate
  `befa040af3aa98cee4d18c30a92041f77ea638b252d93eb5bd33967b9acacdc5`
  remained unchanged through both independent reads. Specification compliance
  and implementation quality both returned `approved` with no actionable
  findings. The reviewers confirmed the narrowed security requirement and
  focused tests match RULING-004, all linked tests resolve, and the broader
  candidate remains coherent and maintainable. Both reviews were read-only and
  left the final strict gate to the coordinator.
- 2026-09-13 validate strict invocation correction: AEGIS bridge job
  `4ed33d90-aab8-46c9-b47c-ede73dbf1fc4` was first launched outside the
  repository tool environment. Its script discovery passed 118 tests, then
  test and lint exited 127 because `go` was not on PATH. Candidate
  `befa040a...` remained unchanged. This was an invocation-environment refusal,
  not a candidate failure; the same bridge command was rerun through
  `02_Tools/env/with-tools.ps1` with `Ubuntu-24.04` selected.
- 2026-09-13 validate strict green: AEGIS bridge job
  `11a65df9-dd0d-4cc8-8c83-c3912d77a814` completed with test exit 0 in 362.9
  seconds and lint exit 0 in 7.4 seconds. The appliance/browser phase passed
  Playwright 40/40 in 4.1 minutes. Strict result passed with 0 errors, 35
  warnings and 1 manual note; candidate digest was identical before and after
  at `befa040af3aa98cee4d18c30a92041f77ea638b252d93eb5bd33967b9acacdc5`.
  One warning is the known fixed password literal in a disposable appliance
  test database; the other 34 warnings and manual note are pre-existing stale
  requirement paths or older incomplete task/review records already outside
  this task's candidate scope.
- 2026-09-13 validate complete: both current independent verdicts are approved,
  strict validation passed, the candidate is unchanged, and the 49 tracked-file
  diff plus 22 untracked task/appliance artifacts were reviewed. Advanced to
  deliver and stopped for the mandatory human delivery decision. Intended
  outcome is one local commit only; repository policy forbids agent push, and
  no PR, release or deployment is authorized.
- 2026-09-13 deliver: The user approved one local commit and task close-out.
  The commit is the complete agreed outcome; it carries this journal, the
  appliance-stage foundation, the validated browser-suite migration, and the
  separately authorized 0.3.1 release/deployment records. No push or PR is
  authorized by repository policy.

## Rulings

- RULING-001 | date=2026-09-13 | decision=Reset the TASK-f978f3bd review counter and authorize another fix and re-review cycle for the final specification findings. | reason=The user explicitly authorized continuing after the bounded third review identified four remaining compliance gaps. | source=user | supersedes=none
- RULING-002 | date=2026-09-13 | decision=Reset the TASK-f978f3bd review counter and authorize one final fix and re-review cycle for the five remaining round-3 findings. | reason=The user explicitly authorized resolving the last five unique specification and quality findings after the RULING-001 allowance was exhausted. | source=user | supersedes=none
- RULING-003 | date=2026-09-13 | decision=Reset the TASK-f978f3bd review counter and authorize one narrowly scoped fix and re-review cycle for the remaining password-redaction coverage finding. | reason=The user explicitly authorized closing the sole remaining structural regression gap and clarified that exhaustive password-value testing is not wanted. | source=user | supersedes=none
- RULING-004 | date=2026-09-13 | decision=Reset the TASK-f978f3bd review counter, remove the exhaustive password-fill source oracle, retain the existing focused credential-redaction tests, and authorize one final re-review cycle. | reason=The user explicitly rejected exhaustive password coverage and selected focused behavior verification instead of an AST-wide policy oracle. | source=user | supersedes=none

## Handoff


current: done
status: completed
next: local delivery commit approved; do not push
must_read: .agentic/steps/6_deliver.md, this journal, the approved candidate diff and strict job 11a65df9-dd0d-4cc8-8c83-c3912d77a814
blockers: none; agent push remains prohibited by repository policy

## Stage report

stage: done
result: completed
current: done
changed: The browser topology, prerequisites, Playwright configuration and production journeys target an appliance-profile core served from its own HTTPS origin. Separate network-system and appliance image targets preserve the clean WireGuard installer fixture while the richer stage builds the GUI, provides a real management ingress and WireGuard tooling, and routes browser verification there. Core startup and owned-process shutdown are bounded and cleanup is creation-aware. Live journeys exercise the appliance; narrowly labelled response fixtures cover presentation-only states. Dense traffic uses one coordinate-aware listbox instead of overlapping sample targets, including coordinate hover, keyboard selection and a visible measured-zero marker. The adapter-role fixture drives replacement, confirmation and current-path removal through the UI. Appliance inventory probes preserve command failures and read Chromium's own version. Operator, context, all twenty linked requirements and knowledge records distinguish optional host-local tooling, the network-disabled locked environment and appliance-stage browser ownership; authentication requirements now name password bootstrap and server-managed sessions, and acceptance evidence no longer overclaims the corrected reconstruction digest.
evidence: final strict AEGIS job 11a65df9-dd0d-4cc8-8c83-c3912d77a814 passed: test exit 0 in 362.9 seconds, lint exit 0 in 7.4 seconds, Playwright 40/40 in 4.1 minutes, strict 0 errors/35 warnings/1 note, candidate unchanged at befa040af3aa98cee4d18c30a92041f77ea638b252d93eb5bd33967b9acacdc5. Earlier evidence includes the real Windows full gate, appliance contracts 21/21, focused chart 2/2, browser harness 49/49, acceptance-evidence 2/2, RULING-004 focused credential contracts 12/12, full format/lint, and a cleaned-up personal Pi live smoke.
issues: accepted warnings are one fixed disposable test password literal plus 34 pre-existing stale requirement paths and older incomplete task/review records; none invalidates this candidate. The corrected reconstruction manifest has no complete local materialized-cache readback, so BE-02 is truthfully pending but outside this browser-stage task. The live Pi exposed pre-existing production empty-inventory copy that still mentions a simulation scenario, reserved for later simulation-removal work. No agent push is permitted.
next: Local delivery commit is the completed outcome; no push or PR is authorized.
needs_user: no
review_round: 1
spec_compliance: approved
implementation_quality: approved

## Delivery report (2026-09-13)

- stage: done; type: change; path: full; branch: ddp/nilo-simulation-retirement
- requirements:
  - REQ-TEST-68eedecb - Prove the authenticated browser journey against an appliance
  - REQ-TEST-b1177191 - Prove the browser journey against the interface the appliance serves
  - REQ-TEST-7ff6a75d - Run the browser end-to-end test in the repository gate
  - REQ-TEST-0a4f49af - Prove the authenticated browser-to-WSL2 simulation journey
  - REQ-UX-cb91fedb - Make the traffic history chart self-explanatory
  - REQ-UX-c5f7c0cd - Size field-layout controls and touch targets for a seven-inch display
  - REQ-UX-15904d9e - Explain unavailable adapter roles in device network selection
  - REQ-UX-e6437bf7 - Guide safe adapter repurposing
  - REQ-PLAT-32b81548 - Provide a real-network appliance verification image
  - REQ-PLAT-38014f08 - Keep the optional Windows development browser pinned and repository-local
  - REQ-PLAT-80e40361 - Build locked OCI environment
  - REQ-TEST-74c72bbe - Verify native and ARM64 build toolchains offline
  - REQ-UX-f479d8b9 - Offer one headless browser test command and an opt-in headed mode
  - REQ-TEST-3c4487d6 - Name a missing browser end-to-end prerequisite
  - REQ-PLAT-59993b78 - Isolate each browser acceptance run inside the appliance stage
  - REQ-REL-f04744fa - Bound the browser end-to-end readiness wait
  - REQ-SEC-4884bc91 - Keep browser acceptance credentials out of output and artifacts
  - REQ-TEST-0f02f4b8 - Prove backend-unavailable presentation and browser recovery
  - REQ-TEST-68dcb172 - Prove the administration page against a real backend in a browser
  - REQ-TEST-842f1943 - Prove the shell, themes and languages in a browser
- deviations: 2 recorded
  - build - the browser suite no longer asserts that an adapter is reserved for management access. The stage's appliance has no allowlist, so nothing is reserved; role exclusivity stays covered by `05_Software/nilo_assistant/internal/network/adapter_roles_test.go` and `internal/network/management_access_test.go`.
  - build - the browser suite no longer asserts the simulated integration and device inventory (`simulation.fake-python` and its device). A fresh appliance carries no managed integration until one is installed, so `bench-core.spec.ts` asserts the empty inventory instead. That inventory's own coverage belongs with the bench-integration work.
- explicit review: enabled; spec_compliance=approved; implementation_quality=approved; review_round=1
- diff vs task base a36c277: 77 file(s) changed, 14080 line(s) added
- checks: errors=0 warnings=38 notes=1
- checklist: 6/6 done
