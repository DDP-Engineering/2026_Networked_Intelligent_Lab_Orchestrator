# Increment 1 acceptance

Increment 1 proves the portable platform-integration foundation. It is not the public remote-lab product yet.

## Prerequisites

- A supported checkout on Windows x86_64, Linux x86_64, or Linux ARM64.
- Git and the host archive/download utilities used by `02_Tools/bootstrap`.
- The repository-local toolchain populated with `02_Tools/bootstrap/bootstrap.ps1` or `02_Tools/bootstrap/bootstrap.sh`.
- A C compiler only when the host supports Go race builds. The verification script reports an explicit skip otherwise.

No system Go, Python, uv, Ruff, or pytest installation is required. The pinned tools and managed environments remain below `02_Tools` and the Python package directories.

## Exact verification commands

Windows PowerShell:

```powershell
.\02_Tools\bootstrap\bootstrap.ps1
.\02_Tools\scripts\test-increment-1.ps1
git diff --check
git status --short
```

Linux x86_64 or ARM64:

```sh
./02_Tools/bootstrap/bootstrap.sh
./02_Tools/scripts/test-increment-1.sh
git diff --check
git status --short
```

The scripts verify the pinned tool versions, sync the Python SDK, run its tests and Ruff, run the generic package contract against the test-only Go runner, run focused and full Go tests plus vet, run race tests where the host supports them, execute the runner acceptance slice in a clean temporary data directory, cross-compile Linux ARM64/Linux x86_64/Windows x86_64, and ensure the checks leave no new repository artifact.

## Observed evidence on 2026-08-31

- Windows x86_64: repository doctor passed with Go 1.27.0, uv 0.11.25, and managed Python 3.12.10.
- Python SDK: 32 tests passed; Ruff check and format check passed.
- Fake Python integration: 7 tests passed and 1 symlink-capability case skipped on this Windows host; Ruff check and format check passed.
- Generic integration contract: 10 tests passed.
- Go: the full suite, vet, the real fake-runner acceptance test, and portable builds for Linux ARM64, Linux x86_64, and Windows x86_64 passed.
- The acceptance slice proved default Ask behavior, immutable approval, ordered progress, verified artifact persistence, live cancellation and lease release, bounded crash diagnostics with truthful `incomplete`, startup reconciliation, and correlated audit evidence.
- Go race execution is not available on this Windows host because the required CGO C compiler is absent.

Native Raspberry Pi 4 Linux ARM64 execution has not yet been recorded. Increment 1 is therefore implementation-complete on the task branch but must not be called release-ready until `test-increment-1.sh` passes natively on the target Pi and that evidence is added here.

## Authoring entry points

- Wire protocol and schemas: `05_Software/contracts/integration/v1` and `05_Software/core/internal/integration/protocol`.
- Package manifest loading: `05_Software/core/internal/integration/manifest`.
- Python integration SDK: `05_Software/integrations/sdk/python`.
- Generic package contract: `05_Software/tests/contract/test_integration_contract.py`.
- Test-only contract package: `05_Software/tests/fixtures/contract_runner`.
- Operation lifecycle and evidence boundary: `05_Software/core/internal/operation`.

## Known later work

- Native Linux ARM64 process-group, cancellation, filesystem, and full-script evidence on Raspberry Pi 4.
- A production package/device catalog adapter around the operation validator and a general artifact-staging mapping for hardware packages.
- The first sigrok package and real supported instruments.
- Service installation, update/rollback, remote transport and authentication surfaces.

Sigrok behavior, MCP/HTTP APIs, GUI, WireGuard, Raspberry Pi image/appliance packaging, PLC flashing, and electronics-specific workflows remain outside Increment 1.
