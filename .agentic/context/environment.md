<!--
Copyright (c) 2026 DDP Engineering
This software is licensed under the PolyForm Noncommercial License 1.0.0.
Commercial use requires a separate commercial license.
See the LICENSE.txt in the .agentic folder for full license details.
SPDX-License-Identifier: LicenseRef-PolyForm-Noncommercial-1.0.0
-->


# Environment guide

## Toolchain

Nothing is installed system-wide. Pinned runtimes are extracted below the
ignored `02_Tools/runtime/<platform>/`, with caches under `02_Tools/cache/`,
and the environment wrappers change `PATH` only for their child process.

- `02_Tools/toolchain.versions` is the authority for the host toolchain: Go,
  uv, the uv-managed CPython, Node.js, npm, and the Playwright and Chromium
  versions, each with its published SHA-256.
- `05_Software/nilo_assistant/go.mod` and `go.sum` pin the Go module graph. The
  module's declared language version can trail the pinned host Go release.
- `05_Software/nilo_gui/package.json` and `package-lock.json` pin React,
  TypeScript, Vite, Vitest, ESLint, Prettier and Playwright by exact version.
- Each locked Python project pins its own graph, including Ruff, in its
  `uv.lock`: `05_Software/integration_sdk/python/`,
  `05_Software/integrations/sigrok/` and `.../fake_python/`.
- The reproducible build and test boundary is one Linux amd64 OCI image,
  generation `noble-20260905-01`, defined by `02_Tools/containers/build/` and
  run through Docker Engine or Podman. It carries the pinned language tools,
  native and ARM64 cross toolchains, offline dependency caches and headless
  Chromium.

## Setup

Two paths, described in full in `02_Tools/README.md`.

Host-local interactive development:

```powershell
.\02_Tools\bootstrap\bootstrap-development.ps1
```

It verifies the selected WSL2 distribution, bootstraps the Windows and Linux
toolchains, restores the GUI lockfile and installs the pinned browser.
`bootstrap.ps1` and `bootstrap.sh` prepare one host alone;
`bootstrap-frontend.ps1` and `bootstrap-browser.ps1` are the focused steps.
`-Offline` on PowerShell and `--offline` on POSIX forbid downloads and require
a populated cache. WSL installation or conversion happens only with an explicit
`-InstallWSL`.

Reproducible container boundary:

```powershell
.\02_Tools\bootstrap\bootstrap-build-environment.ps1 -Distribution Ubuntu-24.04
```

```sh
./02_Tools/bootstrap/bootstrap-build-environment.sh
```

Git and Git LFS are a human prerequisite: the multipart image archive is stored
through Git LFS, and setup never contacts a forge, changes a remote or
authenticates. Missing archive content is reported by file and object with its
three recovery paths - fetch the LFS objects, supply the parts with
`-ArchivePath`, or select the pinned networked rebuild with
`-RebuildFromPinnedInputs`. Installing a container engine is a separate,
explicitly requested host change.

## Configuration and services

- `nilo serve --profile appliance` takes an absolute `--data-dir` and an
  explicit `--bind`, requires `--tls-cert`, `--tls-key` and `--gui-dir`, forces
  secure session cookies, and refuses a public or wildcard listener.
- Further `serve` options name installed resources:
  `--nethelper-socket` (`/run/nilo/nethelper.sock`), `--integration-bundles`
  (`/opt/nilo/integration-bundles`) and `--sigrok-cli` (`/usr/bin/sigrok-cli`).
  Each is an administrator setting and never a client parameter.
- The data directory holds `nilo.db`, `artifacts/`, `integrations/` and
  `staging/`; the layout is decided in
  `05_Software/nilo_assistant/internal/config/config.go`.
- Runtime state - the database, artifacts, sockets and runner temporaries -
  always lives on a Linux filesystem, never on a Windows-mounted path. On the
  appliance the service account owns it under `/var/lib/nilo`; a locally run
  core keeps it below `$HOME/.local/state/` inside the distribution, and a home
  directory resolving to a Windows drive mount is refused rather than used.
- The interface reads `05_Software/nilo_gui/.env` for `VITE_NILO_API_MODE` and
  `VITE_NILO_API_BASE_URL`. Override locally in an ignored `.env.local`. The
  browser requests origin-relative `/api` paths, which the Vite development
  server forwards to the configured endpoint and which an appliance serves
  itself beside the built assets.
- The appliance runs an unprivileged Bench Core beside the root
  `nilo-nethelper` service and its independent recovery timer, installed
  explicitly with `02_Tools/scripts/install-nethelper.sh`. The helper keeps its
  records and the WireGuard keystore under `/var/lib/nilo-nethelper`; the
  service account owns `/var/lib/nilo`.
- Credentials are supplied by route, never on a command line: an API token from
  `--token-file`, a password on standard input, TLS material from operator-owned
  paths, and WireGuard keys and wireless passwords only inside the helper's
  keystore. An appliance's first administrator is created once at the console
  with `nilo admin bootstrap --data-dir <absolute-path> --username <name>`,
  which has no HTTP route and reads the password from standard input.

## Readiness checks

```powershell
.\02_Tools\scripts\doctor.ps1
.\02_Tools\scripts\doctor-wsl.ps1
```

```sh
./02_Tools/scripts/doctor.sh
```

The doctors verify that each pinned executable is present at its exact version;
`doctor-wsl.ps1` additionally checks the WSL2 kernel, the checkout mapping and
the Linux toolchain.

`nilo serve-foundation-check --data-dir <absolute-path>` opens and reconciles a
data directory once and exits. `02_Tools/scripts/verify-build-environment.sh`
and `02_Tools/tests/build-environment/` check the image, the archive and the
recorded acceptance evidence. On a prepared target,
`02_Tools/scripts/check-sigrok-integration.py` starts an isolated HTTPS
instance and exercises the installed analyzer through the integration.

