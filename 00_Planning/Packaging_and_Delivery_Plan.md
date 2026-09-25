# Packaging and Delivery Plan

**Project:** NILO - Networked Intelligent Lab Orchestrator  
**Status:** Revised implementation plan  
**Scope:** Release packaging, fully offline installation, appliance imaging, upgrade, rollback, removal, and release verification  
**Delivery targets:** Ubuntu 24.04 LTS on `amd64`; Raspberry Pi OS based on Debian 13 on `arm64`  
**Release model:** One project version for Bench Core, GUI, helper, integrations, and packaging

---

## 1. Delivery Contract

A NILO release is a complete application release, not an add-on pack for an
older installation. Every release contains the compatible set of:

- Bench Core and CLI;
- browser GUI;
- privileged network helper;
- systemd units and OS-integration files;
- integration manifests, schemas, adapters, and integration-specific runtimes;
- first-boot and maintenance tooling;
- release documentation, licences, checksums, and software bill of materials.

New integrations and integration capabilities are features of the new NILO
version. They are delivered in the release's local integration catalogue and
remain explicitly installed and enabled through NILO's lifecycle controls. A
release must never silently enable laboratory hardware.

The supported delivery workflow has two separate network boundaries:

1. A connected fetch step downloads every declared, pinned input and verifies
   it before placing it in the local cache.
2. Once the cache is complete, package assembly, verification, and acceptance
   builds run with network access disabled.

The produced installer and Raspberry Pi image must install and run without
internet access. The installer may use the target operating system's existing
base packages, but every additional package needed by NILO must be included for
the exact supported OS release and architecture.

No production release may depend on developer-machine state, WSL state,
language package registries, public APT repositories, or a connected first
boot.

---

## 2. Goals and Non-Goals

### 2.1 Goals

The packaging system shall:

1. Produce a complete, versioned NILO application for each supported target.
2. Produce an offline self-extracting installer for an already-installed OS.
3. Produce a flashable Raspberry Pi appliance image.
4. Make the connected fetch boundary explicit, repeatable, pinned, and
   checksum-verified.
5. Build from the completed cache with networking disabled.
6. Verify the final artifacts in clean target systems with no network route.
7. Upgrade an existing NILO installation without losing supported mutable
   state.
8. Provide a tested rollback path for a failed upgrade.
9. Remove NILO-owned software and networking changes predictably.
10. Record enough provenance, licence information, and checksums to audit and
    reproduce a release.

### 2.2 Non-goals for the initial implementation

The first implementation does not promise:

- a generic installer for every Debian- or Ubuntu-derived distribution;
- one dependency pool that works across multiple OS releases;
- automatic conversion from another network manager to NetworkManager;
- automatic direct downgrade of application or database state;
- hardware enablement merely because compatible hardware is detected;
- reproducible rebuilding of the base Raspberry Pi OS image from source;
- an online updater or background update service;
- public unsigned distribution.

Additional OS releases become supported only after a separate dependency pool
and the full acceptance matrix pass for that exact release and architecture.

---

## 3. Supported Target Matrix

The initial release matrix is deliberately finite.

| Target ID | Hardware | Operating system | Architecture | Required network stack | Deliverables |
|---|---|---|---|---|---|
| `ubuntu-24.04-amd64` | Physical or virtual PC | Ubuntu 24.04 LTS | `amd64` | NetworkManager active and managing the selected adapters | `.run`, internal `.deb` |
| `rpi-debian13-arm64` | Raspberry Pi 4 initially | Raspberry Pi OS based on Debian 13 | `arm64` | NetworkManager active | `.run`, internal `.deb`, `.img.xz` |

The release metadata shall record the exact OS identity used for every build,
including `/etc/os-release`, base image version or digest, architecture, and
kernel used during acceptance.

The installer shall fail before changing the host when:

- the architecture is wrong;
- the OS ID or release is outside the artifact's supported matrix;
- NetworkManager is absent, inactive, or not managing the intended adapters;
- required kernel or device capabilities are missing;
- dependency resolution would remove packages, cross OS releases, change
  architecture, or require a network download;
- there is insufficient space for installation plus rollback material.

A newer OS release is not automatically supported. It may be reported as
unsupported even when it is likely compatible. Supporting it requires a new
release pool and acceptance result.

---

## 4. Artifact Set and Layout

Each release produces the following per supported target:

