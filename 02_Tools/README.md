# Repository tools and environment

`02_Tools` owns repository-local environment support. It keeps setup,
verification, deployment, and diagnostic tooling beside the project without
requiring a global installation or mixing tools into product source.

## Everyday commands

Use `nilo-tools.ps1` on Windows and `nilo-tools.sh` on native Ubuntu. Help works
before any runtime, WSL distribution or container engine is installed:

```powershell
.\02_Tools\nilo-tools.ps1 help
.\02_Tools\nilo-tools.ps1 setup -Distribution Ubuntu -Offline
.\02_Tools\nilo-tools.ps1 dev start -Distribution Ubuntu
.\02_Tools\nilo-tools.ps1 dev status -Distribution Ubuntu
.\02_Tools\nilo-tools.ps1 doctor -Distribution Ubuntu
.\02_Tools\nilo-tools.ps1 test -Distribution Ubuntu
.\02_Tools\nilo-tools.ps1 build -Distribution Ubuntu
.\02_Tools\nilo-tools.ps1 package ubuntu-24.04-amd64 -Distribution Ubuntu
```

| Command | Result |
|---|---|
| `setup` | Prepare pinned Windows/WSL host tools and the canonical image. `tools` or `container` selects only that part. Offline restoration requires already-acquired verified inputs. |
| `dev` | Windows `start`/`status` preserves the real simulation backend and GUI in visible consoles. Login is `admin` / `admin_password`; stop each server with Ctrl+C in its console. |
| `doctor` | Inspect host tools and the accepted container inventory/digest. `tools` or `container` selects one boundary. |
| `test` | Run the complete canonical offline suite. `tools` selects tooling tests; `network`, `appliance` and `packaging` explicitly run their host acceptance fixtures. |
| `build` | Publish `core/linux-amd64/{nilo,nilo-nethelper}`, `core/linux-arm64/{nilo,nilo-nethelper}` and `nilo_gui/` below `cache/containers/outputs/`. |
| `package TARGET` | Verify and prepare offline inputs, writing a preparation receipt. This is not a finished release. `package release` assembles previously built artifacts; `package verify` checks a release. |

Native Ubuntu uses the same commands, with POSIX option names:

```sh
sh ./02_Tools/nilo-tools.sh setup --offline
sh ./02_Tools/nilo-tools.sh doctor
sh ./02_Tools/nilo-tools.sh test
sh ./02_Tools/nilo-tools.sh build
sh ./02_Tools/nilo-tools.sh package rpi-debian13-arm64
sh ./02_Tools/nilo-tools.sh dev backend --data-dir /home/me/.local/state/nilo-simulation
```

Linux `dev backend` runs the simulation Core in the current terminal; it does
not launch the Windows GUI development server. Its data directory must be an
absolute Linux path outside `/mnt/`. The Windows development topology remains
the normal interactive browser loop.

`setup -InstallWSL` and `setup -InstallEngine -EngineArchivePath DIR` request
host changes explicitly. `-RebuildFromPinnedInputs` selects connected acquisition
and then an offline image build; it cannot be combined with `-Offline`. Native
Linux uses the equivalent `--install-engine`, `--engine-archive-path` and
`--rebuild-from-pinned-inputs` options. Automatic distribution selection ignores
Docker Desktop utility distributions and requires one development distribution.
Windows container commands discover the selected distribution's backing volume
and mount its free-space evidence read-only. Both launchers provide the host's
kernel-module inventory read-only when available. When invoking the shell launcher
directly inside WSL, set `NILO_WSL_HOST_VOLUME` to the Windows volume containing
that distribution's virtual disk; the checkout drive may be different.

The public launchers select internal helpers below `bootstrap/`, `env/`,
`scripts/`, `packaging/` and `tests/`. Those files stay small and focused; ordinary
work does not require choosing among them. The shared `env/with-tools.*` helpers
resolve only Go for Go commands, so Go-only fixtures need no Python/Node runtime.
Container `run` preserves each argument literally, including spaces and quotes.
Pass `run sh -c 'COMMANDS'` when explicitly requesting a shell expression.
Ignored `tests/build-environment/evidence.local.json` stays on the host; copied
source retains the tracked evidence template and archive control metadata.

