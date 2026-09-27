# Local Hardware Access

## 1. Purpose

This document records the physical devices reachable from the local development
bench, how to find them when their addresses change, and what may and may not be
concluded from running something on them.

It describes a convenience, not a gate. Nothing in the repository's validation
depends on any device listed here, no automated test contacts one, and a
developer without access to this bench must remain able to run every repository
gate. A device listed here may be powered off or off the network at any time,
and nothing may be written that assumes otherwise.

The simulated development topology remains the default and is described in
[Development and Simulation Flow](Development_and_Simulation_Flow.md). Reach for
real hardware only when the question is specifically about native Linux host
behavior, target packaging, architecture, or physical I/O, which simulation is
explicitly not able to answer.

The bench currently has two possible hardware targets: the Raspberry Pi in
section 2 and the Ubuntu mini PC in section 3. Either or both may be on the same
network. Never infer the intended target merely from finding an SSH listener;
select it by the stable identity recorded for that device. At present the mini
PC is the available target and temporarily substitutes for the Raspberry Pi in
tests that do not require ARM64 or Raspberry Pi-specific hardware.

## 2. `ddp-remote` — Raspberry Pi ARM64 test device

### 2.1 Identity

The device takes its addresses from the bench DHCP server and holds no
reservation, so its address changes. The identity below is the part that does
not move.

| Property | Value |
|---|---|
| NILO permanent identity | `nilo-n0hzmysj9v` |
| NILO permanent local URL | `https://nilo-n0hzmysj9v.local:8443/` |
| Bench SSH hostname | `ddp-remote.local` (inherited OS transport name, not a NILO identity) |
| Wired hardware address | `dc:a6:32:77:3a:c4` |
| Wireless hardware address | `dc:a6:32:77:3a:c5` |
| Operating system | Debian 13 (trixie), 64-bit |
| Board model | Raspberry Pi 4 Model B Rev 1.2 (observed 2026-09-13) |
| SSH service | OpenSSH 10.0p2, `Debian-7+deb13u4` |
| Login account | `dpasare` |
| Authentication | Obtain through the approved out-of-band channel |

The bench SSH hostname may resolve on a directly attached local network. Use the
permanent NILO name for the application and the recorded hardware addresses to
distinguish the board when SSH name resolution is unavailable.

`dc:a6:32` is the Raspberry Pi Trading Ltd hardware address prefix, which places
the board in the Raspberry Pi 4 generation. The two addresses differ only in the
final digit, which is how that generation assigns consecutive addresses to the
wired and wireless interfaces; the mapping of the lower address to the wired
interface follows that convention and has not been confirmed on the device
itself.

Reconfirm the exact board model after a hardware replacement rather than
assuming the hostname still names the same board:

```bash
cat /proc/device-tree/model
```

Do not confuse this device with the Raspberry Pi at hardware address
`b8:27:eb:98:44:c7` on the same bench. That prefix belongs to the earlier
Raspberry Pi Foundation range and is a different, older board.

### 2.2 Finding it

Address it by name and the address change costs nothing:

```bash
ssh dpasare@ddp-remote.local
```

If multicast DNS does not resolve — some Windows configurations and most managed
networks do not carry it — find the device by its hardware address instead. A
neighbour table only holds addresses recently spoken to, so sweep the bench
subnet first, then match the prefix:

```powershell
1..254 | ForEach-Object { ping.exe -n 1 -w 200 "192.168.1.$_" | Out-Null }; arp -a | Select-String "dc-a6-32"
```

The sweep takes about a minute and reports nothing itself; the `arp` line is
what answers. Whatever it prints is current, and supersedes section 2.4
entirely.

Two addresses mean both interfaces are up, and either will serve. One means only
one is connected. None means the device is powered off, is on another subnet, or
has had its interfaces replaced — check the third possibility before assuming
the first, because a replaced board keeps the hostname and loses these addresses.

### 2.3 What it is for

This device is the required bench target for the following Raspberry Pi- and
ARM64-specific observations, which the mini PC and WSL2 simulation cannot
currently replace:

- ARM64-native builds, and dependencies that have no ARM64 wheel or binary;
- Debian package installation, systemd units, permissions, and service recovery
  on the actual target distribution;
- real USB, serial, and GPIO enumeration, and udev rules;
- Raspberry Pi timing, thermal, storage, and power behavior under load.

It is a personal bench device, not the certified hardware acceptance
environment. A result observed here is a useful early signal and is **not**
release evidence: its configuration is not recorded, version-controlled, or
reproducible from this repository, so no one can rebuild the machine the
observation was made on — including whoever made it. The hardware-in-the-loop
level in section 9 of the development flow still requires a controlled device
and real equipment. Recording an observation from this device in a task journal
is fine; presenting one as a passed gate is not.