## Build and run

Produce what an appliance installs. `02_Tools/scripts/build-appliance.sh` emits
binaries under `05_Software/nilo_assistant/dist/linux-<arch>/`, the GUI `build`
script emits the static assets for `--gui-dir`, and
`02_Tools/scripts/build-sigrok-bundle.py` prepares an integration catalog for a
target architecture and Python minor.

Install the privileged helper on the target explicitly, substituting the staged
binary and the service account:

```sh
sudo sh 02_Tools/scripts/install-nethelper.sh /path/to/nilo-nethelper nilo
```

Start Bench Core as that service account, with the target's own address and
certificate paths:

```sh
nilo serve --profile appliance \
  --data-dir /var/lib/nilo --bind <private-address>:8443 \
  --gui-dir /opt/nilo/gui \
  --tls-cert /etc/nilo/tls/server.crt --tls-key /etc/nilo/tls/server.key \
  --nethelper-socket /run/nilo/nethelper.sock
```

Iterate on the interface from the development host against that appliance, with
`VITE_NILO_API_BASE_URL` pointing at it:

```powershell
.\02_Tools\env\with-tools.ps1 npm.cmd --prefix .\05_Software\nilo_gui run dev
```

Build inside the locked container, which is the result that counts:

```powershell
.\02_Tools\env\with-build-environment.ps1 build
.\02_Tools\env\with-build-environment.ps1 shell
```

```sh
./02_Tools/env/with-build-environment.sh build
```

The container runs with no network, no privileges, no engine socket and the
checkout mounted read-only; declared artifacts are published to the ignored
`02_Tools/cache/containers/outputs`.

## Verification

`.agentic/aegis.toml` declares the command keys, with `workdir = 05_Software`:

- `test` - `python scripts/aegis_test.py full`
- `test_impacted` - `python scripts/aegis_test.py impacted {tests}`
- `lint` - `python scripts/aegis_test.py lint`

The router in `05_Software/scripts/aegis_test.py` discovers every Go module and
locked Python project below `05_Software`, and on Windows the frontend project
as well. `full` runs the script tests, `go test ./...`, each Python suite
through `uv run pytest`, then the GUI `test:ci`, `build` and `test:e2e`;
impacted routing is deliberately conservative and runs the same set. `lint`
runs `go vet`, Python compilation, and the GUI `format:check` and `lint`.

Other entry points:

- `05_Software/scripts/test-increment-1.ps1` and `.sh` - the foundation gate.
- `05_Software/scripts/verify-portable-core.ps1` and `.sh` - the Go suite plus
  CGO-free builds for Linux ARM64, Linux x86_64 and Windows x86_64.
- `02_Tools/env/with-build-environment.ps1 test` (or the `.sh` wrapper) - the
  authoritative offline gate: locked restores, lint, unit, contract and build
  gates, amd64 and ARM64 checks, and headless Playwright.
- `02_Tools/tests/build-environment/verify-host-isolation.sh` - proves from the
  host side that a container run changed no guarded dependency tree.
- `02_Tools/tests/network/check.sh` - opt-in system test in a disposable
  NetworkManager container; it never joins the host network or system bus.
- `02_Tools/scripts/check-sigrok-integration.py` - opt-in smoke against an
  isolated HTTPS appliance instance with a real analyzer.

`06_SoftwareDocu/Development_and_Simulation_Flow.md` section 9 states what each
verification level proves. A lower level never substitutes for a higher release
gate, and only an observation on the target hardware is evidence that a
physical operation occurred.

## Platform details

- Supported development hosts are exactly Windows x86-64 with an Ubuntu 24.04
  x86-64 WSL2 distribution running systemd, or native Ubuntu 24.04 x86-64
  running systemd. Other distributions, other Ubuntu releases and ARM64
  development hosts are refused by the host gate.
- The Windows checkout is visible from WSL through the normal `/mnt/<drive>`
  mapping. Source is shared through that path; databases, artifacts, sockets
  and runner temporaries stay on the Linux filesystem.
- `02_Tools/runtime/` and `02_Tools/cache/` are keyed by platform
  (`windows-x86_64`, `linux-x86_64`, `linux-arm64`) so Windows and WSL never
  share an incompatible environment.
- Host-local Linux ARM64 runtimes exist because ARM64 is an appliance target
  that has to be built and diagnosed, not because it is a development host.
- `.gitattributes` forces LF for `*.sh` and the container build definitions, and
  routes only the immutable `02_Tools/archives/**/*.partNNN` payloads through
  Git LFS. Generated runtimes, caches, `node_modules/`, `.venv*/`, `dist/` and
  test output are ignored.

## Authoritative references

- `02_Tools/README.md` - setup, recovery, container commands, export,
  publication preflight and exit codes.
- `02_Tools/toolchain.versions` - pinned versions and published checksums.
- `02_Tools/tests/build-environment/README.md` - the BE-01 to BE-10 acceptance
  evidence sequence.
- `06_SoftwareDocu/Development_and_Simulation_Flow.md` - development topology,
  daily flow, browser suite and verification levels.
- `06_SoftwareDocu/Network_Operations.md` - isolated network test and appliance
  preparation.
- `06_SoftwareDocu/Sigrok_Integration.md` - bundle preparation and deployment.
- `05_Software/INCREMENT_1_ACCEPTANCE.md` - foundation gate scope, commands and
  the outstanding native Raspberry Pi ARM64 run.
- `.agentic/knowledge/repository-toolchain.md` - toolchain component detail.