CI calls the same offline route inside its isolated canonical container through
`nilo-tools.sh test --in-container`. That internal option checks the isolated
source location and image lock and refuses ordinary host execution.

## Specialist references

The following sections document internal helper contracts and advanced
environment/archive operations. Release assembly details are in
[packaging/README.md](packaging/README.md), privileged acceptance in
[tests/appliance/README.md](tests/appliance/README.md), and source/image
provenance in [containers/build/README.md](containers/build/README.md).

## Offline delivery input foundation

`packaging/` defines the exact Ubuntu 24.04 amd64 and Raspberry Pi OS/Debian 13
arm64 targets. Its connected fetch commands require version and licence
provenance and validate HTTPS origins, sizes, and SHA-256 digests into the
ignored `cache/packaging/` content-addressed cache.
Verification opens no URL. The `package <target>` build-environment action
mounts that cache read-only into the existing network-none OCI boundary and
produces a deterministic preparation receipt under
`cache/containers/outputs/packaging/`.

The checked-in packaging lock is intentionally incomplete, so these commands
currently prove fail-closed orchestration rather than create a release
artifact. See `packaging/README.md` for commands, exit behavior, and the exact
boundary. Raspberry Pi and mini-PC physical acceptance remains separate and
cannot be inferred from this environment.

## Reproducible OCI build environment

The required build and repository-test boundary is one Linux amd64 OCI image.
Windows x86-64 runs it inside a selected Ubuntu 24.04 WSL2 distribution;
native Ubuntu 24.04 x86-64 runs it directly. Docker Engine is the primary
runtime; Podman consumes the same definition and archive. Docker Desktop is
neither required nor assumed. Native Windows browser execution
remains an optional headed visual-development loop; the acceptance browser gate
runs headlessly in the Linux container.

The tracked `containers/build/Containerfile`, lock, inventory, licence index,
SBOM source record, and provenance source record define generation
`noble-20260928-01`; the archive manifest retains the previous generation until
the new recipe passes rebuild, offline verification and archive round-trip.
Extracted engine state and caches remain on the WSL Linux
filesystem and are ignored. Only immutable `archives/**/.partNNN` payloads use
Git LFS; `current.json` is ordinary Git metadata.

### Supported hosts, prerequisites, and what is not yet proven

The approved developer host is exactly one of:

- Windows x86-64 with an Ubuntu 24.04 x86-64 WSL2 distribution running systemd;
- native Ubuntu 24.04 x86-64 running systemd.

This is not generic "Linux support". Other distributions, other Ubuntu
releases, and ARM64 developer hosts are refused by the host gate; ARM64 appears
only as a cross-compilation and appliance target, never as a development host.

**Git and Git LFS are a human prerequisite, not something this repository can
bootstrap.** The multipart image archive is stored through Git LFS, and the
host-bootstrap archive that can install Git LFS inside WSL is itself stored the
same way, so it cannot bootstrap a machine that has no LFS client. Install Git
LFS first (Git for Windows includes it), then `git lfs pull` before running
setup; alternatively obtain the archive parts by another documented route and
pass them with `-ArchivePath`. Setup never fetches Git or LFS content, changes
remotes, or authenticates to a forge.

What the checks do and do not prove:

- SHA-256 part, archive, and input digests prove **content integrity against
  the reviewed manifest in this repository**. They are not proof of publisher
  authenticity: there is no signed manifest, signed provenance, or external
  trust root, so an actor able to replace both an archive and its manifest
  could produce an internally consistent set. Treat the Gitea authority plus
  the independent backup as the trust anchor.