### 2.4 Optional NILO recovery wiring

The Raspberry Pi delivery supports an optional local recovery button and LED.
Shut down the Raspberry Pi, disconnect its power supply, and verify that it is
unpowered before installing or changing any GPIO wiring. Never move the button,
LED, or resistor connections while the board is powered.
Wire a normally-open momentary button between **BCM GPIO17** and ground; NILO
uses the SoC internal pull-up, so no external pull-up is required. Wire an LED
from **BCM GPIO27** through a suitable current-limiting resistor to ground. The
LED output is active high.

Enable this hardware path only when installing on a Raspberry Pi:

```bash
sudo ./02_Tools/scripts/install-nethelper.sh <nilo-nethelper-binary> <service-user> --raspberry-pi-recovery
```

The installer refuses this option when the kernel model is not a Raspberry Pi
or `/dev/gpiochip0` is unavailable. Generic mini PCs, desktops, laptops and
virtual machines omit the option and expose no GPIO recovery protocol.

Release the button between 3 and 10 seconds to arm or extend temporary network
recovery for ten minutes. Release it at or after 10 seconds to reset only the
network commissioning state and continue live in first boot. Short presses do
nothing. Network reset does not reset passwords, appliance identity, hostname,
TLS material, users, audit history, integrations, or PLC/device-network
assignments. The helper timestamps each recovery-duration release and records which
identified Ethernet links were already connected at that instant; only a later
link-up can be considered. A connected interface without stable identity makes
the snapshot unsafe and recovery is refused. After a service restart, any
non-idle temporary recovery is restored before a new event is accepted.
A GPIO read interruption invalidates the current gesture, so recovery requires
a fresh press after sampling resumes.

The current personal bench record does not confirm that this optional button
and LED are physically wired. A GPIO build or service-start observation is not
evidence that the press thresholds or LED circuit passed hardware acceptance.
On 2026-09-13 an isolated ARM64 candidate helper successfully claimed GPIO17 as
an active-low input and GPIO27 as a low output through the Linux GPIO v2 ABI,
then released both on shutdown. This proves the kernel/ABI capability on this
board, not the unwired physical actions.

### 2.5 RustDesk remote desktop

Installed on 2026-09-02 to reach the desktop, which SSH does not give. It is a
convenience on the same terms as the rest of this section: it is not recorded
anywhere but here, not reproducible from this repository, and no repository gate
depends on it.

| Property | Value |
|---|---|
| RustDesk identifier | Obtain through the approved out-of-band channel |
| Authentication | Not recorded in repository documentation |
| Version | 1.4.9, `aarch64` `.deb` |
| Service | `rustdesk.service`, enabled at boot |

The ID belongs to the installation, not to the network, so it survives the
address changes described in section 2.2 and is the one handle on this device
that does not move.

The board is headless and both HDMI connectors read `disconnected`. labwc
answers that by creating a virtual `1920x1080` output, so there is a framebuffer
to capture: no HDMI dummy plug is needed, and no mode has to be forced in
`/boot/firmware/cmdline.txt`. Confirm it rather than assuming it, because a
compositor change would remove it:

```bash
XDG_RUNTIME_DIR=/run/user/1000 WAYLAND_DISPLAY=wayland-0 wlr-randr
```

Nobody is at the console to accept an incoming session, so the service
authenticates on the permanent password alone and never prompts locally:

```toml
approve-mode = 'password'
verification-method = 'use-permanent-password'
```

Capture goes through `xdg-desktop-portal-wlr` under Wayland, and out of the box
that portal asks which screen to share by drawing a chooser on the device's own
display. Headless, nobody can ever answer it, and the connecting client sits on
a black screen reading `Please select the screen to be shared`. Naming the
output and removing the chooser is what makes the connection complete, in
`~/.config/xdg-desktop-portal-wlr/config` for `dpasare`:

```ini
[screencast]
output_name=NOOP-1
chooser_type=none
max_fps=30
```

`NOOP-1` is generated by labwc at runtime, not fixed. If a later boot names the
headless output differently this config stops matching and the chooser returns,
so read the name back with `wlr-randr` before assuming the config is wrong.

Should Wayland capture fail some other way, the X11 session is still installed
and needs no portal at all:

```bash
sudo raspi-config nonint do_wayland W1 && sudo reboot
```

