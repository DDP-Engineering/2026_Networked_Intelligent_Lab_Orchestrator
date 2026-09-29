# Go to Python Backend Migration Plan

**Project:** NILO - Networked Intelligent Lab Orchestrator\
**Date:** 2026-09-28\
**Status:** Proposed implementation plan; planning only\
**Target:** Python Bench Core, CLI and privileged helper; FastAPI HTTP boundary;
uv dependency/runtime management; Ruff linting and formatting\
**Integration scope:** Camera and USB Serial. Sigrok-specific migration is excluded.\
**GUI:** Existing React/TypeScript interface retained\
**Delivery targets:** Ubuntu 24.04 LTS `amd64` and Raspberry Pi OS based on
Debian 13 `arm64`

## 1. Objective and scope

Replace the Go backend with a maintainable Python implementation while retaining
the application contracts covered by this migration. Reduce language-toolchain
duplication and present developers with a small, consistent command surface.
FastAPI provides HTTP routing and validation; ordinary Python services own
orchestration, authorization, approvals, persistence and recovery.

**STATED:** The user requests a saved migration plan using Python, uv and Ruff.
The preceding backend discussion supports Python with FastAPI. The user excludes
the Sigrok integration from scope.

**OBSERVED:** Go currently implements the Bench Core, `nilo` CLI and privileged
`nilo-nethelper`. Python Camera/Serial integrations and the SDK already use
`pyproject.toml`, `uv.lock`, pytest and Ruff. React/TypeScript owns the browser
interface. See the repository evidence in section 12.

**PROPOSED:** Migrate the complete Go execution surface in bounded work packages,
with one active backend per running appliance and a single final Python backend.
Keep the GUI, integration process boundary and privileged-helper separation.
Use the current Go implementation as a temporary test oracle, then retire it
after the replacement passes the agreed gates.

In scope:

- Bench Core services, HTTP API, MCP endpoint, CLI and build/version reporting.
- SQLite persistence, identity, policy, approvals, activity and retained artifacts.
- Integration package management, supervision, bounded scheduling and Camera/Serial
  behavior currently owned by Go. Existing Python runners remain separate processes.
- Networking, discovery, commissioning, recovery, power and the privileged helper.
- Python project organization, uv locks, Ruff rules, tests and type checking.
- Required development, CI and packaging adaptations and removal of obsolete Go tooling.

Out of scope:

- Sigrok integration porting, feature work, redesign and dedicated acceptance.
  Do not count Sigrok as a migrated or verified integration. Its exclusion is not
  permission in this planning task to delete current product code. PY-01 must
  record its disposition in the replacement catalogue before cutover; generic
  protocol preservation alone does not establish that it works with the new core.
- Replacing React/TypeScript, adopting NiceGUI or HTMX, or removing Node.
- New Camera/Serial capabilities, implementation of the Modbus plan, distributed
  services, Redis/Celery, a database-engine change or a new deployment platform.
- Production implementation, environment installation, deployment, Git changes,
  release publication or version assignment during this planning request.

All architecture choices below are proposals for implementation framing. Writing
this plan does not authorize the migration or destructive state changes.

## 2. Preserve behavior before changing implementation

PY-01 produces a coverage matrix for every in-scope route, command, MCP tool,
background service and helper operation. It records actor, permission, policy,
inputs, outputs, limits, persistence, failure behavior and a verification case.
Existing requirements and tested behavior are the authority.