- Image, definition and archive vulnerability/licence scanning is
  **not implemented**. The plan requires it before acceptance; no scanner is
  wired in, no scan has been run, and no gate reports one. The generated SBOM records
  package identity with `NOASSERTION` licence fields and ships the collected
  licence texts alongside it; that is an inventory, not a legal review.
- **Podman** is implemented against the same definition, archive, and command
  contract as Docker, and is exercised by the source-level tests. A real Podman
  import and container run has not been observed, so its verification status is
  pending, not passed.

### Setup from an already-acquired clone

Obtain the Git checkout and its LFS content yourself, then run:

```powershell
.\02_Tools\bootstrap\bootstrap-build-environment.ps1 -Distribution Ubuntu
```

The value of `-Distribution` is the WSL registration name reported by
`wsl.exe --list --quiet`, not the Ubuntu release label. For example, a
registration named `Ubuntu` can contain Ubuntu 24.04. Docker Desktop's utility
distributions are not development distributions.

On native Ubuntu 24.04 x86-64, use the POSIX entry point instead:

```sh
./02_Tools/bootstrap/bootstrap-build-environment.sh
```

Setup reads local files only. It never fetches Git/LFS, changes remotes, or
authenticates to a forge. It inspects pending or local archive content before
requiring an OCI engine or `skopeo`; those tools are required only after a
usable current image, restore, or explicit rebuild action is selected. It
validates every part, the joined archive, imported image digest, and tool
inventory. If content is missing, corrupt, or still a Git LFS pointer, it names
the affected file/OID and stops with three paths:

1. fetch the named LFS objects for the current clone yourself;
2. obtain the same parts from another repository origin and supply their
   directory with `-ArchivePath`;
3. explicitly select the networked fallback with
   `-RebuildFromPinnedInputs`.

The third path validates and materializes the complete 677-record reconstruction
manifest into the ignored `02_Tools/cache/containers/upstream` directory before
the engine starts. The manifest covers the apt closure, Go and module proxy,
managed CPython, locked Python wheels and the pinned Camera Git source with its
build tools, uv, Node and npm tarballs, and Playwright
browser payloads. Every byte is checked for size and SHA-256; missing, extra,
symlinked, or tampered input fails closed. The engine then receives that directory
as the `nilo_inputs` named context and builds with `--network=none`.
Source-lock coverage is checked before build and export. Offline preparation
checks Python input hashes before clearing environments and checks installed
dependency consistency before collecting tests. Go restore verifies `go.sum`.

`-Offline`/`--offline` forbids the third path. Engine installation is also a separate host
change: generate or obtain the checksum-verified Ubuntu host-bootstrap archive,
then explicitly use `-InstallEngine -EngineArchivePath <directory>`. Select one
of `-Engine docker` or `-Engine podman`; on native Ubuntu use `--engine` and
`--install-engine --engine-archive-path`. Ordinary setup installs neither.

Before installation, the helper classifies every local package and runs a
no-download, no-removal APT simulation over the complete transaction. Missing
dependencies, architecture replacements, removals, and downgrades fail before
host mutation. A reviewed downgrade requires a second explicit switch:
`-AllowEngineDowngrades` on Windows or `--allow-engine-downgrades` on native
Ubuntu. Never use that switch without reviewing the printed package list and
APT simulation.

The Windows entry point reads the selected distribution's registered VHDX
location and supplies its actual backing volume to the free-space preflight.
When invoking Linux restore helpers directly inside WSL instead, export
`NILO_WSL_HOST_VOLUME=/mnt/<drive>` with the drive that really contains that
distribution; the tooling intentionally refuses to guess C:.

An exported manifest begins at `pending-round-trip` and is promoted to
`local-round-trip-complete` only after exact import and the complete offline
suite pass. The live `current.json` records that observed transition for the
current local generation; external acceptance remains separate and still uses:

```powershell
.\02_Tools\bootstrap\bootstrap-build-environment.ps1 -Distribution Ubuntu -InstallEngine -Engine docker -EngineArchivePath .\02_Tools\archives\wsl-host-bootstrap -RebuildFromPinnedInputs
```

