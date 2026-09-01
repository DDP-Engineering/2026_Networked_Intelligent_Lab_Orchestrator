# Repository tools and environment

`02_Tools` owns repository-local environment support. It keeps setup,
verification, deployment, and diagnostic tooling beside the project without
requiring a global installation or mixing tools into product source.

## Layout

- `bootstrap/` — scripts that acquire and verify required tool archives.
- `env/` — PowerShell and POSIX helpers that set process-local environment
  variables such as `PATH`.
- `scripts/` - setup, validation, deployment, and maintenance scripts.
- `runtime/<platform>/` - downloaded or extracted Go, uv, Python, and supported
  host-specific Node.js runtimes for `windows-x86_64`, `linux-x86_64`, or
  `linux-arm64`. This is intentionally ignored by Git.
- `cache/<platform>/` - platform-specific uv data, with verified downloaded
  archives under the shared `cache/downloads/`. This is also ignored by Git.
- `toolchain.versions` - pinned versions and upstream SHA-256 digests readable
  before either Go or Python is available.

## One-command setup

From the repository root on Windows x86_64:

```powershell
.\02_Tools\bootstrap\bootstrap.ps1
.\02_Tools\scripts\doctor.ps1
```

For the approved Windows plus WSL2 development environment, use the combined
bootstrap from PowerShell:

```powershell
.\02_Tools\bootstrap\bootstrap-development.ps1
```

The command verifies that the selected distribution already runs as WSL2,
then bootstraps and diagnoses both Windows and Linux toolchains. It does not
install WSL unless explicitly requested:

```powershell
.\02_Tools\bootstrap\bootstrap-development.ps1 -Distribution Ubuntu -InstallWSL
```

WSL installation or WSL1-to-WSL2 conversion can require administrator access,
a reboot, or first-launch distribution initialization. When setup is requested
but cannot complete in the current process, the script exits with code 10 and
instructs the developer to finish those steps and rerun it. Use the read-only
checks independently with:

```powershell
.\02_Tools\bootstrap\setup-wsl.ps1
.\02_Tools\scripts\doctor-wsl.ps1
```

When exactly one distribution is installed, it is selected automatically.
Use `-Distribution <name>` when several distributions exist or when requesting
a new installation.

On Linux x86_64 or ARM64:

```sh
./02_Tools/bootstrap/bootstrap.sh
./02_Tools/scripts/doctor.sh
```

Bootstrap downloads only the pinned archives for the current platform, verifies
their published SHA-256 digests, and asks the pinned uv to install CPython below
the repository. Windows also receives the Node.js distribution used for native
GUI development. It changes no persistent PATH, shell profile, registry
association, or system package. Pass `-Offline` on PowerShell or `--offline` on
POSIX to forbid downloads and use a populated development kit.

## Go toolchain

Use the repository-local toolchain at
`runtime/<platform>/go/<version>/` when a host does not already provide the
required Go version. Bootstrap scripts pin the archive version and verify its
published checksum before extraction. They do not change the system-wide
`PATH`; environment helpers set it only for the command they launch.

The current development runtime is Go 1.27.0. The compatibility `with-go`
helpers remain available, but normal work uses
`env/with-tools.ps1 <command> [args...]` on Windows or
`env/with-tools.sh <command> [args...]` on POSIX so Go, uv, and managed Python
are all resolved from the current platform directory for that child process
only.

## Python and uv

uv 0.11.25 installs managed CPython 3.12.10 below the current platform runtime.
The combined environment wrapper asks the pinned uv for that interpreter and
adds its actual directory to the child process PATH. Its cache and generated
project environment are also platform-keyed; Windows and WSL therefore do not
share incompatible `.venv` contents. Project dependencies, including Ruff
0.16.5, remain pinned in each Python project's `uv.lock`; do not install Ruff
globally.

## Windows Node.js and frontend dependencies

Windows GUI development uses Node.js 24.20.0 LTS and its bundled npm 11.19.0.
The archive is checksum-verified and extracted below
`runtime/windows-x86_64/node/`; neither executable is installed globally.
`env/with-tools.ps1` adds that directory only to its child process and keeps the
npm cache below `cache/windows-x86_64/npm/`.

Restore the exact GUI dependency graph independently with:

```powershell
.\02_Tools\bootstrap\bootstrap-frontend.ps1
```

The combined development bootstrap invokes this step automatically. Use
`-Offline` only after the ignored npm cache and runtime have been populated.
Direct GUI commands use `npm.cmd` through the tool wrapper to avoid relying on
PowerShell script execution policy.

## WSL2 development support

- `bootstrap/setup-wsl.ps1` checks the selected distribution and requests
  installation or conversion only with `-Install`.
- `bootstrap/bootstrap-development.ps1` orchestrates Windows and WSL Linux
  bootstrap plus locked Windows GUI dependency restore from the shared
  checkout; `-Offline` is forwarded to every step.
- `bootstrap/bootstrap-frontend.ps1` restores the committed GUI lockfile with
  the pinned Windows Node.js and repository-local npm cache.
- `scripts/doctor-wsl.ps1` verifies the WSL2 kernel, Windows-to-WSL repository
  mapping, and the pinned Linux toolchain.
- `lib/Wsl.ps1` centralizes checked WSL invocation and path translation.

Application databases, artifacts, sockets, and simulated appliance state do
not belong under the Windows-mounted checkout. Once the backend server exists,
keep that runtime state on the WSL Linux filesystem as described in
`06_SoftwareDocu/Development_and_Simulation_Flow.md`.

Do not commit compiler binaries or downloaded archives. Commit the bootstrap,
environment, and verification scripts plus their version/checksum metadata so
another developer can recreate the same runtime. A platform-labelled offline
development-kit archive may contain the populated ignored directories together
with third-party licence notices; that archive is a delivery artifact, not Git
source.

## Script expectations

Scripts must document their supported platform, inputs, outputs, and whether
they make external changes. Deployment scripts remain explicit actions; they
must not run as part of ordinary setup or test commands.