| Boundary | Migration contract and acceptance evidence |
|---|---|
| HTTP `/api/v1` | Preserve methods, paths, permission declarations, status/error envelopes, JSON shapes, pagination, streaming and quotas. Explicitly handle omitted versus null fields, strict booleans/numbers, UTC timestamps and validation failures. FastAPI defaults are adapted to the established API. |
| Identity and policy | Preserve Administrator/Operator/Viewer, owner-role/token-scope intersection, browser session/CSRF checks, password policy and Argon2id verification. Permission to request and approval to execute remain independent decisions. |
| Approvals | Match immutable policy snapshots, canonical request bytes, digest binding, expiry and one-use semantics. Cross-language vectors include key ordering, Unicode and rejected number spellings. |
| CLI and MCP | Preserve in-scope command/tool names, options, output, exit codes, transport and refusal behavior. CLI/MCP call the same services as HTTP. Local administration retains its separate authority and startup behavior. |
| Persistence | Keep SQLite schema, migration integrity, transactions and artifact layout unless a separately approved change requires otherwise. Prove rollback on failed writes, startup reconciliation and consistent audit results. |
| Integrations | Preserve `nilo.integration/1`, correlated NDJSON, manifest/schema validation, verified file inventories and executable-plus-argument execution. Camera/Serial retain explicit installation/enablement, device ownership and bounded sessions. |
| Jobs and shutdown | Preserve the bounded queue, current two executing workers, resource serialization, absolute deadlines, cancellation, terminal states and cleanup before a worker slot is reused. Restart must not replay an uncertain instrument mutation. |
| Networking/helper | Preserve preview, acknowledgement, confirmation, durable-before-effect ordering, adapter-role exclusion, rollback and independent recovery. Helper operations stay typed, authenticated and restricted. |
| Appliance startup | Preserve explicit simulation/appliance profiles, TLS identity, commissioning state, discovery, local-status listener, maintenance and power controls. Production starts unprivileged. |

The GUI must pass against the real Python backend. A successful mock UI or
automatically generated OpenAPI schema does not establish behavioral parity.

Development state is disposable under the repository policy. Use fresh isolated
test databases while proving existing schema behavior. Do not add a Go-to-Python
data converter or promise production upgrade compatibility. If a real dataset
must survive cutover, obtain an explicit preservation decision and add the
required backup, upgrade and rollback evidence before deployment.

## 3. Proposed architecture

```text
Existing React browser    nilo CLI    MCP client
          |                  |           |
   FastAPI HTTP adapter   CLI adapter   MCP adapter
          +------------------+-----------+
                             |
                 Python application services
        identity / policy / jobs / integrations / networking
                    activity / artifacts / lifecycle
                             |
               SQLite repositories and supervised runners
                             |
            Typed local IPC to a separate privileged helper
                             |
        NetworkManager / Avahi / nftables / GPIO / power
```

- Keep route functions small: parse, authorize, call a service, serialize.
  Pydantic models describe boundary data; they do not replace domain invariants.
- Use explicit composition and small modules. Domain services import neither
  FastAPI nor HTTP request objects. Share behavior across all interfaces.
- Keep the core and helper separately launched with separate permissions and
  narrowly scoped dependencies. The helper does not host FastAPI.
- Propose one ASGI worker initially. Multiple HTTP worker processes must not
  duplicate schedulers, hardware leases, startup reconciliation or housekeeping.
- Own background services through an explicit startup/shutdown lifecycle. Use
  `asyncio` for bounded I/O and supervised subprocesses; keep blocking DB, D-Bus,
  filesystem and device calls off the event loop through controlled adapters.
  Bound thread/process concurrency and serialize database access deliberately.
- FastAPI `BackgroundTasks` is not the job manager. Job state, admission,
  deadlines, cancellation and restart recovery belong to core services.
- Keep expensive capture/processing in supervised integration processes.
  Measure throughput and memory; do not assume Python threads replace Go
  concurrency for CPU-bound work.
- Preserve helper-owned signing-key custody and pre-HTTP management admission.
  Prove that the selected ASGI/TLS composition supports those contracts before
  adopting a standard Uvicorn launch command. Loading the appliance private key
  into the core or substituting an unauthenticated proxy is not an acceptable shortcut.

## 4. Python, uv and project organization

Start with the repository's pinned CPython and uv from
[`02_Tools/toolchain.versions`](../02_Tools/toolchain.versions). At planning time
these are CPython `3.12.10` and uv `0.11.25`; existing Python projects pin Ruff
`0.16.5`. Reuse those pins for initial feasibility. Select newer versions only
through a reviewed toolchain change with verified archives and target evidence.
FastAPI, the ASGI server, Python MCP SDK, D-Bus and cryptography dependencies
require exact version/closure selection in PY-02, not guesses in this plan.

Proposed development layout:

```text
05_Software/
  pyproject.toml                 # development workspace and shared tools
  uv.lock                        # one workspace dependency resolution
  core-python/                   # temporary replacement location
    pyproject.toml
    src/nilo_core/
      application/  identity/  policy/  store/  integrations/
      network/  activity/  artifacts/  httpapi/  mcp/  cli/
    tests/
  nethelper-python/
    pyproject.toml
    src/nilo_nethelper/
    tests/
  integrations/                 # Camera, Serial and SDK workspace members
```