```sh
./02_Tools/scripts/build-host-bootstrap.sh
./02_Tools/scripts/export-build-environment.sh --engine docker --image nilo/build-environment:noble-20260928-01 --generation noble-20260928-01
```

No checksum, size, digest, hosted capacity, replica, or appliance observation
is accepted until those commands and their independent verification actually
run.

Image builds do not require a pre-emptive source commit. The build command
deterministically hashes every tracked and non-ignored untracked file under
`.gitattributes`, `.gitignore`, the relevant `02_Tools` source, and
`05_Software`, including file content, paths, symlink targets, deletions, and
executable modes where Git honours them. It records the base `HEAD`, the
content-tree SHA-256, and whether the candidate was clean or dirty; runtime,
cache, and archive output trees do not affect the digest. Export refuses an
image when its `io.nilo.source-tree.sha256` label differs from the current
candidate.

Untracked `.sh` entry points are assigned their intended Git mode `100755`
when calculating the candidate identity, independent of Windows
`core.filemode`. Before the eventual human commit, add those task-owned shell
files with `git add --chmod=+x`; `.gitattributes` also forces LF checkout for
shell entry points and container build definitions. This makes the later clean
commit identity match the approved dirty candidate without staging unrelated
files.

After approval and a human-created commit, confirm that the clean committed
tree contains exactly the approved candidate before publication:

```sh
python3 02_Tools/scripts/build_environment.py verify-committed-source \
  --repository-root . --expected-tree-sha256 <approved-candidate-sha256>
```

### Normal build and test

```powershell
.\02_Tools\nilo-tools.ps1 test
.\02_Tools\nilo-tools.ps1 build
.\02_Tools\env\with-build-environment.ps1 shell
```

Native Ubuntu uses the matching POSIX wrapper:

```sh
sh ./02_Tools/nilo-tools.sh test
sh ./02_Tools/nilo-tools.sh build
./02_Tools/env/with-build-environment.sh shell
```

`--image <reference>` verifies a candidate generation that has not yet been
accepted into the archive manifest; the caller is then responsible for having
verified that image's digest itself. Every other invocation checks the
canonical image digest and prints its generation.
The shared CI/local test entry point does not require the source environment
lock to equal the image's embedded copy. Project version bumps therefore reuse
the existing image. Offline preparation verifies current dependency coverage
and package integrity; missing cached dependencies still fail explicitly.
The container has no network, privileges, added capabilities, engine socket, or
credential mount. It receives the repository **read-only** at `/workspace` and
copies it into an isolated container source root at `/work/source`, excluding
`node_modules`, platform virtual environments, build output, and the ignored
archive/cache/runtime trees. Your uncommitted changes are therefore tested, but
the container can never create or replace `node_modules`, a
`.venv-linux-x86_64`, or `dist` in your own checkout. It also receives a
generation-scoped engine volume at `/work/runtime-cache`, seeded on its first
offline run from the image's read-only `/opt/nilo/cache-seed`, and the explicit
ignored output directory `02_Tools/cache/containers/outputs` at `/outputs`,
where declared artifacts such as the GUI production build are published.
Outputs remain editable through UID/GID mapping.
`tests/build-environment/verify-host-isolation.sh` proves this from the host
side after every full verification: it fingerprints the guarded dependency
trees, drops a host-owned sentinel in each, runs the container operations that
previously mutated them, and fails closed on any difference. The mandatory test
command restores locked Go, Python, npm, and Chromium inputs offline, runs
lint/unit/contract/build gates, and performs amd64 and ARM64 Go/C checks.
Browser acceptance runs separately in the privileged appliance stage described
below.

### Export, storage, and replicas