1. **Internal Debian package**
   - `nilo_<version>_<arch>.deb`
   - Contains NILO-owned binaries, GUI, integration catalogue, embedded
     integration runtimes, service units, configuration templates, licences,
     and metadata.
   - Used as the package-manager-owned application payload inside the offline
     installer and Pi image build.

2. **Offline self-extracting installer**
   - `nilo-<version>-<target>.run`
   - Contains the internal `.deb`, the target-specific OS dependency pool,
     install driver, dependency metadata, and internal manifest.
   - Is the supported delivery artifact for an existing OS installation.

3. **Raspberry Pi appliance image**
   - `nilo-pi4-<version>.img.xz`
   - Contains the supported Raspberry Pi OS base, the installed NILO package,
     included OS dependencies, and boot-time provisioning configuration.
   - Must be bootable and operational without network access.

4. **Release-side metadata**
   - SHA-256 digest file for every deliverable;
   - release manifest;
   - SBOM;
   - third-party licence notices and required corresponding-source offer or
     source bundle;
   - build provenance and input-lock file;
   - operator installation, upgrade, recovery, and removal guide.

Recommended release tree:

```text
release/
  <version>/
    manifest.json
    SHA256SUMS
    provenance.json
    sbom.spdx.json
    THIRD_PARTY_NOTICES/
    source-offer/
    ubuntu-24.04-amd64/
      nilo_<version>_amd64.deb
      nilo-<version>-ubuntu-24.04-amd64.run
    rpi-debian13-arm64/
      nilo_<version>_arm64.deb
      nilo-<version>-rpi-debian13-arm64.run
      nilo-pi4-<version>.img.xz
```

`release/` is generated output and remains ignored by Git. Release artifacts,
joined archives, caches, and temporary staging are not committed or stored in
Git LFS.

---

## 5. Debian Package Definition

### 5.1 Package ownership boundaries

The `.deb` shall use package-managed filesystem paths:

```text
/usr/bin/nilo
/usr/libexec/nilo/nilo-nethelper
/usr/share/nilo/gui/
/usr/lib/nilo/integration-bundles/
/usr/lib/nilo/integration-runtimes/
/usr/lib/systemd/system/nilo-nethelper.socket
/usr/lib/systemd/system/nilo-nethelper.service
/usr/lib/systemd/system/nilo-firstboot.service
/usr/lib/systemd/system/nilo-static.service
/usr/lib/systemd/system/nilo.service
/usr/share/doc/nilo/
/etc/nilo/
```

The exact systemd unit directory shall be discovered and verified during
PKG01 for both target distributions. The package must not install managed
files under `/usr/local`.

Mutable runtime paths remain outside the package payload:

```text
/var/lib/nilo/
/var/log/nilo/
/run/nilo/
```

At minimum, `/var/lib/nilo` contains application state, the identity store,
integration installation state, generated NetworkManager profiles, and other
durable state. The packaging implementation must inventory the current runtime
layout before finalising backup and removal rules.

### 5.2 Service topology

The package installs four units:

| Unit | Account | Purpose | Ordering |
|---|---|---|---|
| `nilo-nethelper.socket` | systemd socket | Owns the privileged helper socket | Before core |
| `nilo-nethelper.service` | root | Runs the narrow privileged network helper | Socket activated |
| `nilo-firstboot.service` | `nilo` | Seeds the first administrator on a new appliance | After local filesystems, before core |
| `nilo-static.service` | `nilo` | Starts Bench Core and serves the built GUI | After NetworkManager and successful first boot |

`nilo-static.service` shall execute the current standalone server contract,
including a loopback status endpoint usable even when no management adapter is
eligible:

```text
/usr/bin/nilo serve \
  --static-dir /usr/share/nilo/gui \
  --management-port 8443 \
  --local-status-bind 127.0.0.1:8091
```

The service must not use `--development-administrator`.

The final units shall include appropriate restart policy, timeout behaviour,
resource limits, filesystem protections, capability restrictions, and write
allow-lists. Hardening must be validated against real NetworkManager and helper
operations rather than copied mechanically from a generic template.

The service account contract must preserve the helper's required
`NILO_CALLER_UID` and `NILO_CALLER_GID` configuration. Package scripts shall
write those values from the actual created `nilo` account into a root-owned,
non-world-writable environment file.

### 5.3 First-boot administrator

The package owns one idempotent command that seeds an administrator from a
root-only credential source:

```text
nilo seed-admin --username <name> --password-file <path>
```