After cutover, move the replacement to the canonical `05_Software/core/` path
and retire its temporary name. The helper may remain a separate Python package.
Existing out-of-scope Python packages need not become workspace members.

- Use explicit uv workspace members for Core, helper, SDK, Camera and Serial.
  Verify that their Python constraints, editable sources and dependency pins
  resolve together before replacing independent locks. Avoid duplicate canonical
  locks for the same in-scope project graph.
- A shared development lock does not imply a shared production environment.
  Retain runner isolation and package-specific runtime dependency subsets.
  The privileged helper receives only its own minimal runtime dependencies.
- Keep downloaded runtimes, virtual environments and caches ignored under
  `02_Tools/`, separated by platform. Configure uv's Python, cache and project
  environment locations explicitly; avoid a shared Windows/WSL `.venv`.
- Use `uv sync --locked` and `uv run --locked` for normal reproducible workflows.
  Acceptance uses offline operation with automatic Python downloads disabled.
  Missing dependency/interpreter inputs produce a clear failure, not a fetch.
- Pin `uv_build` or another justified build backend, including build dependencies.
  Editable installs are for development. Delivery uses built wheels and a verified
  target interpreter; never distribute a copied developer virtual environment.
- Preserve one project-level `VERSION`. Generate Core/helper wheel metadata and
  CLI/HTTP version reporting from it and verify all required mirrors. Do not
  introduce independent component release versions.
- Keep dependencies modest: start with Python's SQLite support behind repositories;
  introduce an ORM or additional queue service only for a demonstrated requirement.

## 5. Ruff, typing and tests

Use one shared Ruff configuration with narrow, documented project overrides.
Begin with existing Python conventions: line length 100 and the repository's
required prologues/documentation from
[`Code_Style.md`](../01_Documentation/Code_Style.md). Set the new core's Ruff
target to the selected Python baseline without silently changing the SDK's
supported Python range.

Propose `E4`, `E7`, `E9`, `F`, `I`, `UP`, `B` and applicable `ASYNC` rules first.
Check formatter compatibility and apply fixes only within migrated modules.
Do not reformat unrelated integration code as part of establishing the baseline.

Ruff lints and formats; it is not a type checker. Propose a pinned mypy check
for Core/helper contracts, with minimal documented exceptions for untyped
platform bindings. Exact dependency/target feasibility belongs to PY-02.

Illustrative commands inside the prepared workspace, not commands to run now:

```sh
uv sync --locked --offline --no-python-downloads
uv run --locked --offline --no-python-downloads ruff check core-python nethelper-python
uv run --locked --offline --no-python-downloads ruff format --check core-python nethelper-python
uv run --locked --offline --no-python-downloads mypy core-python/src nethelper-python/src
uv run --locked --offline --no-python-downloads pytest core-python/tests nethelper-python/tests
```

Repository wrappers supply exact package/group selection and paths. Add Camera,
Serial and existing mandatory GUI/tooling checks to the full gate. Do not claim
these proposed commands replace the current repository validation suite.

Port tests by the behavior they protect, not by translating every Go test line.
Keep existing black-box fixtures where reusable. Use deterministic clocks and
fake adapters for unit tests, real child processes for supervision, real SQLite
for transaction/restart tests and disposable Linux fixtures for privileged work.
Add malformed input, overload, permission refusal and cancellation cases before
the corresponding implementation. Maintain the Go oracle until equivalent
Python evidence covers each in-scope behavior.

## 6. Migration work packages

Each package is a separately framed implementation task. Its entry evidence and
acceptance conditions must be complete before dependents rely on it. Implementation
uses AEGIS; repository/package operations follow their own procedures.

