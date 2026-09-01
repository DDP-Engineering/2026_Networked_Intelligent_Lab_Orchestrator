# Repository-local toolchain

## Purpose

The toolchain component gives a fresh supported checkout pinned Go, uv,
managed Python, Node.js, and npm runtimes without persistent host changes.
Downloaded runtimes and caches remain ignored so they can be populated online
or carried in an offline development kit without bloating source history.

## Public interface

Developers run `bootstrap.ps1` or `bootstrap.sh` for one host, or
`bootstrap-development.ps1` to prepare both Windows and a selected WSL2
distribution from one checkout. `setup-wsl.ps1` keeps host installation and
conversion opt-in, while `doctor-wsl.ps1` proves the distribution, repository
mapping, and Linux tools. `bootstrap-frontend.ps1` restores the locked browser
GUI dependencies, including from a populated offline cache. Commands run
through `with-tools` or the Go-only compatibility wrapper.
`toolchain.versions` is the authority for versions, platform archive URLs,
and SHA-256 checksums.

## Key behaviors

- `REQ-PLAT-978855a5` - checksum-verifies pinned repository-local tools and avoids persistent PATH or system installations.
- `REQ-PLAT-e1a2a591` - restores the locked browser frontend and emits static
  production assets without adding a Node.js runtime to the appliance.
- `REQ-PLAT-93e1b194` - supplies the Go runtime used by portable Linux ARM64, Linux x86_64, and Windows x86_64 compilation.
- `REQ-PLAT-bdf0cbdb` - isolates tools, caches, managed Python, and generated
  Python environments by host platform.
- `REQ-PLAT-055d77ba` - verifies WSL2 readiness and requires explicit authority
  before requesting installation or conversion.
- `REQ-FUNC-6d8e811c` - bootstraps Windows and WSL2 tools through one Windows
  entry point.
- `REQ-TEST-e311d3bc` - diagnoses the WSL2 kernel, checkout mapping, and Linux
  toolchain from Windows.
- `REQ-TEST-031b1658` - routes frontend format, lint, unit-test, and build
  commands through the repository validation entry point on Windows.
- `REQ-FUNC-d05e1128` - offers `start` and `status` for the local development
  topology from one Windows entry point, with explicit distribution, scenario
  and port options.
- `REQ-FUNC-d161146d` - reports a successful start only once both components
  answer, and names the silent one on a readiness timeout.
- `REQ-FUNC-645b9d0c` - starts only the components that are not already
  answering.
- `REQ-UX-4f1d3604` - classifies the topology as stopped, healthy, partially
  running or port conflict, from endpoint answers rather than processes.
- `REQ-UX-1f854bdc` - prints the endpoints, the selection, the token-issuance
  command, the status command, and how to stop each component.
- `REQ-UX-09865d40` - opens a browser only when explicitly asked to.
- `REQ-SEC-e0462d41` - never writes a plaintext bearer token to its output or
  to any file.
- `REQ-REL-8c82d667` - reports a port held by an unowned process and starts
  nothing, without ever signalling that process.
- `REQ-PLAT-5360be1a` - keeps Bench Core runtime state on the WSL Linux
  filesystem, refusing a Windows drive mount.
- `REQ-PLAT-022e39b0` - reports each outcome through its own exit code.
- `REQ-TEST-daa9a99d` - exposes the WSL, endpoint, port, launcher and browser
  boundaries as injectable options so tests open no window and bind no port.

## Architecture