Verified on 2026-09-02 by connecting a client: the service is active and enabled
with no restarts, and the desktop renders over the virtual output.

### 2.6 WireGuard tunnel `wg0`

The field profile uses the NILO-managed `nilo0` tunnel. Private and preshared
keys remain only in the privileged key store and the authorized peer store.

| Property | Value |
|---|---|
| Interface | `nilo0` |
| Peer endpoint | `ddp-engineering.go.ro:51820` |
| Tunnel address | `10.205.148.11/32` |
| Startup | Selected through NILO's persisted network profile |

Do not manage this NILO-owned connection with `wg-quick`. Activate and inspect
it through the NILO network workflow so the helper's checkpoint, key custody,
audit and recovery rules remain in force. A live handshake and return after
reboot remain Raspberry Pi acceptance evidence, not an assumption from this
document.

### 2.7 Unattended administration

The board carries `/etc/sudoers.d/90-dpasare-automation`, granting `dpasare`
passwordless `sudo`. It exists so an agent driving the board over SSH can
install binaries, restart services and read privileged network state without a
password being handled in plain text.

```bash
sudo rm /etc/sudoers.d/90-dpasare-automation
```

Remove it when the board stops being a personal test device. It is recorded here
and nowhere else, like the rest of this section.

`nilo0` is set to `autoconnect yes` on its NetworkManager profile. It was
`no`, which meant the tunnel existed only while NILO's startup selection held it
up — and the tunnel is the only route to this board from outside its local
networks. Anything that restarted NILO on a data directory without that
selection would have stranded the board until somebody reached it physically.

### 2.8 Current NILO deployment

The board runs NILO 0.11.0 with runtime state in `/var/lib/nilo`. `nilo.service`,
`nilo-nethelper.service` and the independent recovery timer are active and
enabled. The core service uses the helper-managed TLS identity and has no
file-backed certificate or private-key argument. Its served certificate covers
`nilo-n0hzmysj9v.local` and the provisioning-only `nilo.local`; the helper's
private key remains root-owned and unreadable by the Core service account.

The 2026-09-27 live-test deployment installed NILO 0.11.0 Core, GUI and the
USB to Serial plugin. Both connected serial adapters are configured and ready
with Ask session approval. Its rollback backup is
`/var/backups/nilo-0.11.0-20260927-092635`. Version, service health, served asset
checksums, serial discovery/configuration and retained capture checksums passed.
See the [deployment record](../07_Delivery/NILO_0.11.0_RPi_Live_Test.md).

The persisted management allowlist is `wlan0` and `nilo0`. The DDP Wi-Fi and
NILO-owned WireGuard profiles remain installed, while `eth0` remains the
device-side adapter at `192.168.0.1/24`. The enabled ILC151 forwarding plan maps
the exposed `10.71.1.0/24` range to the physical `192.168.0.0/24` segment and
permits its configured local and WireGuard sources. The current nftables table
contains that plan.

The 2026-09-17 helper-TLS service update preserved every NetworkManager profile
byte-for-byte and preserved the serialized ILC151 forwarding plan byte-for-byte.
The WireGuard interface remained active with a current handshake, and the GUI
returned HTTP 200 through that path while the protected status route returned
HTTP 401. The PLC at `192.168.0.130` did not answer ICMP or TCP 504 during this
check; that is an availability observation, not evidence that its preserved
NILO configuration changed.

The 2026-09-25 development deployment replaced the ARM64 core, helper, and GUI
after archiving the prior runtime and state. The root-only rollback
directory is `/var/backups/nilo/pre-0.9.0-20260925T202739`; its
`runtime-and-state.tar.gz` has SHA-256
`415f6126f0a90278e3c595fc9912c6f8177199986de230a5f7f2eecbed3495df`.
The installed core and helper hashes match the staged build. Both services and
the recovery timer are active and enabled with zero service restarts. The
NetworkManager profile digest is unchanged, `wlan0` and `nilo0` retain their
addresses, and the NILO nftables tables are present. A Windows client received
HTTP 200 for the GUI and HTTP 401 for the protected status API over both local
Wi-Fi and WireGuard. Headless Chromium rendered the live sign-in page showing
version 0.9.0; its served JavaScript matches the locally built asset byte for
byte. This bench smoke test did not sign in, exercise the PLC, or test GPIO.

The preceding 0.8.0 helper-TLS rollback remains at
`/var/backups/nilo/pre-helper-tls-0.8.0-20260917T114203Z`.

