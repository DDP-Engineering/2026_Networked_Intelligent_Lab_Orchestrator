# Repository-local toolchain

## Purpose

The toolchain component gives a fresh supported checkout pinned Go, uv, and managed Python runtimes without persistent host changes. Downloaded runtimes and caches remain ignored so they can be populated online or carried in an offline development kit without bloating source history.

## Public interface

Developers run `bootstrap.ps1` or `bootstrap.sh` for one host, or
`bootstrap-development.ps1` to prepare both Windows and a selected WSL2
distribution from one checkout. `setup-wsl.ps1` keeps host installation and
conversion opt-in, while `doctor-wsl.ps1` proves the distribution, repository
mapping, and Linux tools. Commands run through `with-tools` or the Go-only
compatibility wrapper. `toolchain.versions` is the authority for versions,
platform archive URLs, and SHA-256 checksums.

## Key behaviors

- `REQ-PLAT-978855a5` - checksum-verifies pinned repository-local tools and avoids persistent PATH or system installations.
- `REQ-PLAT-93e1b194` - supplies the Go runtime used by portable Linux ARM64, Linux x86_64, and Windows x86_64 compilation.
- `REQ-PLAT-bdf0cbdb` - isolates tools, caches, managed Python, and generated
  Python environments by host platform.
- `REQ-PLAT-055d77ba` - verifies WSL2 readiness and requires explicit authority
  before requesting installation or conversion.
- `REQ-FUNC-6d8e811c` - bootstraps Windows and WSL2 tools through one Windows
  entry point.
- `REQ-TEST-e311d3bc` - diagnoses the WSL2 kernel, checkout mapping, and Linux
  toolchain from Windows.

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
    wrapper(["platform-keyed with-tools"])
    doctor(["Windows + WSL doctors"])
    commands(["Go / uv / Python commands"])
    combined --> wsl_setup
    combined --> win_bootstrap
    combined --> linux_bootstrap
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
```

The scripts modify environment variables only in their child process. They do not package the Raspberry Pi appliance.

## Files

- `02_Tools/toolchain.versions` - pinned versions, archive URLs, and checksums.
- `02_Tools/bootstrap/bootstrap.ps1` and `02_Tools/bootstrap/bootstrap.sh` - checksum-gated online/offline population.
- `02_Tools/bootstrap/bootstrap-development.ps1` - combined Windows and WSL2
  setup, bootstrap, and diagnostic entry point.
- `02_Tools/bootstrap/setup-wsl.ps1` - read-only WSL2 readiness check with
  explicit installation or conversion requests.
- `02_Tools/lib/Wsl.ps1` - checked WSL invocation, discovery, kernel check, and
  Windows-to-WSL path translation.
- `02_Tools/env/with-tools.ps1` and `02_Tools/env/with-tools.sh` - combined child-process environment.
- `02_Tools/env/with-go.ps1` and `02_Tools/env/with-go.sh` - Go-only compatibility wrappers.
- `02_Tools/scripts/doctor.ps1` and `02_Tools/scripts/doctor.sh` - executable and version verification.
- `02_Tools/scripts/doctor-wsl.ps1` - Windows-side combined WSL2 and Linux-tool
  diagnosis.
- `02_Tools/README.md` - usage, support, cache, and offline-kit guidance.
- `05_Software/scripts/aegis_test.py` - suite discovery that excludes every
  platform-specific generated virtual environment.
- `05_Software/scripts/tests/test_toolchain_bootstrap.py` - manifest,
  non-global bootstrap, WSL safety, orchestration, and diagnostic contract
  tests.
- `.gitignore` - runtime, cache, and generated-artifact exclusions.