The implementation shall reuse the application's authoritative bootstrap
semantics and password hashing rather than duplicate them in shell or Python.
It shall create the administrator only when the identity store has no users,
perform the change atomically, avoid logging the password, delete or invalidate
the credential source after success, and return success when the requested
state already exists.

For an appliance image, the credential source is created during image
customisation and consumed on first boot. For an existing-host installation,
the operator supplies or generates it explicitly. No production artifact may
contain the development `admin` / `admin_password` credentials.

### 5.4 Maintainer scripts

Maintainer scripts must be idempotent and correct in all supported contexts,
including image construction where systemd is installed but not running.

They may:

- create the locked `nilo` system user and group;
- create mutable directories with explicit ownership and modes;
- install configuration defaults without overwriting local configuration;
- run versioned state migrations only in the controlled upgrade transaction;
- enable units for the next boot;
- request a daemon reload when systemd is actually running.

They must not:

- start services during an image-build chroot;
- assume `/run/systemd/system` exists;
- create a production identity or live database while building an image;
- make network requests;
- configure laboratory adapters without an explicit operator selection;
- manually delete package-owned files in `postrm`.

The image builder shall install a temporary `policy-rc.d` that denies service
starts, remove it before sealing the image, and verify that no NILO processes
or sockets were created in the chroot.

---

## 6. Offline OS Dependency Contract

### 6.1 Target-specific dependency pools

Each `.run` carries a dependency pool produced for exactly one target ID. The
pool shall include every non-base OS package that the internal `.deb` requires,
including recursive dependencies that are not guaranteed by the documented
base installation.

The initial dependency inventory is expected to cover at least:

- NetworkManager and required command-line tools;
- nftables;
- Avahi and mDNS libraries or tools;
- the distribution-appropriate dnsmasq package or binary arrangement that does
  not enable a competing system service;
- SQLite runtime support;
- libsigrok and driver dependencies when not embedded in the integration
  runtime;
- utility packages used by maintainer, health-check, or recovery scripts.

PKG01 must determine the exact package names and decide whether dnsmasq and
sigrok components are installed as OS packages or embedded, relocatable
integration runtime assets. That decision shall be consistent per target and
captured in the lock file.

### 6.2 Dependency collection

The connected fetch step shall:

1. start from a clean container or VM matching the target OS release and
   architecture;
2. use pinned repository snapshots or record repository metadata and package
   versions;
3. resolve the complete install transaction for the documented base image;
4. download all required `.deb` files without installing them on the build host;
5. verify repository signatures and record each file's SHA-256 digest;
6. generate local APT metadata for the pool;
7. store the files in the content-addressed cache;
8. emit a lock file that names every package, version, architecture, origin,
   and digest.

A cache-completeness command shall fail if any locked input is absent, changed,
or unverified.

### 6.3 Offline installation

The installer shall configure APT to use only the extracted local pool for the
transaction. Before making changes it shall simulate the transaction and refuse
to proceed if APT proposes:

- a network fetch;
- removal of an existing package;
- a cross-release package;
- a foreign architecture;
- an unlisted downgrade;
- an unauthenticated or digest-mismatched file.

The real transaction shall run with acquisition networking disabled. Successful
installation is not sufficient evidence: acceptance must show that packet
capture or namespace isolation observed no external network attempt.

---

## 7. Production Integration Packaging

### 7.1 Bundled catalogue versus installed integrations

The immutable release catalogue lives under
`/usr/lib/nilo/integration-bundles`. The active installed integration state
lives under `/var/lib/nilo/integrations`.

A package upgrade therefore updates the catalogue, not the active integration
copy directly. The current lifecycle behaviour treats an already installed
integration as complete, so packaging alone cannot update an installed
integration. Before release delivery, the product needs an explicit integration
upgrade operation.

For every bundled integration changed by a release, upgrade shall:

1. compare the installed integration version and digest with the new catalogue;
2. preserve enabled/disabled state and selected operating mode;
3. refuse or wait while related jobs are running;
4. stage the new files in a separate directory;
5. verify manifest, schema, runtime, and file policy;
6. perform any versioned integration-state migration;
7. atomically switch the active copy;
8. run the integration health check;
9. roll back the active copy and state migration if verification fails;
10. append an audit record.

New integrations remain present in the catalogue but uninstalled and disabled
until the operator explicitly installs and enables them.

### 7.2 Self-contained runtime

Each production integration bundle shall contain everything not supplied by the
supported base OS or the package dependency contract:

- `integration.json`;
- the real adapter;
- JSON schemas and static resources;
- a standalone runtime where needed;
- libsigrok tools and shared libraries where needed;
- driver and firmware assets whose redistribution permits inclusion;
- a runtime-local launcher that sets only confined library and module paths;
- integration-specific licence notices and provenance.

The contract runner under `05_Software/tests/fixtures/` is test-only and must
never enter a production release.

The Sigrok integration is a release blocker on both architectures. If a
redistributable, validated runtime cannot be produced for `amd64` and `arm64`,
the release is incomplete; removing it from only one target is not the default
fallback.

### 7.3 Archive and runtime limits

The current integration verifier limits archive index size, file count, and
total bytes, and rejects symbolic links. A standalone CPython runtime and
Sigrok closure may exceed those limits or require safe relative links.

PKG04 shall first measure the real runtime closure for each architecture. The
product shall then either:

- fit the runtime into the existing limits without links; or
- make a reviewed core change that raises bounded limits and permits only safe
  relative links whose resolved targets remain inside the integration root.

The installer and runtime copier must reject path traversal, absolute links,
escaping links, device nodes, sockets, and FIFOs. Verification limits must be
derived from measured production bundles with explicit headroom, not disabled.

---

## 8. Connected Fetch and Offline Build Boundary

### 8.1 Script layout

Repository-local tooling belongs under `02_Tools/packaging/`:

```text
02_Tools/packaging/
  config/
    targets.json
    inputs.lock.json
  fetch/
    fetch-all.ps1
    fetch-all.sh
    fetch-go.sh
    fetch-node.sh
    fetch-python.sh
    fetch-sigrok.sh
    fetch-os-packages.sh
    fetch-pi-base.sh
  build/
    assert-cache-complete.sh
    assert-network-blocked.sh
    build-gui.sh
    build-go.sh
    build-runtimes.sh
    build-deb.sh
    build-offline-installer.sh
    build-pi-image.sh
    build-release.sh
  verify/
    verify-inputs.sh
    verify-deb.sh
    verify-installer.sh
    verify-image.sh
    verify-release.sh
```

PowerShell is the Windows orchestration entry point; actual Linux package and
image construction runs in a clean Linux environment. Cross-compilation may
produce Go binaries, but target runtime and OS package acceptance must execute
on the target architecture or under a documented, validated emulation path.

### 8.2 Fetch phase

The fetch phase may access the network. It must pin and checksum all external
inputs, including:

- Go modules and toolchain;
- Node packages and runtime used for the GUI build;
- standalone Python runtimes and Python wheels;
- Sigrok source or binary inputs;
- OS dependency packages and repository metadata;
- Raspberry Pi OS base image;
- SBOM and licence-scanning tool inputs when those are part of the build.

An input without a verified origin, version, digest, and applicable licence
record cannot enter the release cache.

### 8.3 Offline build phase

After cache completion, the release build shall run in a clean environment
with networking disabled and with only these inputs mounted:

- repository checkout at the release commit;
- verified cache;
- empty staging and output directories;
- explicitly declared signing material, if signing is enabled.

The build shall fail on any undeclared download attempt or cache miss.

To make project-owned artifacts reproducible, scripts shall set
`SOURCE_DATE_EPOCH` from the release commit, normalise archive order, timestamps,
ownership, permissions, locale, and timezone, and remove nondeterministic
temporary paths. The release manifest shall store deterministic source time
separately from the human publication time.

The release entry point shall verify:

- the checkout is at an identified commit;
- the working tree policy for release builds is satisfied;
- root `VERSION`, Go build information, GUI package version, lockfile version,
  changelog, and artifact names agree;
- no development credential or simulation fixture is present;
- no artifact includes a build-host absolute path.

---

## 9. Offline Self-Extracting Installer

### 9.1 Container format

The `.run` artifact contains:

1. a small POSIX shell launcher;
2. a compressed payload;
3. an internal manifest with size and SHA-256 for every member;
4. the internal NILO `.deb`;
5. the exact target dependency pool and APT metadata;
6. installer and health-check scripts;
7. target, version, and compatibility metadata.

The installer shall extract into a root-owned temporary directory created with
safe permissions. Extraction must reject absolute paths, `..` traversal,
device nodes, sockets, FIFOs, and escaping links before any payload script is
executed.

### 9.2 External integrity

