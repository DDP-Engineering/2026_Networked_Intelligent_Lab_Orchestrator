# REQ-PLAT-978855a5 - Bootstrap repository-local development toolchain

## Behavior

The source repository carries small bootstrap, environment, and diagnostic
scripts. Large downloaded tool archives, extracted runtimes, caches, and
virtual environments stay below ignored repository directories. Bootstrap
never updates a user or system PATH, shell profile, registry association, or
system package manager.

The pinned toolchain is Go 1.27.0, uv 0.11.25, and CPython 3.12.10. Go and uv
archives are accepted only after a SHA-256 match against the tracked manifest.
The pinned uv executable installs its frozen managed-Python distribution below
`02_Tools/runtime/python` and uses `02_Tools/cache/uv` for downloads. A populated
offline development kit may skip downloads but must pass the same doctor.

## Data / interface

- `02_Tools/toolchain.versions` contains simple `NAME=VALUE` records so both
  PowerShell and POSIX shell can read it before Python exists.
- `02_Tools/bootstrap/bootstrap.ps1` supports Windows x86_64.
- `02_Tools/bootstrap/bootstrap.sh` supports Linux x86_64 and ARM64.
- `02_Tools/env/with-tools.ps1 <command> [args...]` and the POSIX equivalent
  ask the pinned uv for the managed interpreter and prepend only the current
  process environment with the local Go, uv, and managed-Python directories.
- `02_Tools/scripts/doctor.ps1` and `doctor.sh` fail if a tool is absent or its
  reported version differs from the manifest.
- `02_Tools/runtime`, `02_Tools/cache`, Python virtual environments, and Python
  bytecode are ignored by Git.

The tracked manifest covers these archives:

| Tool | Target | Archive |
|---|---|---|
| Go | Windows x86_64 | `go1.27.0.windows-amd64.zip` |
| Go | Linux x86_64 | `go1.27.0.linux-amd64.tar.gz` |
| Go | Linux ARM64 | `go1.27.0.linux-arm64.tar.gz` |
| uv | Windows x86_64 | `uv-x86_64-pc-windows-msvc.zip` |
| uv | Linux x86_64 | `uv-x86_64-unknown-linux-gnu.tar.gz` |
| uv | Linux ARM64 | `uv-aarch64-unknown-linux-gnu.tar.gz` |

## Examples

```powershell
.\02_Tools\bootstrap\bootstrap.ps1
.\02_Tools\scripts\doctor.ps1
.\02_Tools\env\with-tools.ps1 go test ./...
```

```sh
./02_Tools/bootstrap/bootstrap.sh
./02_Tools/scripts/doctor.sh
./02_Tools/env/with-tools.sh go test ./...
```

An archive with a wrong digest fails before extraction. An offline bootstrap
with an incomplete runtime fails with the missing tool named and does not try
the network.

## Acceptance

- Manifest tests prove every supported Go and uv archive has a pinned 64-digit
  lowercase SHA-256 digest and an HTTPS upstream URL derivable from its version.
- Script-contract tests prove checksum gates, repository-local runtime/cache
  variables, and the absence of persistent PATH or shell-profile mutation.
- The Windows doctor passes on the development host after bootstrap.
- The POSIX doctor and full suite pass natively on Raspberry Pi 4 Linux ARM64
  before Increment 1 is called release-ready.