On 2026-09-25, a GUI-only development candidate was deployed from the current
worktree without changing the 0.9.0 project version or the Core and helper
binaries. The previous GUI is preserved at
`/var/backups/nilo/pre-gui-20260925T232139` (including a root-only archive).
The served JavaScript and CSS SHA-256 digests match the local build:
`69ca5eb0682c603bac86174da62f7633d6b3b7f3ac82e014e46dbfb229617582`
and `9b58c7b6b22a3e5a338ebfcb45a0d346f43fabe06d488856a0842ef09abb9e51`.
After the Core restart, the Core, helper and recovery timer were active; the
GUI returned HTTP 200, and the protected status API returned HTTP 401 without
a session.
An authenticated live browser showed Camera and Sigrok Instruments beneath
System > Plugins. A scrollable Camera view reached the 1920-pixel viewport edge.

On 2026-09-26, Camera administration and the current ARM64 Core were deployed
to `ddp-remote` at `10.205.148.11`. The prior Core rejected the Camera manifest's
`device_configuration_schema` and artifact `media_types` fields; deploying the
current Core restored enablement without modifying the installed bundle.
Core SHA-256 is
`686e8472635091ec01f1e383f18e87cfb8fa5f8329a15dc8f6a404b6b7b710b8`.
The root-only runtime/state backup is
`/var/backups/nilo/pre-camera-admin-20260926T035020/runtime-and-state.tar.gz`,
verified SHA-256
`6bf63b131b5d2b3f9aa932ffbf8b137dea35dfa9ae7d993bc17cf1e3fa1ad4fd`.
At that deployment, the project version was 0.9.0 and the Camera bundle was 0.1.0.
Final GUI assets are `index-D-cmzIfi.js` (SHA-256
`d5b647720659532848e19dbdba8799afbfc7f2e352030b6eda813c413f28aa45`)
and `index-BIKcA3US.css` (SHA-256
`fc7e4106740a3bd7e39cc0476cbcfebe6f39afe8a714b3bc7386d29b033bec5c`).
The served JavaScript matches the deployment hash. The final GUI also labels
a disabled camera as disabled rather than exposing its internal failed readiness.

The accepted build container passed the full Core Go suite, 533 frontend tests,
lint, formatting and production builds. Its offline cache needed the existing
locked go-qrcode and goqr downloads copied from the verified local Go cache.
Authenticated Chrome checks confirmed plugin and device enable/disable,
discovery (`cam-1`, present/ready), name persistence and sidebar updates, and
the Camera-filtered View activity link. The original camera name was restored;
Camera and its device were left enabled with approval policy `ask` for live testing.
Update plugin is disabled because the catalog has no newer Camera version;
newer/older/prerelease/integrity cases are covered by automated tests.
HTTPS checks ran from the Windows management client; self-connections
from the Pi timed out. Both NILO
services remained active; existing device settings and retained artifacts were preserved.

Later on 2026-09-26, the compact Camera GUI was deployed without changing Core
or plugin versions. Controls and connected cameras now share one frame, with
an inline approval selector and a wrapping toolbar. The previous GUI is backed
up at `/var/backups/nilo/pre-camera-compact-20260926T070635/gui.tar.gz`
(SHA-256 `ae1a4708c8c847347d7c63de123347f79dec35f03a448c06cff82fd5ad3c0175`).
The served `index-COdv7rnN.js` matches SHA-256
`a8126d803e71021f98adeb65def76dd4912b0cf1ca4aa50c0a1b9d4a7b317081`;
`index-qqfVNH4g.css` matches
`211f51560d9d251e5cb88ab58cf29d68b34a4fde49e891105a5db1b2c0aa7b4c`.
The accepted container passed 534 frontend tests, lint, formatting and build.
Live Chrome layout checks at 1920 pixels, 800x480 and 320x568 confirmed wrapping
and no horizontal overflow. Connected cameras needing setup or re-enabling stay
visible; disconnected and unknown-presence records are hidden. Device state and
the user-assigned camera name were preserved.

The Camera administration adjustments and Pi deployment task were closed on
2026-09-26 at the user's request, with the verification recorded above.

The navigation splitter GUI was deployed on 2026-09-26 with the project still
at 0.9.0. The production build used the accepted container; its implementation
had passed 539 frontend tests, lint and formatting. The previous GUI is retained
at `/var/backups/nilo/pre-navigation-splitter-20260926T083902/gui.tar.gz`, with
verified SHA-256
`12b1fb6674a8dfc99a5481412a8a85075de34b32813e1ab4c3a90747c3ac6c02`.
The served `index-BG2EI9Kr.js` matches SHA-256
`c4ba044eb464fbddbaf0d06bca04ac3f18b5569aabbf09eed82efdb3c2295d85`;
installed `index-CRztB_4g.css` matches
`c9daaf234d09f7e01a287ac0f8f77f2ca905e1d5fbc2c5fa6b27c0df1110c622`.
Both services are active and HTTPS returns 200. Authenticated Chrome confirmed
dragging from 216 to 336 pixels and preserving that width after reload. Camera
settings then showed full navigation labels. Core, helper and plugin binaries
were unchanged; no active queued, running or pending-approval work was present
at the deployment check.

