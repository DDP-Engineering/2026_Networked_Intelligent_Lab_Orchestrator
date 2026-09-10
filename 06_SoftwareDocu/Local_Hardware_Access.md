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
real hardware only when the question is specifically about ARM64, Linux
packaging, or physical I/O, which simulation is explicitly not able to answer.

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

This device is the only place on the bench where the following can be observed
at all, because WSL2 x86_64 simulation cannot produce any of it:

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

## 3. Credential handling

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