`scripts/export-build-environment.sh` uses deterministic gzip metadata, defaults
to 1 GiB parts, rejects a part size above 2,000,000,000 bytes, and imposes no
total archive cap. It first proves that the current candidate digest matches
the image label, then records the base commit, exact candidate-tree digest, and
clean/dirty status in generated provenance and `current.json`. It generates
`current.json` with `pending-round-trip`, then
the explicit `tests/build-environment/verify-archive-round-trip.sh` command
imports the archive, runs the complete offline verification, preserves its log,
and promotes that status. The personal Gitea remote is the complete authoritative archive;
an independent Git+LFS backup is mandatory. GitHub is a source mirror and may
receive an archive only if the complete generation fits its measured allowance.
A partial GitHub archive is unusable.

Publication is human-operated. First run the non-mutating preflight:

```powershell
.\02_Tools\scripts\mirror-build-environment-lfs.ps1 -ManifestPath .\02_Tools\archives\build-environment\current.json -GiteaCapacityBytes <bytes> -BackupCapacityBytes <bytes>
```

After human publication, acquire each remote into a separate clean cache and
run `tests/build-environment/verify-lfs-replicas.ps1`. Agents never push Git or
LFS. Keep the current and previous accepted generations on Gitea; do not remove
the previous generation before a clean current-generation Gitea restore and
independent backup verification. Recheck capacity/readback quarterly and after
storage migration, and perform a full offline restore twice yearly.

### Acceptance and residual external proof

`tests/build-environment/evidence.example.json` lists BE-01 through BE-10 and
the exact command/evidence expected for each. Copy it to the ignored
`evidence.local.json`, replace pending entries only with observed results, then
run:

```powershell
python .\02_Tools\tests\build-environment\verify-acceptance.py --evidence .\02_Tools\tests\build-environment\evidence.local.json
```

Two clean independent image builds must have matching OCI digests before the
image is called bit-reproducible. Gitea/backup capacity and readback, fresh
clone restores from both configured forges, and the authorized ARM64 Sigrok ABI
smoke require their real external systems; source tests cannot substitute for
them. See `tests/build-environment/README.md` for the evidence sequence.

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
.\02_Tools\nilo-tools.ps1 setup tools
```

The command verifies that the selected distribution already runs as WSL2,
then bootstraps and diagnoses both Windows and Linux toolchains. It does not
install WSL unless explicitly requested:

```powershell
.\02_Tools\nilo-tools.ps1 setup tools -Distribution Ubuntu -InstallWSL
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

These host-local runtimes are available on Linux ARM64 because ARM64 is an
appliance target that has to be built and diagnosed. That is not a claim that
Linux ARM64 is a supported developer host: the reproducible OCI build/test
boundary above accepts only Ubuntu 24.04 x86-64, natively or under WSL2.

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

The current development runtime is Go 1.27.0. Internal tool invocation uses
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

## Browser for the integrated browser test

The acceptance browser gate runs only in the appliance verification stage:

```sh
./02_Tools/tests/appliance/check.sh
```

From Windows, the repository gate invokes that script through the prepared WSL2
distribution. It selects the only installed development distribution
automatically and ignores Docker Desktop's utility distributions; when several
development distributions are installed, set `NILO_WSL_DISTRIBUTION` to the
same distribution prepared by
`bootstrap-development.ps1 -Distribution <name>`. The
stage image pins Node.js and Playwright from `toolchain.versions`, restores the
GUI's exact lockfile, and installs Chromium. It then builds the production GUI
and serves it from Bench Core's appliance-profile HTTPS origin. Chromium enters
through an isolated network namespace and the real management-interface
firewall. Direct `npm run test:e2e` is an internal stage command and refuses to
run without the appliance-stage environment.

The Windows repository-local Chromium installed by
`bootstrap/bootstrap-browser.ps1` remains useful for manual frontend work, but
it is not evidence for the acceptance gate. See
`02_Tools/tests/appliance/README.md` and
`06_SoftwareDocu/Development_and_Simulation_Flow.md`, section 8.3.

## One-command development lifecycle