Internal payload hashes detect extraction corruption only after the launcher is
trusted. Every release must therefore publish a SHA-256 digest of the complete
`.run` and `.img.xz` through a separate trusted channel.

For internal delivery, independent SHA-256 verification is the minimum initial
policy. Before public or customer distribution, artifacts and release metadata
must be signed and the installation guide must define how the verification key
is obtained and rotated.

### 9.3 Installer transaction

The `.run` installer shall:

1. require root and acquire a single installation lock;
2. verify its external digest according to operator procedure;
3. extract safely and verify the complete internal manifest;
4. validate target OS, release, architecture, NetworkManager, capabilities,
   storage, and package transaction;
5. discover current NILO version and state schema;
6. stop and quiesce NILO in the defined order;
7. checkpoint the SQLite database and create the rollback snapshot;
8. install OS dependencies from only the local pool;
9. install or upgrade the internal `.deb`;
10. run application and integration migrations;
11. start the helper and application units;
12. wait for the loopback status endpoint;
13. validate the database, helper socket, GUI assets, integration catalogue,
    and NetworkManager prerequisites;
14. commit the upgrade and apply snapshot retention;
15. on failure, restore packages and state according to the rollback contract
    and report the retained recovery location.

The install driver must not configure an adapter, enable an integration, or
enable Raspberry Pi recovery unless the operator selected that action
explicitly.

---

## 10. Raspberry Pi Appliance Image

### 10.1 Image construction

The image builder shall:

1. verify the pinned Raspberry Pi OS base image digest;
2. decompress into staging;
3. enlarge the image and root filesystem with checked free-space margins;
4. mount with a trap-based cleanup path;
5. install `policy-rc.d` to block service starts;
6. install target OS dependencies from the offline pool;
7. install the NILO `.deb`;
8. configure NetworkManager as the supported network stack;
9. enable NILO units for first boot without starting them;
10. create the one-time root-only administrator credential source;
11. remove host keys, machine ID, package caches, logs, temporary files, and
    build-specific identifiers that must be unique at boot;
12. remove `policy-rc.d`;
13. unmount, check the filesystem, shrink when supported, compress, checksum,
    and verify the image.

The builder must fail if unmounting is incomplete or if a NILO process started
inside the image.

### 10.2 First boot

First boot shall:

1. expand the root filesystem using the distribution-supported mechanism;
2. generate unique host identity and SSH host keys when SSH is enabled;
3. consume the one-time administrator credential;
4. start the helper and NILO service;
5. expose the local status endpoint immediately;
6. expose the management plane only through NILO's adapter-eligibility rules;
7. leave integrations and hardware recovery disabled until explicitly enabled.

### 10.3 Recovery GPIO

GPIO-based recovery is opt-in. Hardware detection alone is not consent to claim
pins.

The image and package ship recovery disabled. The installer or operator may use
an explicit option such as:

```text
--enable-pi-recovery
```

The option shall display or document the exact BCM pin mapping, active level,
bias, hold time, electrical assumptions, and conflict risk. Acceptance requires
a confirmed wiring fixture and physical Raspberry Pi test. Incompatible models
or unavailable GPIO libraries must leave the feature disabled without blocking
normal NILO startup.

---

## 11. Upgrade, Rollback, and State Preservation

### 11.1 State classification

PKG01 shall publish a state inventory with one of these classifications for
every path:

- **Irreplaceable mutable state:** database, identities, credentials, audit
  records, local configuration, and operator-managed integration state.
- **Regenerable mutable state:** generated NetworkManager profiles and cached
  discovery data where regeneration is proven safe.
- **Immutable package state:** binaries, GUI, units, catalogue, embedded
  runtimes.
- **Operational artifacts:** logs, job outputs, captures, and exports, each
  with an explicit retention policy.

Backups shall not blindly copy all of `/var/lib/nilo`; they shall capture the
defined irreplaceable set and any other data required for a consistent restore.

### 11.2 Consistent snapshot

Before upgrade, while services are stopped, the installer shall:

- wait for active jobs or fail with a clear force policy;
- stop core before the privileged helper;
- checkpoint SQLite and verify database integrity;
- calculate required backup and installation space;
- create a versioned snapshot on the same protected filesystem unless an
  operator-selected location is used;
- preserve owners, groups, modes, ACLs, extended attributes, and secret-file
  restrictions;
- write a snapshot manifest containing source version, schema version, target
  version, and file digests;
- fsync critical files and directories before proceeding.