```mermaid
flowchart TD
    manifest[("toolchain.versions")]
    win_bootstrap(["Windows bootstrap"])
    linux_bootstrap(["Linux bootstrap through WSL2"])
    wsl_setup(["WSL2 setup / check"])
    combined(["combined development bootstrap"])
    cache[("platform-keyed ignored cache")]
    runtime[("platform-keyed ignored runtime")]
    frontend_bootstrap(["Windows frontend bootstrap"])
    gui_lock[("GUI package-lock.json")]
    npm_cache[("ignored Windows npm cache")]
    static_assets[("ignored GUI dist assets")]
    wrapper(["platform-keyed with-tools"])
    doctor(["Windows + WSL doctors"])
    commands(["Go / uv / Python / Node.js / npm commands"])
    combined --> wsl_setup
    combined --> win_bootstrap
    combined --> linux_bootstrap
    combined --> frontend_bootstrap
    manifest --> win_bootstrap
    manifest --> linux_bootstrap
    cache --> win_bootstrap
    cache --> linux_bootstrap
    win_bootstrap --> runtime
    linux_bootstrap --> runtime
    manifest --> doctor
    runtime --> doctor
    runtime --> wrapper
    wrapper --> commands
    runtime --> frontend_bootstrap
    gui_lock --> frontend_bootstrap
    npm_cache --> frontend_bootstrap
    frontend_bootstrap --> static_assets
    lifecycle(["nilo-dev start/status"])
    backend_window(["Bench Core window under WSL2"])
    frontend_window(["interface window on Windows"])
    wsl_data[("WSL-native runtime data")]
    lifecycle --> wrapper
    lifecycle --> backend_window
    lifecycle --> frontend_window
    backend_window --> wsl_data
```

The scripts modify environment variables only in their child process. The
frontend bootstrap restores locked dependencies and can produce static assets;
it does not package the Raspberry Pi appliance.

The lifecycle entry point starts the topology those scripts prepared, each
component in its own visible console window, and answers whether the endpoints
are responding. It has no arrow to any stored state on purpose: it keeps no
metadata, tracks no process, and signals nothing, so a developer stops a
component by closing its window. Bench Core's database and artifacts are the
only durable state, and they live on the WSL Linux filesystem.

## Files

- `02_Tools/toolchain.versions` - pinned versions, archive URLs, and checksums.
- `02_Tools/bootstrap/bootstrap.ps1` and `02_Tools/bootstrap/bootstrap.sh` - checksum-gated online/offline population.
- `02_Tools/bootstrap/bootstrap-development.ps1` - combined Windows and WSL2
  setup, bootstrap, and diagnostic entry point.
- `02_Tools/bootstrap/bootstrap-frontend.ps1` - locked online or offline npm
  restore for the browser GUI.
- `02_Tools/bootstrap/setup-wsl.ps1` - read-only WSL2 readiness check with
  explicit installation or conversion requests.
- `02_Tools/lib/Wsl.ps1` - checked WSL invocation, discovery, kernel check, and
  Windows-to-WSL path translation.
- `02_Tools/env/with-tools.ps1` - Windows child-process environment for Go,
  uv, managed Python, Node.js, npm, and the repository-local npm cache.
- `02_Tools/env/with-tools.sh` - Linux child-process environment for Go, uv,
  and managed Python.
- `02_Tools/env/with-go.ps1` and `02_Tools/env/with-go.sh` - Go-only compatibility wrappers.
- `02_Tools/scripts/doctor.ps1` - Windows Go, uv, Python, Node.js, and npm
  executable and exact-version verification.
- `02_Tools/scripts/doctor.sh` - Linux Go, uv, and Python executable and
  exact-version verification.
- `02_Tools/scripts/doctor-wsl.ps1` - Windows-side combined WSL2 and Linux-tool
  diagnosis.
- `02_Tools/scripts/nilo-dev.ps1` - the `start` and `status` entry point, its
  option surface, and its exit-code contract.
- `02_Tools/lib/DevLifecycle.ps1` - endpoint-based state classification, shell,
  PowerShell and Windows command-line quoting, listening-socket parsing,
  WSL-native data directory resolution, and the real endpoint, port, launcher
  and browser boundaries.
- `02_Tools/README.md` - usage, support, cache, offline-kit, lifecycle option,
  and exit-code guidance.
- `05_Software/scripts/aegis_test.py` - Go, Python, integration, and Windows
  frontend suite discovery that excludes generated environments and outputs.
- `05_Software/scripts/tests/test_toolchain_bootstrap.py` - manifest,
  non-global bootstrap, WSL safety, orchestration, and diagnostic contract
  tests.
- `05_Software/scripts/tests/test_development_lifecycle.py` - lifecycle command,
  option, readiness, conflict, quoting and reporting tests, driven entirely
  through injected boundaries.
- `.gitignore` - runtime, cache, and generated-artifact exclusions.
