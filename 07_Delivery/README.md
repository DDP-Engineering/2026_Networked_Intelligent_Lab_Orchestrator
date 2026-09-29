# Delivery

Release artifacts, installation bundles, release notes, and the official
documentation that accompanies the application.

## Official documentation

- [CLI reference](../06_SoftwareDocu/CLI_Reference.md) - implemented commands,
  Camera workflows, recording, export and retention.
- [MCP reference](../06_SoftwareDocu/MCP_Reference.md) - authenticated endpoint,
  tool catalog, inputs, permissions and examples.
- [NILO â€” Command Line and Authentication](NILO_CLI_and_Authentication.md) -
  the administrator's reference for the `nilo` command line, the fixed role and
  permission model, first-run setup, browser sessions versus API tokens, the
  HTTP authentication routes, audit coverage, upgrade behavior, and what this
  release does not yet provide.
- [Dual-path management verification](NILO_Dual_Path_Management_Verification.md) -
  the implemented security contract, automated acceptance evidence, base-state
  caveat, and remaining Raspberry Pi hardware checks.


## Current bench deployment

- [2026-09-29 NILO 0.12.0 deployment](NILO_0.12.0_RPi_Current_20260929.md) —
  merged-source validation, Pi installation, HTTPS/API checks and rollback.

## Packaging and deployment decisions

These decisions govern how NILO is packaged and installed. The verified fetch,
cache, exact-target, and offline-preparation foundation lives under
`02_Tools/packaging/`. Its checked-in input lock remains explicitly incomplete,
so final `.deb`, `.run`, and `.img.xz` builders are not available yet. The
current `02_Tools/scripts/build-appliance.sh` and the optional bundle switch
below predate the release packaging flow.

- **Exact target definitions.** The initial targets are
  `ubuntu-24.04-amd64` for Ubuntu 24.04 LTS and
  `rpi-debian13-arm64` for Raspberry Pi OS based on Debian 13. Each target owns
  its OS dependency closure and acceptance result.
- **Connected fetch, offline build.** An explicit connected command fetches
  only lock-declared HTTPS inputs and verifies their sizes and SHA-256 digests
  into the ignored content-addressed cache. Later cache verification opens no
  URL. Package preparation verifies the cache again inside the network-none
  build environment.
- **One package, everything included.** A NILO release is one package per
  target that contains Bench Core, the browser interface, the privileged helper
  and every integration at its current state. NILO is one software;
  integrations are functionality of that software, developed and released with
  it, not separately released products. On the appliance they remain explicit
  to install and enable through the Plugins page, but they ship inside the one
  package and carry every dependency they need.
- **Installation is offline.** Installing from the package needs no internet
  connection. The package is self-contained.
- **Raspberry Pi also gets an image.** Besides the installable package, the
  Raspberry Pi delivery is a complete image that is written directly to a card
  with an external tool such as Rufus. The writing tool is outside NILO's
  scope.
- **A finite tested release matrix.** A newer distribution release is not
  implicitly supported because an offline dependency pool may not be compatible
  with it. Adding an OS release requires a new exact target, dependency pool,
  and acceptance run.
- **Hardware evidence remains physical.** Cross-builds, emulation, cache
  verification, and static checks do not prove that an image boots, networking
  works, GPIO recovery is correctly wired, or an instrument is accessible.
  Preparation receipts keep those checks at `not_run` until separate physical
  target evidence exists.

## Optional integration bundles

Set `NILO_BUNDLE_SIGROK=1` when building an edition to include the sigrok catalog alongside its binaries. The same catalog can be deployed later; installation and enablement remain explicit. See [Sigrok integration](../06_SoftwareDocu/Sigrok_Integration.md).
