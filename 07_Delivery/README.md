# Delivery

Release artifacts, installation bundles, release notes, and the official
documentation that accompanies the application.

## Official documentation

- [NILO — Command Line and Authentication](NILO_CLI_and_Authentication.md) -
  the administrator's reference for the `nilo` command line, the fixed role and
  permission model, first-run setup, browser sessions versus API tokens, the
  HTTP authentication routes, audit coverage, upgrade behavior, and what this
  release does not yet provide.
- [Dual-path management verification](NILO_Dual_Path_Management_Verification.md) -
  the implemented security contract, automated acceptance evidence, base-state
  caveat, and remaining Raspberry Pi hardware checks.


## Packaging and deployment decisions

These decisions govern how NILO is packaged and installed. The packaging
scripts that implement them are not written yet; the current
`02_Tools/scripts/build-appliance.sh` and the optional bundle switch below
predate them.

- **Separate scripts per target.** The Raspberry Pi delivery and the Ubuntu
  mini PC delivery are produced by separate packaging scripts. Each script is
  self-sufficient from the repository: anything it needs from outside is
  fetched and verified once when the script is created, and a later run needs
  no network.
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
- **A minimum release, and everything newer.** NILO is not bound to one Ubuntu
  release or one Raspberry Pi OS release. Each target declares a minimum
  supported release and supports every newer release. The baseline named
  elsewhere in this documentation is Ubuntu 24.04 LTS and Raspberry Pi OS based
  on Debian 13; the packaging work confirms or revises it. A package therefore
  relies on the distribution's own packages for system dependencies and refuses
  only a release below the minimum; it never pins one release.

## Optional integration bundles

Set `NILO_BUNDLE_SIGROK=1` when building an edition to include the sigrok catalog alongside its binaries. The same catalog can be deployed later; installation and enablement remain explicit. See [Sigrok integration](../06_SoftwareDocu/Sigrok_Integration.md).