Rollback restarts the previous package set only after restoring its matching
state snapshot and checking database integrity.

### 11.3 Downgrade policy

An in-place direct downgrade is refused by default. The supported reverse path
is restoration of a snapshot created for the exact prior application and schema
version, or restoration from an operator backup under a documented recovery
procedure.

Snapshot retention is configurable; the default shall keep the last two
successful upgrade snapshots plus the currently active state, subject to a
preflight disk-space check.

---

## 12. Removal and Network Cleanup

Removal has two explicit modes:

- **Remove software, preserve state** - uninstall package-owned files and stop
  services, but retain documented mutable state for reinstall or recovery.
- **Purge NILO** - remove NILO-owned mutable state and network configuration
  after local confirmation and, by default, creation of an export or backup.

Cleanup shall inventory and handle all NILO-owned external effects:

- NetworkManager connection profiles;
- nftables tables and chains;
- dnsmasq processes and configuration;
- Avahi or SSDP advertisements;
- helper socket and runtime files;
- commissioning and recovery state;
- integration subprocesses.

Names and metadata for NILO-owned network objects must be collision-resistant
and attributable to NILO. Cleanup must never delete an object solely because it
resembles a NILO object.

Remote removal shall not tear down the operator's only management path without
an explicit local or out-of-band confirmation policy. Package-manager
maintainer scripts must remain non-interactive; destructive purge confirmation
belongs in the supported `nilo` removal command or installer driver.

---

## 13. Supply-Chain, Licence, and Provenance Requirements

Every release shall include:

- an SPDX SBOM covering NILO, language dependencies, embedded runtimes, shared
  libraries, OS packages, firmware, and base-image identity;
- third-party copyright and licence notices;
- source, written offer, or other corresponding-source material required by
  redistributed GPL/LGPL components, including Sigrok components;
- hashes and origin URLs for fetched inputs;
- build environment identity and tool versions;
- release commit and project version;
- test and acceptance result identifiers.

Licence review is a release gate, not post-release documentation work.
Redistribution of firmware, Python, Node assets, and Sigrok drivers must be
confirmed before they are added to the cache.

Secrets, signing keys, developer credentials, `.git`, editor files, caches,
and simulation-only fixtures are forbidden in release payloads.

---

## 14. Required Product Changes Before Packaging Can Complete

Packaging work may begin immediately, but the following are product
prerequisites for a releasable artifact:

1. Add a production-safe first-administrator seeding command.
2. Add an explicit, atomic integration upgrade path for already installed
   integrations.
3. Measure production runtime bundles and revise integration archive/copy rules
   if required for size or safe relative links.
4. Add or confirm loopback-local health status for installer validation.
5. Define versioned application and integration state migrations with rollback
   boundaries.
6. Define a supported removal command that can safely inventory and undo
   NILO-owned networking changes.
7. Confirm the server's externally configurable management port and production
   static-GUI behaviour.
8. Remove every dependency on test-only runtimes and development credentials
   from production paths.

These changes follow the repository's normal software-development process. The
release packaging, version assignment, dependency fetch, artifact assembly,
and delivery operations remain repository/package operations.

---

## 15. Verification and Acceptance Matrix

### 15.1 Static verification

Every build runs:

- shell linting and strict-mode checks;
- PowerShell analysis for Windows entry points;
- Debian package metadata and filesystem-policy checks;
- systemd unit verification;
- archive traversal and special-file tests;
- manifest and checksum validation;
- secret and development-fixture scan;
- SBOM and licence completeness checks;
- version consistency checks;
- deterministic rebuild comparison for project-owned artifacts.

### 15.2 Clean offline installation

For each target:

1. begin with the documented clean base OS;
2. disconnect the external network or run in a namespace with no route;
3. run the `.run` installer;
4. prove that no download or external connection was attempted;
5. reboot;
6. verify services, loopback status, GUI, helper, authentication, and catalogue;
7. install and enable each production integration from the local catalogue;
8. execute its real-runtime health check.

### 15.3 Upgrade and rollback

For each supported source version:

1. create representative users, configuration, integrations, audit entries,
   and job artifacts;
2. run a successful offline upgrade;
3. verify preserved state and upgraded integrations;
4. inject failures at dependency install, package install, migration, service
   start, and health-check stages;
5. verify rollback restores the prior package and matching state;
6. power-interrupt selected transaction boundaries and verify recovery or a
   precise operator recovery instruction.

