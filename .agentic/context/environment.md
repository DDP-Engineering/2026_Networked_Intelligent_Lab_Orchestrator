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
- `05_Software/core/go.mod` and `go.sum` pin the Go module graph. The
  module's declared language version can trail the pinned host Go release.
- `05_Software/frontend/package.json` and `package-lock.json` pin React,
  TypeScript, Vite, Vitest, ESLint, Prettier and Playwright by exact version.
- Each locked Python project pins its own graph, including Ruff, in its
  `uv.lock`: `05_Software/integrations/sdk/python/` and
  `05_Software/integrations/sigrok/` and
  `05_Software/integrations/camera/`.
- The reproducible build and test boundary is one Linux amd64 OCI image,
  generation `noble-20260905-01`, defined by `02_Tools/containers/build/` and
  run through Docker Engine or Podman. It carries the pinned language tools,
  native and ARM64 cross toolchains, offline dependency caches and headless
  Chromium. Its network-disabled suite stops at GUI component/build checks;
  browser acceptance runs separately in the privileged appliance-stage image.

## Setup

Two paths, described in full in `02_Tools/README.md`.

Host-local interactive development:

```powershell
.\02_Tools\bootstrap\bootstrap-development.ps1
```

It verifies the selected WSL2 distribution, bootstraps the Windows and Linux
toolchains, restores the GUI lockfile and installs the pinned Windows browser
for host-local manual GUI work. The appliance acceptance stage supplies its own
browser.
`bootstrap.ps1` and `bootstrap.sh` prepare one host alone;
`bootstrap-frontend.ps1` and `bootstrap-browser.ps1` are the focused steps.
`-Offline` on PowerShell and `--offline` on POSIX forbid downloads and require
a populated cache. WSL installation or conversion happens only with an explicit
`-InstallWSL`.

Reproducible container boundary:

```powershell
.\02_Tools\bootstrap\bootstrap-build-environment.ps1 -Distribution Ubuntu
```

`-Distribution` names the WSL registration, not its release label; use the name
reported by `wsl.exe --list --quiet` (for example, `Ubuntu` may contain Ubuntu
24.04). Repository validation ignores Docker Desktop's utility distributions
when it selects the sole development distribution. Set `NILO_WSL_DISTRIBUTION`
when more than one development distribution is installed.

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

- `nilo serve --profile appliance` takes an absolute `--data-dir`, requires a
  built `--gui-dir`, and uses either the helper-backed appliance certificate or
  an explicit `--tls-cert`/`--tls-key` pair. Interface-managed mode uses the
  wildcard listener only behind its destination gate; legacy `--bind` remains
  literal and private. `--local-status-bind` accepts only a literal loopback
  address.
- Further `serve` options name installed resources:
  `--nethelper-socket` (`/run/nilo/nethelper.sock`), `--integration-bundles`
  (`/opt/nilo/integration-bundles`) and `--sigrok-cli` (`/usr/bin/sigrok-cli`).
  Each is an administrator setting and never a client parameter.
- The data directory holds `nilo.db`, `artifacts/`, `integrations/` and
  `staging/`; the layout is decided in
  `05_Software/core/internal/config/config.go`.
- Runtime state - the database, artifacts, sockets and runner temporaries -
  always lives on a Linux filesystem, never on a Windows-mounted path. On the
  appliance the service account owns it under `/var/lib/nilo`; a locally run
  core keeps it below `$HOME/.local/state/` inside the distribution, and a home
  directory resolving to a Windows drive mount is refused rather than used.
- The interface reads `05_Software/frontend/.env` for `VITE_NILO_API_MODE` and
  `VITE_NILO_API_BASE_URL`. Override locally in an ignored `.env.local`. The
  browser requests origin-relative `/api` paths, which the Vite development
  server forwards to the configured endpoint and which an appliance serves
  itself beside the built assets.
- The appliance runs an unprivileged Bench Core beside the root
  `nilo-nethelper` service and its independent recovery timer, installed
  explicitly with `02_Tools/scripts/install-nethelper.sh`. The helper keeps its
  records and the WireGuard keystore under `/var/lib/nilo-nethelper`; the
  service account owns `/var/lib/nilo`. On Raspberry Pi deliveries only, append
  `--raspberry-pi-recovery` after wiring BCM17 (active-low button) and BCM27
  (active-high LED); omit it on every generic Linux system.
- Credentials are supplied by route, never on a command line: an API token from
  `--token-file`, a password on standard input, TLS material from operator-owned
  paths, and WireGuard keys and wireless passwords only inside the helper's
  keystore. An appliance's first administrator is created once at the console
  with `nilo admin bootstrap --data-dir <absolute-path> --username <name>`,
  which has no HTTP route and reads the password from standard input. Packaged
  first boot instead uses `nilo seed-admin` with an absolute root-protected
  password file; the command consumes that file only after durable bootstrap.

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
`02_Tools/tests/camera/` contains isolated discovery and capture probes;
physical camera acceptance requires observations on the named target hardware.

## Build and run

Produce what an appliance installs. `02_Tools/scripts/build-appliance.sh` emits
binaries under `05_Software/core/dist/linux-<arch>/`, the GUI `build`
script emits the static assets for `--gui-dir`.
`02_Tools/scripts/build-sigrok-bundle.py` and
`02_Tools/scripts/build-camera-bundle.py` prepare target-specific integration
catalogs from verified runtime inputs. The Sigrok bundle needs its Python
entrypoint, backend and third-party notices; Camera also needs its pinned
wheelhouse and dependency closure. `build-appliance.sh` includes these bundles
only when their respective bundle switches and inputs are supplied.

