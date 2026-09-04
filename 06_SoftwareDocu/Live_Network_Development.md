# Live Network development

The explicit Linux `appliance` profile now uses NetworkManager for live
interface observations and runtime enable/disable. Bench Core runs as an
unprivileged user; a separately installed root helper owns changes and recovery.
The simulation launcher still uses the isolated simulated provider.

This is a development checkpoint. Live WireGuard profile management,
power-cycle recovery acceptance and Raspberry Pi hardware acceptance remain.
The appliance's instrument inventory is empty; it contains no simulated bench.

WireGuard readiness is now read through the helper: NetworkManager availability
and version, running-kernel support and the required `wg` tool/version. The probe
uses D-Bus without service auto-start, kernel metadata and `/usr/bin/wg --version`
with a fixed environment, two-second timeout and bounded output. It never loads
modules, invokes `wg-quick`, reads tunnel keys or creates a connection.

Kernel support needs evidence from the loaded module, the running kernel's
configuration or its installed module metadata; a kernel version number is not
enough. Missing metadata (common in containers) makes the observation inconclusive,
with `reported: false` and a diagnostic. A missing `wg` is reported explicitly
with `installed: false` and an empty version. Only `wg` is required by NILO;
`wg-quick` is neither required nor probed.

Live replies carry `readiness_only: true`: profiles and handshake state have not
been inventoried. The GUI shows this distinction and offers no profile controls;
CLI and MCP receive the same marker. `can_install` is true when missing tools can
be installed on Debian 13 or Ubuntu 24.04 with a supported repository configuration.
These checks do not prove tunnel activation or peer reachability.

## Acknowledged tool installation

On a supported disposable appliance, open Network, choose **Install WireGuard
tools**, review the proposal and select **Apply**. Installation has no connectivity
confirmation countdown. The page reports refresh, transaction planning, download,
installation and verification progress, and blocks competing network changes.
The CLI can use `nilo network vpn install`; its returned operation ID can be read
with `nilo network operation OPERATION_ID` using the same server/token options.

The helper runs a bounded ten-minute job independent of the requesting browser.
Its private `install.json` stores the last job's progress and result. Restart
marks an interrupted job failed without replaying it. Bench Core operation history
and browser reattachment after reload remain unimplemented.

Only `wireguard-tools` is requested, with recommendations, upgrades and removals
disabled. A fresh signed index update precedes candidate selection; insecure,
weak or expiry-bypassing source overrides are refused. The helper uses its own
APT configuration without site hooks, preserves configured sources/keyrings, pins
the selected package version, and verifies the actual locked transaction in a
fixed pre-install hook before dpkg runs. Any unrelated install, configuration,
upgrade or removal is refused. Repository output is bounded and never sent to
the API; lock, offline, authentication and package-state failures have fixed
actionable messages. Existing tooling is verified without a package operation.
Kernel replacement, distribution upgrades and reboot are never requested.

Installing packages requires the root helper's systemd service to write package
payloads under `/usr` and `/etc`, APT/dpkg/systemd state and package logs, and to
contact repositories. The installer declares those writable paths and network
families explicitly; `/boot` and home directories remain protected. This service
configuration still requires VM/Pi acceptance; container tests run the helper
directly. No development host or Raspberry Pi package is changed by the tests.

## Run the isolated Linux test

From the Windows repository root, with the pinned toolchain and Docker already
available in the repository's Ubuntu-24.04 WSL distribution:

```powershell
wsl -d Ubuntu-24.04 -- sh -lc 'sh /mnt/f/2026_Networked_Intelligent_Lab_Orchestrator/02_Tools/tests/network/check.sh'
```

For the Go race detector, set `NILO_NETWORK_RACE=1` before `sh` inside the quoted
Linux command. The harness builds a pinned Debian image, creates a dedicated
IPv6-capable Docker network and runs NetworkManager on a private system bus.
The repository is mounted read-only. Its `NET_ADMIN` capability applies only
inside that container; no host network namespace or system bus is mounted.
The test container and network are removed afterwards; the reusable image and
`nilo-network-test-go-cache` Docker volume retain build dependencies.

The fixture uses Debian NetworkManager 1.52.1. The tests exercise real D-Bus observations, dual-stack addresses/routes/DNS,
disable and restore, an actual 120-second NetworkManager timeout without a
helper timer, lost-checkpoint reconstruction and persistent recovery state.
The process test runs the actual root helper and an unprivileged HTTPS backend,
signs in, verifies acknowledged installation of already-present tooling through
HTTPS, confirms disable/enable and restarts a killed backend with a pending
change. TLS verification uses a trusted test certificate, without a bypass.
Allow several minutes for compilation and the two-minute timeout.

The image starts without WireGuard. Before replacing its online route, the harness
checks held package locks and offline repositories, installs through the helper
socket, checks observable progress and fresh readiness, and compares the complete
package inventory. A second run must leave packages unchanged. Set
`NILO_NETWORK_DISTRO=ubuntu24 NILO_INSTALL_ONLY=1` to run the same installation
checks in the pinned Ubuntu 24.04 image. Debian 13 remains the default full fixture.

Ordinary Windows and Linux unit suites skip these opt-in system tests. Their
pass result does not stand in for the separate command above.

## Start the appliance interface on a disposable Linux machine

Use an existing non-root service account, a NetworkManager-managed secondary
interface and an existing saved connection. Retain a separate console while
testing connectivity changes. Do not install this helper on the development
host simply to run the test suite.