The optional GPIO recovery path remains disabled until the BCM17 button and
BCM27 LED circuit is physically confirmed.

### 2.9 Power-control checks

Use the board only after deploying the candidate ARM64 binaries, installing the
candidate privileged helper, and starting Bench Core in the appliance profile.
Keep the local console or physical power control available. The repository test
suite must remain independent of this sequence.

For a restart check, record the current boot identity before selecting **Restart
NILO** as an Operator or Administrator:

```bash
cat /proc/sys/kernel/random/boot_id
systemctl is-active nilo-nethelper.service
```

Confirm the action once in the browser. Observe the host become unreachable,
wait for `ddp-remote.local` to return, then reconnect and read the boot identity
again. A different non-empty value proves a new boot; also confirm the helper is
active and the appliance interface can be reached and signed into. Record the
old and new boot identities, down/up observation and service health in the task
journal. Do not treat a dropped HTTP response alone as proof of restart.

The power-off/power-on check belongs to the person at the bench. Before the
action, record the boot identity and ensure there is a known way to restore
power. Select **Power off NILO**, confirm once, and observe that the board fully
halts. Restore power physically, wait for the host to return, and verify a new
boot identity, `nilo-nethelper.service`, and the appliance interface. The tail of
the previous boot journal can provide supporting evidence of an orderly halt:

```bash
journalctl -b -1 --no-pager | tail -n 50
```

This personal board is still not a reproducible acceptance environment. Record
the observation as early bench evidence, including any missing service or
manual recovery, and never turn it into a required automated gate.

## 3. `ddp-remote-linux` - Ubuntu mini PC test device

### 3.1 Identity

This mini PC is a temporary substitute for the Raspberry Pi during development
testing. It is a separate device, not a replacement identity for `ddp-remote`,
and results from it must say which target produced them.

| Property | Value |
|---|---|
| Multicast DNS hostname | `ddp-remote-linux.local` |
| System hostname | `ddp-remote-linux` |
| Current IPv4 address | `192.168.1.47` (DHCP; observed 2026-09-10) |
| Wireless interface | `wlp6s0`, `d4:d2:52:9e:72:49` (connected) |
| Wired interface | `eno1`, `f8:b4:6a:c2:e6:ba` (disconnected) |
| Hardware | HP EliteDesk 705 G4 DM 65W |
| Processor | AMD Ryzen 5 PRO 2400G with Radeon Vega Graphics, `x86_64` |
| Operating system | Ubuntu 26.04.1 LTS (Resolute Raccoon) |
| Kernel | Linux 7.0.0-31-generic |
| SSH service | OpenSSH server `1:10.2p1-2ubuntu3.6`; active and enabled |
| Login account | `dpasare` |
| Authentication | Obtain through the approved out-of-band channel |

The address is an observation, not a reservation. Prefer the multicast DNS name
and use the hardware address to distinguish this device if DHCP or name
resolution is ambiguous. Do not use another Ubuntu host merely because it
answers on port 22.

### 3.2 Finding and connecting to it

Resolve the name and verify the result before connecting:

```powershell
Resolve-DnsName ddp-remote-linux.local -Type A
arp -a 192.168.1.47
ssh dpasare@ddp-remote-linux.local
```

On 2026-09-10 the name resolved to `192.168.1.47`, ICMP replied, and the neighbor
table reported the connected wireless address `d4:d2:52:9e:72:49`. TCP port 22
initially returned `Connection refused`, and a local
`sudo systemctl enable --now ssh` reported that `ssh.service` did not exist.
Running the package installation command confirmed that `openssh-server` was
already at the current installed version; enabling and starting the service then
succeeded. A subsequent SSH login with the account above verified the identity,
OS, architecture, interfaces, authentication, and active/enabled service state
in this section.

If the service is absent or port 22 refuses connections again, reconcile the
package and start the server locally on the mini PC:

```bash
sudo apt update
sudo apt install -y openssh-server
sudo systemctl enable --now ssh
systemctl is-active ssh
sudo ss -lntp | grep ':22'
```