| ID | Work and deliverable | Dependencies | Acceptance condition |
|---|---|---|---|
| PY-01 | Inventory Go commands, routes, tools, services, schema, helper protocol and tests. Create the parity/retirement matrix; record Sigrok disposition and interactions with pending plans. Establish resource/performance baselines and cutover policy. | None | Every in-scope behavior has an owner and verification case; exclusions are explicit; no undocumented feature loss. |
| PY-02 | Feasibility slice: uv workspace, pinned tools/dependencies, Ruff/mypy/pytest, FastAPI skeleton, SQLite transaction, Camera/Serial runner handshake, Python helper IPC/D-Bus and helper-backed TLS/management admission. Resolve offline amd64/arm64 inputs. | PY-01 | Slice passes in the canonical network-disabled environment; ARM64 runtime/wheel availability and privileged/TLS approach are demonstrated or migration is blocked with evidence. |
| PY-03 | Port domain models, validation, configuration, SQLite migrations/repositories, canonical JSON/digests, artifact confinement and version reporting. | PY-02 | Fresh schema, migration-digest rejection, transactional failure, path traversal and cross-language approval vectors pass. |
| PY-04 | Port identity, sessions/tokens, roles/scopes, password policy, approvals, audit/activity and quotas/retention. | PY-03 | Shared-service tests prove refusal paths, CSRF/session behavior, approval binding, atomic writes and secret redaction. |
| PY-05 | Port verified package lifecycle, integration supervision, persisted jobs, worker/resource admission, deadlines and cancellation. | PY-04 | Real-process tests prove bounded I/O, timeout/kill/reap, cleanup, queue limits, concurrent device exclusion and restart reconciliation without uncertain-write replay. |
| PY-06 | Port Camera/Serial Core-owned records, live sessions, device monitoring, preview/export/recording and retention services. Keep existing Python runners. | PY-05 | Existing Camera/Serial behavior passes simulation and real-runner contract tests, including unplug, slow client, disconnect and simultaneous use. No Sigrok-specific work. |
| PY-07 | Port HTTP, CLI and MCP adapters. Match HTTP error handling/serialization and CLI output/exit codes; serve the retained built GUI. | PY-04; PY-05/06 for managed operations | The parity matrix passes across interfaces; permissions/policy use shared services; GUI acceptance targets the real Python backend. |
| PY-08 | Port privileged helper and Core networking composition: NetworkManager, WireGuard, nftables, Avahi/SSDP, commissioning, TLS identity, GPIO recovery, maintenance and power. | PY-02 feasibility; PY-03/04; PY-07 for public flows | Disposable Linux topology tests prove adapter exclusion, durable intent, rollback after crash, timer-owned recovery, key custody and management admission; hardware gates remain explicit. |
| PY-09 | Integrate production composition, startup/shutdown ownership, discovery/CLI flows, simulation administrator and all in-scope capabilities. Run behavioral/performance comparison. | PY-06/07/08 | One unprivileged core owns services; bounded shutdown/restart and real-GUI simulation acceptance pass; agreed resource limits are met. |
| PY-10 | Adapt setup/dev/test/build/package entry points, canonical OCI image, dependency escrow and Gitea CI to the locked Python stack. Update exact-target packaging and systemd launch paths. | PY-09; dependency preparation starts in PY-02 | Clean checkout restores verified inputs; network-none checks/build/package preparation pass; resulting exact-target installations run offline. Publication/backup maturity is reported separately. |
| PY-11 | Cut over canonical paths; remove superseded Go production code, tests with proven replacements, compiler/cache/bootstrap branches and version mirror. Reconcile docs, AEGIS command/layout configuration and pending plans. | PY-10; all cutover gates | No Go runtime/build dependency remains for in-scope product/tooling; fresh setup and full CI pass; no temporary backend fallback or duplicate public command surface remains. |

Recommended first implementation request: **PY-01**, followed by the small
**PY-02** feasibility slice. Do not begin a large mechanical translation before
dependency, TLS/helper and resource feasibility are established.

## 7. Development tooling and CI

Aim for one documented command surface under `02_Tools/`:

| Command | Responsibility |
|---|---|
| `setup` | Verify platform; restore pinned runtime/dependencies from verified inputs; report missing inputs. Host installation remains explicit. |
| `dev` | Start/stop/check the real simulation backend and existing GUI using the established development credentials and profile fences. |
| `test` | Run focused checks or the full canonical gate; choose platform fixtures explicitly. |
| `build` | Produce Python wheels, version/provenance metadata and the retained GUI bundle. |
| `package` | Assemble exact-target release payloads/installers/images from complete verified inputs. |

