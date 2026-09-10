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
| Multicast DNS hostname | `ddp-remote.local` |
| Wired hardware address | `dc:a6:32:77:3a:c4` |
| Wireless hardware address | `dc:a6:32:77:3a:c5` |
| Operating system | Debian 13 (trixie), 64-bit |
| SSH service | OpenSSH 10.0p2, `Debian-7+deb13u4` |
| Login account | `dpasare` |
| Authentication | Obtain through the approved out-of-band channel |

Both interfaces are usually connected at once, so the device answers on two
addresses simultaneously and both resolve to the same hostname. Prefer the wired
interface for anything long-running.

`dc:a6:32` is the Raspberry Pi Trading Ltd hardware address prefix, which places
the board in the Raspberry Pi 4 generation. The two addresses differ only in the
final digit, which is how that generation assigns consecutive addresses to the
wired and wireless interfaces; the mapping of the lower address to the wired
interface follows that convention and has not been confirmed on the device
itself.

The exact board model is **not confirmed**. Establish it on the device rather
than assuming it:

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

### 2.7 Power-control checks

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

NILO `0.2.0` was deployed to the mini PC on 2026-09-10 for live development
checks. It runs in the real `appliance` profile, not simulation, and serves the
built GUI over HTTPS:

| Property | Value |
|---|---|
| URL | `https://192.168.1.47:8443` |
| NILO username | `admin` |
| NILO password | `admin_password` |
| Core service | `nilo.service`, active and enabled |
| Privileged helper | `nilo-nethelper.service`, active and enabled |
| Recovery timer | `nilo-nethelper-recovery.timer`, active and enabled |
| Runtime data | `/var/lib/nilo` |
| GUI files | `/opt/nilo/gui` |

The `admin` / `admin_password` pair is the fixed **development-only NILO
credential** used by this repository. It is separate from the Ubuntu/SSH login
in section 3.1 and must not be used on a production or customer appliance. This
mini PC remains a disposable development target even though NILO uses the
`appliance` runtime profile so that live NetworkManager and privileged-helper
behavior can be exercised.

The current certificate is locally self-signed for the mini PC hostname, LAN
address and WireGuard address. A browser that does not trust it displays a
certificate warning; this is acceptable for this bench deployment only and is
not a production certificate model.

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