Once both toolchains are bootstrapped, the local topology - the Go Bench Core
under WSL2 on the simulation profile, and the browser interface development
server on native Windows - starts from one entry point:

```powershell
.\02_Tools\nilo-tools.ps1 dev start
.\02_Tools\nilo-tools.ps1 dev status
```

Each component is launched into **its own visible console window**. Its output
is in that window, and you stop it by pressing Ctrl+C there or closing it.

This command supervises nothing. It records no process identifiers, keeps no
state files anywhere in the repository, and never signals or terminates a
process. That is deliberate: owning long-lived processes across Windows and a
WSL2 distribution means handling identifier reuse and process trees in two
operating system namespaces, and a command that terminates nothing cannot
terminate the wrong thing. There is no `stop` and no `logs` command, because
closing a window and reading a window replace both.

| Option | Applies to | Default | Meaning |
| --- | --- | --- | --- |
| `-Distribution <name>` | both | the only installed distribution | WSL2 distribution running Bench Core |
| `-Scenario <healthy\|unavailable>` | `start` | `healthy` | deterministic simulation scenario |
| `-BackendPort <1-65535>` | both | `8090` | Bench Core loopback port |
| `-FrontendPort <1-65535>` | both | `5173` | interface development-server port |
| `-OpenBrowser` | `start` | off | open the interface once it is answering |
| `-ReadyTimeoutSeconds <n>` | `start` | `180` | how long to wait for each component |
| `-WslExecutable <path>` | both | `wsl.exe` | injected WSL boundary, for tests |
| `-BoundaryScript <path>` | both | none | injected endpoint, port, launcher and browser boundaries, for tests |

The default readiness wait is 180 seconds because the first start compiles the
Go backend, which can take well over a minute on a cold build cache.

`start` reports success only once Bench Core answers on its loopback endpoint
and the interface answers on its port. It starts only what is not already
answering, so running it twice is harmless and running it against a half-up
topology completes it rather than duplicating it. It prints the endpoints, the
selected distribution and scenario, the exact command that issues a development
bearer token, and how to stop each component. It never issues, stores, or prints
a token itself.

A port that is held while its component does not answer is reported as a
conflict, naming the port and the holding process, and nothing is started. That
process is left running.

`status` reports `healthy`, `partially running`, `stopped`, or `port conflict`,
decided purely from what the endpoints answer. Bench Core is recognised by a
`401` to an unauthenticated status request, which proves it is Bench Core
without this command ever holding a token.

Exit codes, for automation:

| Code | Meaning |
| --- | --- |
| `0` | the command succeeded, or `status` found the topology healthy |
| `1` | an unexpected failure |
| `2` | a usage error |
| `3` | a port conflict with a process this command does not own |
| `4` | the topology is stopped |
| `5` | the topology is partially running, or a start did not become ready |

Bench Core's SQLite database, artifacts, and other product runtime state are
kept on the WSL Linux filesystem under `$HOME/.local/state/nilo-development/`,
never under `/mnt/`. A distribution whose home directory resolves to a Windows
drive mount is refused and nothing is started.

## WSL2 development support

- `bootstrap/setup-wsl.ps1` checks the selected distribution and requests
  installation or conversion only with `-Install`.
- `bootstrap/bootstrap-development.ps1` orchestrates Windows and WSL Linux
  bootstrap plus locked Windows GUI dependency restore from the shared
  checkout; `-Offline` is forwarded to every step.
- `bootstrap/bootstrap-frontend.ps1` restores the committed GUI lockfile with
  the pinned Windows Node.js and repository-local npm cache.
- `bootstrap/bootstrap-browser.ps1` installs the pinned browser used for
  optional host-local interface diagnosis below the ignored platform runtime
  directory; appliance acceptance supplies its own browser image.
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

## Sigrok bundle tooling

`build-sigrok-bundle.py` prepares a curated target bundle; `check-sigrok-integration.py` exercises an isolated appliance instance. See [deployment and verification](../06_SoftwareDocu/Sigrok_Integration.md).
