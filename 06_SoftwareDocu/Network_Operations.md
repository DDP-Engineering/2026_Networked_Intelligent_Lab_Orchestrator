# Network operations and development

Use this guide for the current network workflow, Linux test entry point and
appliance setup. Historical phase reports belong in Git history, not this guide.
The default development topology is described in
[Development and Simulation Flow](Development_and_Simulation_Flow.md).

## Runtime and interfaces

The simulation profile uses simulated network state. The Linux appliance profile
uses an unprivileged Bench Core and a root NetworkManager helper. The helper
owns managed connections, WireGuard key storage, device-network forwarding and
recovery. It also owns the dedicated `inet nilo_management` nftables table that
enforces the HTTPS management-interface allowlist before a request reaches
Bench Core. Bench Core sends only validated interface names and a port through
the typed helper protocol; it has no network-administration capability and
cannot submit arbitrary firewall rules. NILO-managed resources are distinct from external connections and
tunnels; do not manage an external tunnel as if NILO created it.

In the GUI, Network contains Overview, Network adapters, Device access,
VPN profiles and Diagnostics. Activity provides authorized audit queries,
exports and retention controls. Use the real simulation backend for integrated
workflows; the browser mock does not reproduce every network lifecycle.

WireGuard profiles can be saved, renamed, activated and removed, with one
managed profile active at a time. Readiness checks report tooling/kernel support;
they do not prove handshake state or remote reachability. Device access records
specify the device-facing adapter/subnet, exposed addresses, allowed sources and
services. Configured intent, locally applied forwarding and remote connectivity
are separate facts. Access instructions identify routes needed on other machines.

## Changes and recovery

Network mutations require review and acknowledgement. Connectivity-changing
operations then require confirmation before their deadline, normally 120
seconds. Acknowledging a proposal is not confirmation that connectivity survived.
Retain an independent console when testing management connectivity changes.

Accepted operations have durable IDs. A browser reconnects to the recorded
operation; a lost response is not proof that a change failed. Inspect the existing
operation before retrying. NetworkManager checkpoints protect connection changes;
device forwarding has a separate durable inventory of NILO-owned nftables state.
Unverified recovery blocks further changes rather than reporting success.

Management access is a separate, persistent change with its own last-known-good
record. The interface names are the policy; their DHCP addresses are only
observations. A single HTTPS server listens on the configured management port,
then both the socket gate and the helper firewall admit only connections whose
destination belongs to an enabled interface. The listener refreshes the
interface/address mapping while it runs, so address renewal, link loss and link
return do not require another process or a database restart.

An update must retain at least one currently usable path. Removing the path used
by the current browser requires a second explicit action. An accepted change is
`pending` for 90 seconds and must be confirmed through one of the newly enabled
interfaces. NILO restores the last-known-good allowlist after the deadline, on
an explicit restore, or on startup when it finds an interrupted pending change.
The GUI reports `pending`, `applied`, `failed` and `recovered` rather than
turning a lost response into success. Keep console or independent VPN access
available while changing the allowlist.

The helper stores recovery records under `/var/lib/nilo-nethelper`. Its service
and the independent `nilo-nethelper-recovery.timer` reconcile interrupted work.
The timer invokes `nilo-nethelper recover`; it does not accept arbitrary remote
commands. Keep runtime records and WireGuard private keys out of the GUI, source
tree, logs and exported diagnostics.

## CLI examples

Replace IDs, URLs, token paths and addresses with the intended appliance values.
Use `--ca-file PATH` when its certificate authority is not in the trust store.

```text
nilo network status --server URL --token-file TOKEN_FILE
nilo network operation list --server URL --token-file TOKEN_FILE
nilo network operation OPERATION_ID --server URL --token-file TOKEN_FILE
nilo network operation confirm OPERATION_ID --server URL --token-file TOKEN_FILE
nilo network interface settings ID --method static --address 192.168.0.1/24 --never-default --server URL --token-file TOKEN_FILE
nilo network wifi scan ID --server URL --token-file TOKEN_FILE
nilo network vpn profiles rename ID --name "Head office" --server URL --token-file TOKEN_FILE
nilo network device-networks create --file line-3.json --server URL --token-file TOKEN_FILE
nilo network device-networks enable ID --server URL --token-file TOKEN_FILE
nilo network device-networks check ID --tcp-port 502 --server URL --token-file TOKEN_FILE
nilo activity list --category audit --operation OPERATION_ID --server URL --token-file TOKEN_FILE
nilo activity export --format csv --out activity.csv --server URL --token-file TOKEN_FILE
```