Only investigate the firewall after confirming a listener. A closed listener
normally produces the immediate refusal observed above; a firewall more often
produces a timeout.

### 3.3 What it may substitute for

Use the mini PC for native Ubuntu installation and service behavior, Linux
permissions, systemd recovery, network integration, and end-to-end application
checks that do not depend on the Raspberry Pi architecture or peripherals.

Until its architecture and connected peripherals are verified, it does not
substitute for ARM64-native builds, Raspberry Pi packaging, GPIO, Pi-specific
USB/serial behavior, or Raspberry Pi timing, thermal, storage, and power
observations. Those remain attached to `ddp-remote`. Like the Raspberry Pi, this
mini PC is a personal development target and not a reproducible
hardware-acceptance environment; observations from it are useful bench evidence,
not release-gate evidence.

When both devices are present, commands and test records must name the selected
host explicitly. When only one is present, absence of the other is normal and
must not fail repository validation.

### 3.4 RustDesk remote desktop over X11

Configured on 2026-09-10 for unattended access without Wayland's per-connection
desktop-region chooser.

| Property | Value |
|---|---|
| RustDesk ID | Obtain through the approved out-of-band channel |
| RustDesk version | 1.4.9 |
| Display manager | LightDM 1.32.0, X11 greeter on `DISPLAY=:0` |
| Desktop session | Xfce 4.20.1 over Xorg |
| Xorg packages | `xorg` / `xserver-xorg` 7.7+26ubuntu1, AMDGPU driver 25.0.0 |
| RustDesk service | Active and enabled |
| Approval mode | `password` |
| Verification method | `use-permanent-password` |

Ubuntu 26.04's installed GNOME 50 session was Wayland-only. Installing Xorg by
itself did not provide a GNOME X11 session, and setting `WaylandEnable=false` in
GDM did not move the GDM greeter off Wayland. The working arrangement therefore
uses LightDM as the display manager and Xfce as the X11 desktop. GDM remains
installed but is inactive.

The active configuration is:

```ini
# /etc/lightdm/lightdm.conf.d/50-ddp-x11.conf
[Seat:*]
user-session=xfce
```

The `dpasare` AccountsService record also selects `Session=xfce`. RustDesk's
effective user options in `~/.config/rustdesk/RustDesk2.toml` are:

```toml
verification-method = 'use-permanent-password'
approve-mode = 'password'
```

After the display-manager change, `loginctl` reported the LightDM greeter as
`Type=x11`, Xorg was running on `:0`, and the RustDesk server process was running
as the LightDM user with `DISPLAY=:0` and LightDM's X authority file. This removes
the Wayland portal region-selection step. The permanent password still controls
access; `approve-mode=password` removes the additional local click approval.

The previous GDM and user-session files are retained on the mini PC as:

```text
/etc/gdm3/custom.conf.before-x11-20260910
/var/lib/AccountsService/users/dpasare.before-x11-20260910
```

To return to the previous GNOME Wayland arrangement, restore both files, select
GDM again when `dpkg-reconfigure` prompts, and restart the display manager:

```bash
sudo cp /etc/gdm3/custom.conf.before-x11-20260910 /etc/gdm3/custom.conf
sudo cp /var/lib/AccountsService/users/dpasare.before-x11-20260910 \
  /var/lib/AccountsService/users/dpasare
sudo dpkg-reconfigure gdm3
sudo systemctl restart display-manager
```

### 3.5 WireGuard client

Verified on 2026-09-10 with the `ddp_nilo` NetworkManager profile active. The
developer may refer to it as `ddp-nilo`; commands must use the stored underscore
name shown by `nmcli`.

| Property | Value |
|---|---|
| Package | `wireguard-tools` 1.0.20250521-1ubuntu1 |
| CLI | `wireguard-tools v1.0.20250521` |
| Kernel support | In-tree `wireguard.ko.zst` module for kernel 7.0.0-31-generic |
| Connection manager | NetworkManager |
| Connection/interface name | `ddp_nilo` |
| Tunnel addresses | `10.205.148.11/32`, `fd11:5ee:bad:c0de::acd:9411/128` |
| Tunnel DNS | `10.205.148.1` |
| Routing | IPv4 and IPv6 default traffic through `ddp_nilo` |
| At boot | Direct NetworkManager autoconnect disabled; guarded systemd activation enabled |
| State | Activated; recent authenticated handshake and traffic observed after a reboot |

The package was already at the newest repository version when explicitly
installed and was marked as manually installed. The peer profile was imported
into NetworkManager separately; its private and preshared keys must not be
committed to this repository or printed into diagnostic records.