The supported source-version matrix must be declared per release. "Upgrade from
any historical version" is not an implicit promise.

### 15.4 Raspberry Pi hardware acceptance

On a physical Raspberry Pi 4:

- flash and boot the final compressed image;
- verify filesystem expansion and unique host identity;
- verify cold boot, reboot, and power-cycle behaviour;
- verify local administrator creation and credential consumption;
- verify NetworkManager ownership of selected adapters;
- verify the UI and all bundled integrations;
- verify recovery GPIO only with explicit enablement and confirmed wiring;
- verify the system remains usable when recovery GPIO is disabled or
  unavailable.

### 15.5 Acceptance criteria

A release is acceptable only when:

- all artifacts build from a complete cache with network blocked;
- all deliverables install and operate with network blocked;
- both target architectures contain all promised production integrations;
- upgrade and injected rollback tests pass for the declared source versions;
- Raspberry Pi hardware acceptance passes;
- external and internal checksums verify;
- SBOM, licence, source-obligation, provenance, and operator documents are
  complete;
- no unresolved release-blocking decision remains.

---

## 16. Work Packages

### PKG01 - Delivery Inventory and Contracts

**Tasks**

1. Inventory built binaries, GUI assets, state paths, network side effects,
   runtime users/groups, and systemd dependencies.
2. Finalise the exact target OS images and base-package assumptions.
3. Resolve target package names and the dnsmasq/Sigrok packaging strategy.
4. Classify mutable state and define removal ownership markers.
5. Measure free-space requirements.

**Exit criteria**

- Target matrix, payload map, dependency map, state inventory, and cleanup
  inventory are reviewed and version-controlled.

### PKG02 - Reproducible Build Inputs

**Tasks**

1. Build release-mode Go and GUI artifacts for both architectures.
2. Add version consistency and clean-source checks.
3. Normalise deterministic metadata.
4. Produce initial SBOM and licence inventory.

**Exit criteria**

- Project-owned artifacts rebuild byte-for-byte from the same inputs, or every
  documented nondeterministic field has an approved normalisation plan.

### PKG03 - Verified Fetch Cache

**Tasks**

1. Implement pinned fetchers for language dependencies, integration runtimes,
   OS packages, and Pi base image.
2. Generate lock files, checksums, local APT metadata, and provenance.
3. Implement cache-completeness verification.
4. Demonstrate a network-blocked rebuild from the cache.

**Exit criteria**

- A fresh machine can fetch once, transfer the cache, and complete the build
  without network access.

### PKG04 - Self-Contained Production Integrations

**Tasks**

1. Produce real standalone Python and Sigrok runtime closures for both
   architectures.
2. Measure and resolve archive-limit and link-policy conflicts.
3. Add the explicit integration upgrade transaction.
4. Verify health checks against real runtimes.
5. Complete integration licence and source obligations.

**Exit criteria**

- Every production integration installs, upgrades, rolls back, and passes its
  health check on both targets with no system Python or network dependency.

### PKG05 - First Boot, Health, and Removal Primitives

**Tasks**

1. Implement the atomic administrator seed command.
2. Confirm loopback health semantics.
3. Implement safe NILO-owned network cleanup and removal modes.
4. Define application and integration migration boundaries.

**Exit criteria**

- Package and installer scripts can invoke stable application-owned primitives
  rather than reimplementing domain logic.

### PKG06 - Debian Package

**Tasks**

1. Create filesystem layout, metadata, conffiles, users, permissions, and units.
2. Implement idempotent maintainer scripts and image-chroot behaviour.
3. Integrate immutable catalogue and runtime bundles from PKG04.
4. Validate install, upgrade, remove, purge, and reinstall.

**Exit criteria**

- The `.deb` passes package checks and lifecycle tests on both targets.

### PKG07 - Offline Installer

**Tasks**

1. Implement safe self-extraction and manifests.
2. Integrate exact target dependency pools.
3. Implement preflight, quiesce, snapshot, migration, health, and rollback.
4. Add external checksum and optional signing workflow.

**Exit criteria**

- Clean offline install and failure-injected upgrade/rollback pass on both
  targets.

### PKG08 - Raspberry Pi Image

**Tasks**

1. Build from the pinned base image without service starts.
2. Configure first boot, NetworkManager, filesystem growth, and opt-in GPIO.
3. Sanitize image identity and transient state.
4. Verify final image structure and boot on physical Pi.

**Exit criteria**

