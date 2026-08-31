# Repository tools and environment

`02_Tools` owns repository-local environment support. It keeps setup,
verification, deployment, and diagnostic tooling beside the project without
requiring a global installation or mixing tools into product source.

## Layout

- `bootstrap/` — scripts that acquire and verify required tool archives.
- `env/` — PowerShell and POSIX helpers that set process-local environment
  variables such as `PATH`.
- `scripts/` - setup, validation, deployment, and maintenance scripts.
- `runtime/` - downloaded or extracted tool runtimes, such as Go. This is
  intentionally ignored by Git.
- `cache/` - verified archives and dependency downloads used for repeatable or
  offline setup. This is also ignored by Git.
- `toolchain.versions` - pinned versions and upstream SHA-256 digests readable
  before either Go or Python is available.

## One-command setup

From the repository root on Windows x86_64:

```powershell
.\02_Tools\bootstrap\bootstrap.ps1
.\02_Tools\scripts\doctor.ps1
```

On Linux x86_64 or ARM64:

```sh
./02_Tools/bootstrap/bootstrap.sh
./02_Tools/scripts/doctor.sh
```

Bootstrap downloads only the pinned Go and uv archives, verifies their
published SHA-256 digests, and asks the pinned uv to install CPython below the
repository. It changes no persistent PATH, shell profile, registry association,
or system package. Pass `-Offline` on PowerShell or `--offline` on POSIX to
forbid downloads and use a populated development kit.

## Go toolchain

Use a repository-local toolchain at `runtime/go/<version>/` when a host does
not already provide the required Go version. Bootstrap scripts must pin the
archive version and verify its published checksum before extraction. They must
not change the system-wide `PATH`; environment helpers set it only for the
command they launch.

The current development runtime is Go 1.27.0 at
`runtime/go/1.27.0/go/`. The compatibility `with-go` helpers remain available,
but normal work uses `env/with-tools.ps1 <command> [args...]` on Windows or
`env/with-tools.sh <command> [args...]` on POSIX so Go, uv, and managed Python
are all resolved from `02_Tools` for that child process only.

## Python and uv

uv 0.11.25 installs managed CPython 3.12.10 below `runtime/python`. The
combined environment wrapper asks the pinned uv for that interpreter and adds
its actual directory to the child process PATH. Its cache is `cache/uv`.
Project dependencies, including Ruff 0.16.5, remain pinned in each Python
project's `uv.lock`; do not install Ruff globally.

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