The concrete launcher name is chosen in PY-10 after surveying current entry
points. Reuse existing platform wrappers and internal helpers rather than adding
five parallel families of scripts. Keep substantive logic in Python; PowerShell
and POSIX wrappers handle host invocation and explicit environment selection.

Retain native Windows GUI development with Linux backend/runners under WSL2.
SQLite, sockets and runtime artifacts stay on the Linux filesystem. Production
appliances remain Linux; inventory portable Windows CLI expectations before
retiring Go cross-build checks and replace them with equivalent Python checks.

Retain the Gitea workflow's canonical network-disabled validation container and
artifact upload. Replace Go checks with Python Core/helper checks only when the
parity matrix proves coverage. Keep Node/TypeScript checks for the retained GUI.
Continue uploading logs, structured results and built GUI output; add Python
wheel/provenance evidence. Replace image references/digests through a new verified
generation, never by relabeling the old image or editing its evidence in place.

## 8. Offline packaging and platform acceptance

uv's lockfile describes resolution; it does not alone supply a complete offline
installer. The dependency closure must include:

- CPython distributions and uv for each supported development/build host.
- Core/helper/Camera/Serial runtime wheels, transitive dependencies and any build
  backend inputs; dev tools are a separate subset.
- Native libraries and OS packages needed by crypto, D-Bus, device bindings or
  selected wheels for each exact Linux/architecture target.
- GUI build inputs and required appliance integration/systemd files.
- Versions, licences, checksums and provenance for all reconstructed inputs.

Prefer prebuilt compatible wheels; source-only dependencies require a pinned,
tested target build process. An amd64 Python test run cannot substitute for
ARM64-native dependency installation and execution. Do not assume removing Go
also removes all C/native toolchains.

Delivery includes an immutable target Python runtime and package-specific
environments assembled for their installation paths; account for virtualenv
shebang/path binding. The installed appliance launches local entry points without
contacting registries, resolving dependencies or requiring development tools.
Avoid running `uv sync` on first boot.

Keep the existing one-release-per-target model, complete integration payloads
for the accepted catalogue, exact-target offline installers and writable Pi image.
Publication, archive escrow backup/readback, and installation are separate
authorized repository/delivery operations with their existing evidence rules.

## 9. Cutover gates and rollback

| Gate | Required evidence |
|---|---|
| G1: Scope/dependencies | Accepted parity/exclusion matrix, exact pins, complete closure and passing feasibility slice. |
| G2: Behavioral parity | Every in-scope HTTP/CLI/MCP/service/helper row verified; existing relevant tests replaced or retained with reasons; Camera/Serial accepted. |
| G3: Concurrency/recovery | Burst/overload, cancellation, child failure, DB failure, core/helper restart and uncertain mutation handling demonstrated; no leaked process/device leases. |
| G4: GUI/simulation | Existing browser against real Python simulation core; authentication, approvals, Camera/Serial sessions and networking flows verified. |
| G5: Appliance fixtures | Privileged disposable Linux tests, helper-backed TLS and interface admission, independent network rollback/recovery, commissioning and offline clean-target installation pass. |
| G6: Performance/targets | Measured startup/RSS/CPU, job admission, serial throughput/latency, preview load and API responsiveness against PY-01 thresholds; ARM64-native and physical Pi Camera/Serial/network/GPIO checks reported. |
| G7: Reproducibility/cleanup | Fresh-checkout setup, network-none builds/tests, target packages, source-lock/image provenance and final Gitea run pass; Go removal and current-state documentation reconciled. |

Record unavailable hardware checks as `not_run`, with their owner and required
fixture. Simulation never closes physical acceptance. Numerical resource and
latency thresholds are set from measured baselines in PY-01; this plan does not
invent acceptance numbers.

Develop the replacement alongside the Go test oracle using isolated disposable
state; never have both backends own the same database, helper or device. Before
cutover, a failed gate leaves Go as the active implementation. At accepted
cutover, Python becomes the sole product backend and temporary comparison code
is removed.

Git history can restore the old source when explicitly requested. Deployment
rollback must restore a tested complete software/runtime set and compatible
state; merely pointing systemd at an old executable is insufficient. Any
preserved real dataset requires an approved backup/restore policy before cutover.

## 10. Risks and mitigations