Install the privileged helper on the target explicitly, substituting the staged
binary and the service account:

```sh
sudo sh 02_Tools/scripts/install-nethelper.sh /path/to/nilo-nethelper nilo
```

Start Bench Core as that service account. The normal appliance path reads the
helper-backed identity and needs no private-key path:

```sh
nilo serve --profile appliance \
  --data-dir /var/lib/nilo --bind <private-address>:8443 \
  --gui-dir /opt/nilo/gui \
  --nethelper-socket /run/nilo/nethelper.sock
```

Iterate on the interface from the development host against that appliance, with
`VITE_NILO_API_BASE_URL` pointing at it:

```powershell
.\02_Tools\env\with-tools.ps1 npm.cmd --prefix .\05_Software\frontend run dev
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

Packaging has a separate connected acquisition step followed by the same
network-none boundary. The checked-in input lock is intentionally incomplete,
so production preparation and artifact assembly refuse until its reasons are
resolved. Resolve reviewed source declarations and inspect the proposal before
adopting it as the lock:

```powershell
.\02_Tools\packaging\fetch\resolve-inputs.ps1
```

```sh
sh ./02_Tools/packaging/fetch/resolve-inputs.sh
```

After adoption, fetch, verify and prepare each exact target:

```powershell
.\02_Tools\packaging\fetch\fetch-all.ps1 ubuntu-24.04-amd64
.\02_Tools\packaging\verify\verify-cache.ps1 ubuntu-24.04-amd64
.\02_Tools\env\with-build-environment.ps1 package ubuntu-24.04-amd64
```

The equivalent POSIX entry points live under `02_Tools/packaging/{fetch,verify}`
and `02_Tools/env/with-build-environment.sh`. A successful preparation writes a
deterministic receipt below `02_Tools/cache/containers/outputs/packaging/`.
With target binaries, GUI, production bundles, dependency pools and (for Pi)
the base image staged, the POSIX entry points under
`02_Tools/packaging/build/` create canonical payloads, `.deb` packages,
self-extracting `.run` installers, the `.img.xz` card image and the closed
release directory. `02_Tools/packaging/verify/` recomputes their integrity and
evidence closure. These tools never record physical hardware acceptance as
passed; those rows remain `not_run` until executed on the named target.

## Verification

`.agentic/aegis.toml` declares the command keys, with `workdir = 05_Software`:

- `test` - `python ../02_Tools/scripts/aegis_test.py full`
- `test_impacted` - `python ../02_Tools/scripts/aegis_test.py impacted {tests}`
- `lint` - `python ../02_Tools/scripts/aegis_test.py lint`

The router in `02_Tools/scripts/aegis_test.py` discovers every Go module and
locked Python project below `05_Software`, and on Windows the frontend project
as well. `full` runs the Camera probe tests under `02_Tools/tests/camera/`,
the tooling tests under `02_Tools/tests/software/`, `go test ./...`, and each
Python suite through `uv run pytest`. On Windows it then runs GUI `test:ci`
and `build`, followed immediately by `02_Tools/tests/appliance/check.sh`
through WSL2. The shared runner contract suite runs last on either host.
The appliance stage rebuilds the GUI and runs Playwright against an
appliance-profile core; direct browser execution on the host is not acceptance
evidence. Impacted routing is deliberately conservative and runs the same set.
`lint` runs `go vet`, Python compilation, and on Windows GUI `format:check`
and `lint`.

Other entry points:

- `02_Tools/scripts/test-increment-1.ps1` and `.sh` - the foundation gate.
- `02_Tools/scripts/verify-portable-core.ps1` and `.sh` - the Go suite plus
  CGO-free builds for Linux ARM64, Linux x86_64 and Windows x86_64.
- `02_Tools/env/with-build-environment.ps1 test` (or the `.sh` wrapper) - the
  authoritative offline gate: locked restores, lint, unit, contract and build
  gates plus amd64 and ARM64 checks. It cannot run the privileged browser stage.
- `02_Tools/tests/build-environment/verify-host-isolation.sh` - proves from the
  host side that a container run changed no guarded dependency tree.
- `02_Tools/tests/appliance/check.sh` - the appliance verification stage: Bench
  Core on `--profile appliance` with generated TLS material, a built interface
  directory and the real root helper, against adapters a real NetworkManager
  detected in disposable container networks. Chromium reaches the management
  listener from a dedicated network namespace. Live journeys exercise real
  network sampling and WireGuard lifecycle behavior; explicitly labelled
  presentation fixtures cover bounded UI-only states. Each specification owns
  a fresh backend. Linux and WSL2 only; a missing container engine or an
  unsupported host is named and refused rather than skipped, and the host's
  interfaces and default route are compared around every run.
- `02_Tools/tests/network/check.sh` - opt-in system test in a disposable
  NetworkManager container; it never joins the host network or system bus.
- `02_Tools/scripts/check-sigrok-integration.py` - opt-in smoke against an
  isolated HTTPS appliance instance with a real analyzer.
- `02_Tools/tests/camera/` - isolated camera probes and target-hardware
  evidence capture; its unit tests are included in `full`.

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
- `06_SoftwareDocu/Camera_Integration.md` - Camera bundle inputs, operation and
  physical acceptance evidence.
- `05_Software/INCREMENT_1_ACCEPTANCE.md` - foundation gate scope, commands and
  the outstanding native Raspberry Pi ARM64 run.
- `.agentic/knowledge/repository-toolchain.md` - toolchain component detail.