To initialize interface-managed appliance access, use repeatable interface and
certificate-identity options. This creates one server and one state directory;
it must not be replaced by parallel `nilo serve` processes.

```sh
nilo serve --profile appliance \
  --data-dir /var/lib/nilo \
  --management-interface wlan0 \
  --management-interface nilo0 \
  --management-port 8443 \
  --management-tls-name ddp-remote.local \
  --management-tls-name 10.205.148.11 \
  --gui-dir /opt/nilo/gui \
  --tls-cert /etc/nilo/tls/server.crt \
  --tls-key /etc/nilo/tls/server.key \
  --nethelper-socket /run/nilo/nethelper.sock
```

After initialization, start the service with `--management-port 8443` and no
`--management-interface` options. NILO loads the saved allowlist and TLS names
from `/var/lib/nilo`; this lets an administrator's later GUI changes survive a
service restart and reboot. Repeating interface options that still match the
saved state is harmless. A different console list is refused unless
`--management-reconfigure` is present; that explicit change is staged as
pending with the previous list retained for automatic recovery. `--bind` and
interface-managed options are mutually exclusive. Existing installations
continue to use their literal `--bind` listener until an administrator
deliberately initializes interface-managed access.

Interface names must be valid non-loopback Linux interface names. An empty or
duplicate list or TLS identity is refused. A saved interface may be absent at
ordinary startup; initialization and explicit reconfiguration cannot select a
list with no currently usable path. Startup reports the configured interfaces
and their observed addresses.
Do not treat a Wi-Fi DHCP address as configuration.

Mutations prompt for acknowledgement or accept their explicit `--acknowledge`
option. Use the command's help for its complete options. GUI, CLI and exposed MCP
tools share authorization; do not assume every mutating CLI command has an MCP
counterpart. Activity access uses its own permissions.

## Isolated Linux test

With the repository's WSL distribution and Docker prepared, run from Windows:

```powershell
wsl -d Ubuntu-24.04 -- sh -lc 'sh /mnt/f/2026_Networked_Intelligent_Lab_Orchestrator/02_Tools/tests/network/check.sh'
```

Set `NILO_NETWORK_RACE=1` inside the Linux command for the race detector.
The harness creates a disposable NetworkManager environment with a private
system bus/network namespace and mounts the checkout read-only. Privileged
container capabilities apply to that fixture, not the host network. Allow time
for image compilation and real recovery timeouts. Ordinary unit tests do not
replace this opt-in system test.

For the Ubuntu installation-only fixture, set
`NILO_NETWORK_DISTRO=ubuntu24 NILO_INSTALL_ONLY=1` before invoking the harness.
The default fixture is Debian 13. Tests must not install the live helper onto
the Windows/WSL development host or reconfigure its network.

## Prepare a disposable appliance

Use a Linux machine with NetworkManager, an existing non-root NILO service
account, a separate management path and a trusted TLS certificate. Resolve the
bench Pi through [Local Hardware Access](Local_Hardware_Access.md); historical
IP addresses are not deployment configuration.

1. Build the GUI with `npm run build` through the repository tool wrapper in
   `05_Software/nilo_gui`; deploy its `dist` contents to `/opt/nilo/gui`.
2. Run `sh 02_Tools/scripts/build-appliance.sh` using the prepared Linux
   toolchain. Binaries are produced under
   `05_Software/nilo_assistant/dist/linux-<arch>/`; choose `arm64` for the Pi.
3. Install the helper explicitly on the target, substituting the staged binary:
   `sudo sh 02_Tools/scripts/install-nethelper.sh /path/to/nilo-nethelper nilo`.
   It installs the helper service and independent recovery timer. Device-network
   forwarding requires `nftables`; absent tooling is not working forwarding.
4. Give the service account its writable `/var/lib/nilo` data directory and
   restricted access to the TLS key. Keep the key and runtime state outside the
   GUI directory. Trust the issuing CA on clients.
5. Bootstrap the administrator once as the service account:
   `nilo admin bootstrap --data-dir /var/lib/nilo --username administrator`.
   The development-administrator convenience is simulation-only.
