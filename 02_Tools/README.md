# Repository tools and environment

`02_Tools` owns repository-local environment support. It keeps setup,
verification, deployment, and diagnostic tooling beside the project without
requiring a global installation or mixing tools into product source.

## Layout

- `bootstrap/` — scripts that acquire and verify required tool archives.
- `env/` — PowerShell and POSIX helpers that set process-local environment
  variables such as `PATH`.
- `scripts/` — setup, validation, deployment, and maintenance scripts.
- `runtime/` — downloaded or extracted tool runtimes, such as Go. This is
  intentionally ignored by Git.

## Go toolchain

Use a repository-local toolchain at `runtime/go/<version>/` when a host does
not already provide the required Go version. Bootstrap scripts must pin the
archive version and verify its published checksum before extraction. They must
not change the system-wide `PATH`; environment helpers set it only for the
command they launch.

Do not commit compiler binaries or downloaded archives. Commit the bootstrap,
environment, and verification scripts plus their version/checksum metadata so
another developer can recreate the same runtime.

## Script expectations

Scripts must document their supported platform, inputs, outputs, and whether
they make external changes. Deployment scripts remain explicit actions; they
must not run as part of ordinary setup or test commands.