1. Build the GUI on Windows, from `05_Software/nilo_gui`:

   ```powershell
   & ../../02_Tools/env/with-tools.ps1 npm run build
   ```

   Copy the resulting `dist` directory to the disposable machine, for example
   `/opt/nilo/gui`. No endpoint needs to be embedded: requests use the origin
   that served the page, and Bench Core adds its selected runtime to the HTML.

2. Build the Linux binaries from `05_Software/nilo_assistant`, using the pinned
   Linux toolchain:

   ```sh
   ../../02_Tools/env/with-go.sh go build -o /tmp/nilo ./cmd/nilo
   ../../02_Tools/env/with-go.sh go build -o /tmp/nilo-nethelper ./cmd/nilo-nethelper
   ```

3. Explicitly install the helper on that disposable machine. From its repository
   root, where `nilo` is the existing service account:

   ```sh
   sudo sh 02_Tools/scripts/install-nethelper.sh /tmp/nilo-nethelper nilo
   ```

   This installs `/usr/local/libexec/nilo-nethelper`, creates a root-only
   `/var/lib/nilo-nethelper` directory, enables `nilo-nethelper.service` and
   enables `nilo-nethelper-recovery.timer`, the independent recovery pass
   described below. It requires an existing NetworkManager service and does
   not install or replace the host's network manager. Device-network
   forwarding needs `nftables` on the host (`apt install nftables`); without
   it a device network stays saved and is reported as unsupported. The
   installer's units have not yet been exercised on a booted systemd machine;
   the container test runs the binary and its `recover` pass directly.

   For a Raspberry Pi, build both binaries for `linux/arm64` (and x86-64 for a
   disposable machine) with the pinned toolchain:

   ```sh
   sh 02_Tools/scripts/build-appliance.sh
   ```

   The result lands under `05_Software/nilo_assistant/dist/linux-<arch>/`.

4. Provision an operator-trusted PEM certificate whose SAN covers the selected
   private IP/DNS name. Its key must be readable by the service account and
   inaccessible to other users, for example `root:nilo` mode `0640`. Keep TLS
   keys and runtime state outside the GUI directory. Trust the issuing CA in
   the browser; the CLI supports `--ca-file` and has no TLS bypass.

5. As the service user, create the administrator once. The command reads and
   confirms the password from standard input:

   ```sh
   /tmp/nilo admin bootstrap --data-dir /var/lib/nilo --username administrator
   ```

   The data directory must already be writable by that user. The published
   simulation `admin` password is refused as a startup convenience in this
   profile: `--development-administrator` is simulation-only.

6. As that same service user, substitute the machine's actual private address
   and certificate paths, then start:

   ```sh
   /tmp/nilo serve --profile appliance \
     --data-dir /var/lib/nilo --bind 192.168.10.24:8443 \
     --gui-dir /opt/nilo/gui \
     --tls-cert /etc/nilo/tls/server.crt --tls-key /etc/nilo/tls/server.key \
     --nethelper-socket /run/nilo/nethelper.sock
   ```

   Open `https://192.168.10.24:8443` and sign in with the administrator created
   above. The GUI, authenticated API and MCP share that origin. Appliance
   startup requires TLS and built GUI assets; public/wildcard binds are refused.

## Recovery and current limits

Before disconnecting, the helper atomically writes and fsyncs root-only
`recovery.json`, then arms a target-specific NetworkManager checkpoint.
NetworkManager owns the independent 120-second timer. Helper and backend startup
reconcile pending recovery before accepting management. Confirmation destroys
only that operation's checkpoint. Recovery is successful only after the saved
connection, autoconnect state and observed IP/route/DNS digest match again.

Recovery snapshots and enable completion wait for NetworkManager to finish
both IP families, so a transient connected-but-addressless state cannot become
the saved recovery target.

Disconnect uses NetworkManager's runtime inhibition, leaving saved profiles
unchanged. A confirmed disable retains the previous saved connection UUID, so
enable can select it. If no previous connection is known, the API requires an
explicit compatible `connection_id`; the GUI connection chooser remains to be
implemented. Physical Ethernet, Wi-Fi, DHCP changes and reboot behavior require
further acceptance beyond the static dual-stack container fixture.

Device-network forwarding is not a NetworkManager resource. The helper keeps
its own inventory (`forwarding.json`) of the nftables table it owns and the
forwarding switch it turned on, written before any rule is; a device-network
change is recovered from a snapshot of that inventory rather than from a
NetworkManager checkpoint, and the helper checks every ten seconds that its
table is still there. Independently of both processes, the root-owned
`nilo-nethelper-recovery.timer` runs `nilo-nethelper recover` twenty seconds
after boot and every thirty seconds: it takes the helper's lock without
waiting, does nothing while the helper is alive, and otherwise puts an
interrupted change back and reinstalls the owned table from the inventory.
It consumes only the saved typed records; there is no remote way to feed it.

If the helper cannot verify restoration it blocks new changes. A pending
operation is adopted by a restarted Bench Core from its own durable record and
settled against the helper's record of what became of it. The helper recovery
file is a safety record, not a complete network audit trail.

NetworkManager API references: [device disconnect and autoconnect](https://networkmanager.pages.freedesktop.org/NetworkManager/NetworkManager/gdbus-org.freedesktop.NetworkManager.Device.html)
and [checkpoint operations](https://networkmanager.pages.freedesktop.org/NetworkManager/NetworkManager/gdbus-org.freedesktop.NetworkManager.html).

APT references: [configuration order and pre-install hook protocol](https://manpages.debian.org/trixie/apt/apt.conf.5.en.html),
[package transaction options](https://manpages.debian.org/trixie/apt/apt-get.8.en.html)
and [source trust overrides](https://manpages.debian.org/trixie/apt/sources.list.5.en.html).