- The final `.img.xz` passes offline physical-hardware acceptance.

### PKG09 - Release Gate and Delivery Documentation

**Tasks**

1. Run the complete target/source-version acceptance matrix.
2. Finalise SBOM, notices, corresponding-source obligations, provenance, and
   checksums.
3. Document install, upgrade, rollback, recovery, removal, and verification.
4. Produce final release manifest and artifact index.

**Exit criteria**

- A release reviewer can trace every artifact to verified inputs and reproduce
  the documented offline installation and recovery procedures.

Recommended order:

```text
PKG01 -> PKG02 -> PKG03
PKG01 -> PKG04 -> PKG05 -> PKG06 -> PKG07
PKG03 --------------------^       -> PKG08
PKG07 + PKG08 --------------------> PKG09
```

PKG04 and PKG05 are intentionally before final package and installer work:
the package cannot correctly deliver upgraded integrations or first-boot state
until those product contracts exist.

---

## 17. Decisions Required Before Implementation Is Declared Ready

The following decisions shall be resolved in PKG01-PKG05 and recorded in the
plan or an associated design note:

1. Exact Raspberry Pi OS image release and digest.
2. Minimum Raspberry Pi model and storage size.
3. Exact list of supported upgrade source versions.
4. Whether Sigrok is fully embedded or partly supplied through target OS
   packages on each target.
5. Final integration verifier limits based on measured runtime closures.
6. Initial artifact signing mechanism and verification-key distribution.
7. Installer behaviour when active jobs do not quiesce within the timeout.
8. Snapshot location, retention, and minimum free-space formula.
9. Initial administrator credential delivery mechanism.
10. Whether SSH is enabled in the Pi appliance and how access is provisioned.
11. Recovery GPIO pin assignment and electrical contract.
12. Support policy for virtualised Ubuntu installations and USB device
    passthrough.

None of these decisions may silently expand the stated support matrix.

---

## 18. Principal Risks

| Risk | Consequence | Mitigation |
|---|---|---|
| Dependency pool built against a different OS release | Offline install conflicts or fails | Exact target IDs, clean resolver roots, simulated transaction, acceptance on base image |
| Installed integration remains stale after package upgrade | Core and adapter contract mismatch | Explicit atomic integration upgrade with health check and rollback |
| Embedded runtime exceeds verifier limits | Production integration cannot install | Measure closures early; bounded reviewed verifier changes |
| Backup is taken while services write state | Corrupt or inconsistent rollback | Quiesce, SQLite checkpoint, integrity check, manifest, fsync |
| Image chroot starts services or creates identity | Cloned or contaminated appliances | `policy-rc.d`, systemd-aware scripts, image sanitation |
| Automatic GPIO activation claims user hardware | Electrical conflict or unsafe recovery | Disabled by default; explicit enablement and wiring acceptance |
| Internal checksum is mistaken for publisher authenticity | Tampered launcher can bypass checks | Whole-artifact hash out of band; signing before public delivery |
| GPL/LGPL or firmware obligations are missed | Distribution cannot legally ship | Licence gate, SBOM, source bundle/offer, provenance |
| Network cleanup removes unrelated configuration | Host connectivity damage | Stable ownership markers, inventory-based cleanup, management-path protection |
| "Newer OS" is treated as supported without testing | Unbounded compatibility failures | Finite matrix; new pool and acceptance per OS release |
| One architecture lacks a real Sigrok runtime | Feature-incomplete NILO release | Both architecture closures are a release gate |
| Build timestamp or host paths vary | Releases cannot be reproduced | `SOURCE_DATE_EPOCH`, normalisation, clean isolated build |

---

## References

- `AGENTS.md`
- `01_Documentation/Code_Style.md`
- `00_Planning/Development_Build_Environment_Plan.md`
- `06_SoftwareDocu/Development_and_Simulation_Flow.md`
- `06_SoftwareDocu/Network_Operations.md`
- `06_SoftwareDocu/Local_Hardware_Access.md`
- `06_SoftwareDocu/Sigrok_Integration.md`
- `VERSION`
- `CHANGELOG.md`
- `05_Software/core/cmd/nilo/main.go`
- `05_Software/core/cmd/nilo-nethelper/`
- `05_Software/core/internal/bench/packages.go`
- `05_Software/core/internal/httpapi/server.go`
- `05_Software/core/internal/integration/`
- `05_Software/core/internal/nethelper/`
- `05_Software/integrations/`