| Risk | Planned response |
|---|---|
| Framework validation changes HTTP behavior | Explicit boundary models/exception handlers and black-box contract vectors. |
| JSON/digest differences change approvals | Compare canonical bytes and digests before relying on Python authorization. |
| Blocking calls or duplicate ASGI workers break scheduling | Single service owner, bounded I/O adapters and saturation/shutdown tests. |
| Python memory/CPU differs on Pi | Measure early, then verify representative workloads on the target hardware. |
| TLS/private-key or management admission cannot use standard ASGI setup | Prove the server/helper composition in PY-02; stop migration before bulk porting if unresolved. |
| Native Python dependencies complicate offline ARM64 installation | Resolve complete target wheel/OS closures early and test installs natively. |
| Workspace hides integration dependency/isolation problems | Resolve together for development, build/test separate production subsets. |
| New command wrappers add more scripts | Consolidate current entry points; retain only justified platform adapters and internal helpers. |
| Active feature plans still refer to Go paths | Update their implementation seams at cutover while preserving unrelated work and proposed behavior. |
| Sigrok exclusion is mistaken for verified support | Explicit catalogue disposition at PY-01; no Sigrok acceptance claim in migration reports. |

## 11. Decisions to settle during implementation framing

No decision blocks delivery of this proposed plan. Before implementation/cutover,
record these choices rather than silently making them:

1. Accept the full scope including the Python privileged helper, retained React
   GUI and Camera/Serial acceptance. Record the excluded Sigrok catalogue disposition.
2. Choose exact framework, MCP SDK, D-Bus/crypto and type-checker dependencies
   after the PY-02 prototype; record the ASGI/TLS/admission mechanism.
3. Accept workspace members, package/runtime isolation and the final helper path.
4. Set measurable resource/performance budgets and identify physical ARM64 fixtures.
5. Identify any real data that must survive. The default development policy is
   disposable state; production preservation/upgrade needs explicit scope.
6. Sequence pending Camera/network/build work and the separate Modbus plan around
   the parity baseline. Do not mix new features into the language port.

## 12. Evidence and authoritative references

Repository sources inspected for this proposal:

- [Project responsibilities](../.agentic/context/project.md),
  [Bench Core](../.agentic/knowledge/bench-core-foundation.md) and
  [appliance networking](../.agentic/knowledge/appliance-network.md).
- [Go dependencies](../05_Software/core/go.mod),
  [CLI](../05_Software/core/cmd/nilo/main.go),
  [HTTP boundary](../05_Software/core/internal/httpapi/server.go),
  [MCP adapter](../05_Software/core/internal/mcp/server.go),
  [job scheduler](../05_Software/core/internal/bench/scheduler.go),
  [approval canonicalization](../05_Software/core/internal/policy/model.go) and
  [helper protocol](../05_Software/core/internal/nethelper/protocol.go).
- [Python SDK configuration](../05_Software/integrations/sdk/python/pyproject.toml),
  [Camera configuration](../05_Software/integrations/camera/pyproject.toml) and
  [Serial configuration](../05_Software/integrations/serial/pyproject.toml).
- [Pinned tools](../02_Tools/toolchain.versions),
  [environment guide](../.agentic/context/environment.md),
  [development/simulation contract](../06_SoftwareDocu/Development_and_Simulation_Flow.md),
  [code style](../01_Documentation/Code_Style.md),
  [packaging plan](Packaging_and_Delivery_Plan.md) and
  [current CI](../.gitea/workflows/ci.yaml).

Primary framework documentation:

- [FastAPI features](https://fastapi.tiangolo.com/features/): HTTP model validation,
  dependency injection and generated API documentation support the adapter choice.
- [uv workspaces](https://docs.astral.sh/uv/concepts/projects/workspaces/):
  shared resolution and member selection support the proposed development workspace.
- [uv locking/syncing](https://docs.astral.sh/uv/concepts/projects/sync/) and
  [command reference](https://docs.astral.sh/uv/reference/cli/): locked/offline
  operation and Python-download controls support reproducible environment restoration.
- [Ruff linter](https://docs.astral.sh/ruff/linter/),
  [formatter](https://docs.astral.sh/ruff/formatter/) and
  [FAQ](https://docs.astral.sh/ruff/faq/): Ruff supplies linting/formatting;
  separate type checking remains a proposed quality gate.