6. Provision, without having NILO generate or replace it, a certificate whose
   subject alternative names include every stable management identity. For the
   field profile this is at least `DNS:ddp-remote.local` and
   `IP:10.205.148.11`. NILO warns with the missing names when the certificate
   does not cover the configured identities. Trust the issuing CA on clients.
7. Initialize the appliance management listener and certificate paths:

```sh
nilo serve --profile appliance \
  --data-dir /var/lib/nilo \
  --management-interface wlan0 --management-interface nilo0 \
  --management-port 8443 \
  --management-tls-name ddp-remote.local \
  --management-tls-name 10.205.148.11 \
  --gui-dir /opt/nilo/gui \
  --tls-cert /etc/nilo/tls/server.crt --tls-key /etc/nilo/tls/server.key \
  --nethelper-socket /run/nilo/nethelper.sock
```

8. Configure the long-running service to use the same data directory with
   `--management-port 8443` rather than repeating the bootstrap interface list.
   The persisted selection then remains authoritative across restarts.

The administrator-only **Network > Network adapters > Management access
interfaces** panel lists detected names, type, link state and current addresses.
Enable `wlan0` and `nilo0`; leave `eth0` disabled. The panel identifies the
current browser path, requires explicit confirmation before removing it, and
offers keep/restore actions while a configuration is pending. It presents
`https://ddp-remote.local:8443` for local Wi-Fi and
`https://10.205.148.11:8443` for WireGuard. `.local` discovery works only on the
local network. The numeric Wi-Fi DHCP address is deliberately shown only as
current telemetry, not as a durable URL.

For a sigrok-enabled delivery, also follow the bundle instructions in
[Sigrok integration](Sigrok_Integration.md). Verify service startup, authenticated
access and recovery on the target; a successful build is not deployment evidence.

Missing WireGuard tools can be installed through the acknowledged NILO workflow
on supported Debian 13/Ubuntu 24.04 targets. Installation is a bounded helper
operation; it does not request a distribution upgrade, kernel replacement or
reboot. Inspect its operation result and readiness afterwards.

## Appliance power controls

On the live appliance profile, Operator and Administrator browser sessions can
choose **Restart NILO** or **Power off NILO** from the account menu. Each action
has a separate confirmation. Viewer sessions do not see the actions, and bearer
tokens are refused even when their stored scope set contains
`system.power.manage`. Sign out remains available to every browser session.

The browser sends exactly `POST /api/v1/system/power` with either
`{"action":"restart"}` or `{"action":"power_off"}` and the session CSRF
header. Bench Core persists `system.power.requested` before dispatch and returns
202 only when the privileged helper accepts the request. A connection loss is
reported as unconfirmed, not converted into success.

The peer-authenticated helper accepts only the empty `system.restart` and
`system.power_off` envelopes. As root, it calls systemd-logind on the system bus
with `Manager.Reboot(false)` or `Manager.PowerOff(false)`; no request data becomes
a command, argument, executable path or environment value. The simulation and
browser mock have only an unavailable controller.

Restart and power-off checks on the local bench Pi follow
[Local Hardware Access](Local_Hardware_Access.md#27-power-control-checks). The
restart can be observed remotely. The person at the bench owns the power-off and
power-restoration sequence because a powered-off board has no remote recovery
path. These observations are useful development evidence, not repository gates.

## Acceptance still requiring explicit evidence

Do not infer that these scenarios passed from the existence of code or the
removal of old reports. Reconcile existing test/journal evidence before scheduling:

- Core/helper/NetworkManager crashes, reboot during a pending apply, offline
  startup and migration with existing data on a booted appliance.
- Real-radio Wi-Fi join, USB adapter hotplug and power-cycle recovery.
- Dual-path Raspberry Pi acceptance: Wi-Fi DHCP renewal and reconnect,
  `nilo0` returning as `10.205.148.11`, access through both stable URLs,
  refusal on the PLC-facing `eth0`, unchanged PLC communication, and a full
  appliance reboot.
- PLC register-map validation and vendor download using an authorized tool.
- Real-backend Activity browser coverage, connection selection when an adapter
  has no remembered connection, and usability on the physical seven-inch panel.
- Event coverage for adapter appearance/disappearance/rebinding, handshake
  changes, clock synchronization and service lifecycle.

These are carried-forward verification/work items, not a claim that each is
unimplemented. Detailed historical observations remain in Git and task journals.
