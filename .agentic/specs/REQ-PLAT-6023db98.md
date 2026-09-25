# Native Ubuntu build-host contract

## Supported entry points

- Windows 11 x86-64: `bootstrap-build-environment.ps1` validates Ubuntu 24.04
  x86-64 under WSL2, then calls the shared Linux engine and image-setup scripts.
- Native Ubuntu 24.04 x86-64: `bootstrap-build-environment.sh` validates the
  local host, then calls those same shared scripts directly.

Both entry points select `docker` or `podman` and consume the same
`linux/amd64` image reference, reconstruction lock, multipart Git LFS archive,
inventory verification, and network-disabled execution wrapper. They do not
define separate dependency or container manifests. ARM64 developer hosts are
not supported by this contract.

## Native POSIX interface

```text
bootstrap-build-environment.sh
  [--engine docker|podman]
  [--install-engine --engine-archive-path DIR]
  [--archive-path DIR]
  [--offline | --rebuild-from-pinned-inputs]
```

The command exits with usage status 2 for invalid combinations. Before calling
an engine it requires `ID=ubuntu`, `VERSION_ID=24.04`, `uname -m` equal to
`x86_64`, and systemd state `running` or `degraded`. Installation is never
implicit: `--install-engine` requires `--engine-archive-path`, and the shared
installer verifies every multipart/archive/package checksum before mutation.
An already-present selected engine is preserved while missing common tools may
be installed from the verified common package group.

The engine verifier rejects storage below `/mnt/` so the Windows/WSL2 path does
not place engine state on a Windows-mounted filesystem. Native Ubuntu normally
uses Linux-native paths and passes the same check. Docker Desktop is not
inspected, required, or documented as a dependency.

## Test injection boundary

Executable contract tests may override the OS-release file, architecture and
systemd probes, and the shared engine/image helper paths through `NILO_TEST_*`
environment variables. These variables substitute process-local probes only;
they do not bypass the production validation sequence or authorize package
installation.

## Acceptance

- Contract tests accept native Ubuntu 24.04 x86-64 with systemd running or
  degraded and reject other OS releases and architectures before engine use.
- Windows/WSL2 and native Ubuntu entry points invoke the same Linux engine and
  image helpers against the same lock and archive format.
- A present selected engine is preserved; installation requires an explicit
  option and a verified package archive before any host mutation.
- The engine verifier refuses Windows-mounted storage, and the execution
  wrapper keeps the build container network-disabled.