Direct WireGuard autoconnect caused a boot-time DNS dependency loop. The virtual
profile started before the physical uplink had usable DNS, installed full-tunnel
routes and selected the tunnel-only DNS server, then could not resolve its own
`ddp-engineering.go.ro` endpoint. NetworkManager still called the connection
activated even though WireGuard had no endpoint, no handshake and zero traffic;
the resulting DNS outage also kept RustDesk from reaching its rendezvous server.

The device now uses an uplink-independent guarded activator:

| File | Purpose |
|---|---|
| `/usr/local/sbin/ddp-nilo-online` | Resolves and routes the endpoint before activating the tunnel, then requires a handshake |
| `/etc/systemd/system/ddp-nilo-online.service` | Starts after `network-online.target`, retries failures every 15 seconds, and remains enabled at boot |
| `/etc/NetworkManager/dispatcher.d/90-ddp-nilo-online` | Re-evaluates the tunnel after any Wi-Fi or Ethernet uplink event |
| `/etc/NetworkManager/dispatcher.d/no-wait.d/90-ddp-nilo-online` | Runs the event hook asynchronously so it cannot block NetworkManager |

The helper accepts an already-active tunnel only when its most recent handshake
is at most 180 seconds old. Otherwise it removes the stale tunnel, resolves the
endpoint through the base network, verifies that the endpoint route does not use
`ddp_nilo`, activates the profile, generates traffic, and waits up to 20 seconds
for a real handshake. Any resolution, activation, or handshake failure takes the
tunnel back down so ordinary DNS, RustDesk and local management remain usable for
the next retry.

This is event-driven rather than a fixed boot delay and works with any usable
non-WireGuard uplink. A controlled reboot on 2026-09-10 verified successful
automatic endpoint resolution, handshake, traffic, DNS, RustDesk rendezvous
reachability, and a direct return route to the development host.

Manage the imported profile through the guard for automatic operation, or
through NetworkManager for deliberate manual diagnostics:

```bash
sudo systemctl restart ddp-nilo-online.service
systemctl status ddp-nilo-online.service
journalctl -t ddp-nilo-online -b

nmcli connection up ddp_nilo
nmcli connection down ddp_nilo
sudo wg show ddp_nilo
```

The active tunnel installed separate IPv4 and IPv6 policy-routing tables while
the route back to the development host at `192.168.1.13` remained directly on
`wlp6s0`; enabling it did not strand SSH. The peer-side tunnel address
`10.205.148.1` did not answer ICMP, but that is not treated as a failure because
the WireGuard handshake was recent, byte counters were non-zero, and default
routes selected the tunnel. Diagnose with those signals rather than requiring
the peer to answer ping.

### 3.6 Physical connections and local hardware

The mini PC is headless and has no monitor attached. Its active X11 display is
the LightDM/Xorg display described in section 3.4; do not assume that a physical
monitor or EDID is available when diagnosing resolution or capture behavior.

| Connection | Attached target | Current observation |
|---|---|---|
| Wi-Fi `wlp6s0` | Development LAN and current internet uplink | `192.168.1.47/24` |
| Ethernet `eno1` | Phoenix Contact ILC 151 PLC | PLC is `192.168.0.130`; mini PC currently has only `169.254.5.174/16` |
| USB | ATK logic analyzer | Attached; USB identity and device node not yet read back |

The PLC link is physically up, but it is not usable yet: the mini PC has no
address in `192.168.0.0/24`. While that remains true, the full-tunnel policy sends
traffic for `192.168.0.130` into `ddp_nilo`. Assign a confirmed, unused static
address in the PLC subnet to `eno1` before attempting PLC communication; do not
invent one because an address collision on a control network is unsafe. Once the
connected route exists, it must continue to take precedence over WireGuard.

Treat the logic analyzer device node as dynamic. Identify it by USB vendor,
product and serial attributes before binding tools or permissions to it, rather
than assuming a fixed `/dev/tty*` or `/dev/bus/usb/*` path.

### 3.7 Live NILO development deployment

NILO `0.2.0` from source commit
`11f4644a4df251749cff251871c14f8876b63d7e` was deployed to the mini PC on
2026-09-10 for live development checks. It runs in the real `appliance`
profile, not simulation, and serves the built GUI over HTTPS:

| Property | Value |
|---|---|
| Local URL | `https://ddp-remote-linux.local:8443` |
| WireGuard URL | `https://10.205.148.11:8443` |
| Management allowlist | `wlp6s0`, `ddp_nilo` |
| Excluded PLC interface | `eno1` |
| NILO username | `admin` |
| NILO password | `admin_password` |
| Core service | `nilo.service`, active and enabled |
| Privileged helper | `nilo-nethelper.service`, active and enabled |
| Recovery timer | `nilo-nethelper-recovery.timer`, active and enabled |
| Runtime data | `/var/lib/nilo` |
| GUI files | `/opt/nilo/gui` |
| Rollback snapshot | `/var/backups/nilo/pre-dual-path-11f4644` |

The `admin` / `admin_password` pair is the fixed **development-only NILO
credential** used by this repository. It is separate from the Ubuntu/SSH login
in section 3.1 and must not be used on a production or customer appliance. This
mini PC remains a disposable development target even though NILO uses the
`appliance` runtime profile so that live NetworkManager and privileged-helper
behavior can be exercised.

The service starts with `--management-port 8443` and loads the interface list
from `/var/lib/nilo`; a service restart confirmed that the saved configuration
survives without repeating interface arguments. The helper-owned
`inet nilo_management` table accepts TCP 8443 only through `wlp6s0` and
`ddp_nilo`, then drops that port on every other ingress interface. The local GUI
returned HTTP 200 after the restart, while unauthenticated status and management
API requests returned HTTP 401. The deployed GUI bundle contains the Management
access interfaces panel.

The current certificate is locally self-signed for the mini PC hostname, LAN
address and WireGuard address; its SANs include `ddp-remote-linux.local` and
`10.205.148.11`. A browser that does not trust it displays a certificate
warning; this is acceptable for this bench deployment only and is not a
production certificate model. The development host had no route to
`10.205.148.0/24`, so a separate WireGuard peer must still exercise the VPN URL.

### 3.8 Raspberry Pi dual-path development deployment

The dual-path candidate at `11f4644a4df251749cff251871c14f8876b63d7e` was
deployed with explicit authorization on 2026-09-11 to
the Raspberry Pi 4 development appliance `ddp-remote`. Existing NILO state,
administrator data, TLS material and NetworkManager profiles were preserved.
The root-only rollback archive is
`/var/backups/nilo/pre-dual-path-11f4644-pi-20260911-184541/files-and-state.tar`.

The Pi service loads its persistent management policy from `/var/lib/nilo` and
serves HTTPS on port 8443. The temporary development allowlist is `eth0`,
`wlan0` and `nilo0` because only the directly attached `eth0` recovery path was
usable during deployment. `eth0` must be removed from this list before it is
reconnected to the PLC network. A service restart and a full Pi reboot both
restored NILO, the helper, the recovery timer and the helper-owned firewall.
HTTPS returned HTTP 200 after reboot, and protected APIs remained HTTP 401
without authentication.

The certificate was preserved and contains `DNS:ddp-remote.local` and
`IP:10.205.148.11`. The `DDP` Wi-Fi profile remains set to autoconnect, but the
hotspot was unavailable during verification. The saved `nilo0` WireGuard
profile was offline and had NetworkManager autoconnect disabled; enable it
through the approved NILO network workflow before expecting VPN recovery after
a reboot. Do not connect the mini PC and Pi with the duplicated temporary
WireGuard identity at the same time.

After the `DDP` hotspot became available, the Pi connected as
`10.44.118.42/24` and `nilo0` was started as `10.205.148.11/32`. The WireGuard
hub reported a current handshake, and the Windows client at `10.205.148.7`
received HTTP 200 from `https://10.205.148.11:8443`. The GUI was then updated
from combined branch commit `04ccc12209c4062855fc28702221f5a0d0f190d6`, which
includes the newer login interface from remote `master`. The deployed
JavaScript hash matched the tested production build. Its immediate rollback
archive is `/var/backups/nilo/pre-master-04ccc12-20260911-1954/runtime.tar`.

## 4. Credential handling

Repository documentation records no bench password, remote-desktop identifier,
Wi-Fi password, WireGuard private/preshared key, token or other authentication
secret. Obtain temporary access through the approved out-of-band channel and do
not paste it into issues, logs, command arguments, API responses or test data.

RustDesk may register with a public rendezvous or relay even when no inbound
port is forwarded. Disable it when remote desktop is not required:

```bash
sudo systemctl disable --now rustdesk
```

WireGuard key material belongs only in the helper-owned root-only key store and
the authorized peer store outside the repository. If any real credential was
ever committed in earlier history, deletion from the current document does not
revoke it; rotate it at its authority and invalidate the exposed value.

Devices added to this document later are not covered by the reasoning above.
Each one needs its own decision, and the default for anything that is not
disposable is that its credentials stay out of the repository.
