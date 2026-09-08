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
| Password | `Cyborg806` |

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
| RustDesk ID | `309803716` |
| Permanent password | `Cyborg806` |
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

Installed on 2026-09-02 from a peer configuration supplied by the developer.
`wireguard-tools` 1.0.20210914-3, with the in-tree kernel module.

| Property | Value |
|---|---|
| Interface | `wg0`, from `/etc/wireguard/wg0.conf`, mode `0600 root:root` |
| Peer endpoint | `ddp-engineering.go.ro:51820` |
| Tunnel addresses | `10.205.148.4/32`, `fd11:5ee:bad:c0de::acd:9404/128` |
| Routing | `AllowedIPs = 0.0.0.0/0, ::/0` — full tunnel |
| Tunnel DNS | `10.205.148.1` |
| At boot | **not** enabled; `wg-quick@wg0` is `disabled` |

The keys are deliberately not in this table and not in this repository. See
section 3.2.

This is a full tunnel, so every packet the device originates leaves through the
peer and its public address becomes the endpoint's rather than the bench's.
Local subnet traffic is not affected: `wg-quick` installs a
`suppress_prefixlength 0` rule, so the `192.168.1.0/24` route in the main table
still wins and SSH and RustDesk over the bench LAN keep working with the tunnel
up. Verified: handshake established, egress address reported as the endpoint's,
and the route to the development host still direct on `eth0`.

Bring it up and down by hand:

```bash
sudo wg-quick up wg0
sudo wg-quick down wg0
sudo wg show wg0
```

Two traps are worth keeping written down.

The `DNS =` line makes `wg-quick` call `resolvconf`, which Raspberry Pi OS does
not ship. The package that satisfies it is **`openresolv`**, not `resolvconf`.
Installing Debian's `resolvconf` package instead repoints `/etc/resolv.conf` at
a file it never populates and takes name resolution on the device down until it
is purged and the symlink is aimed back at
`/run/NetworkManager/resolv.conf`. That was done here and undone; do not repeat
it.

Bringing a full tunnel up over SSH can strip the route that the SSH session is
running on. Arm a dead-man switch first and cancel it once the session has
proved it survived:

```bash
sudo systemd-run --on-active=240 --unit=wg-deadman /usr/bin/wg-quick down wg0
sudo wg-quick up wg0
# once verified
sudo systemctl stop wg-deadman.timer
```

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

The password above is committed deliberately, so the constraints that make that
acceptable have to hold and have to keep holding:

- the device is a disposable test target on the local bench, reachable from the
  local network only, and is never exposed to the internet or given a
  port forward;
- it is never given a production, delivery, or signing role, and no credential,
  key, token, or customer data that matters anywhere else is placed on it;
- the password is used by this device and nothing else, and is not reused for
  any account, host, or service;
- anything the device is trusted with is treated as public.

If any of those stops being true, the password is no longer safe to keep here.
Rotate it on the device, remove it from this document, and move it to an ignored
local file — a committed value cannot be recalled from the history of a
repository that has already been cloned or delivered, so the rotation is the
part that does the work, not the deletion.

### 3.1 What RustDesk changes about that reasoning

The RustDesk installation in section 2.5 breaks the first constraint above, and
this is recorded rather than argued away.

RustDesk registers outbound with the public rendezvous server
`rs-ny.rustdesk.com` and falls back to a public relay. No port was forwarded and
no inbound rule was added, but the effect of the first constraint is gone
anyway: the device is now reachable from outside the bench by anyone who holds
the ID and the password, both of which are committed to this repository a few
lines above, in a repository that may already have been cloned or delivered.
The ID is stable, so this does not expire on its own.

It also concentrates the risk. The same string is now the SSH login and the
RustDesk password, so one committed value is the whole of both doors, and the
RustDesk one is answerable from the internet.

The remaining constraints are what keep this acceptable, and they now have to
carry more weight than they did: the device stays disposable, holds nothing that
matters anywhere else, takes no production, delivery, or signing role, and
everything on it is treated as public. If that is not enough, the fix is to
change the exposure and not the wording — set a private RustDesk relay, or stop
the service when it is not in use:

```bash
sudo systemctl disable --now rustdesk
```

### 3.2 The WireGuard keys are not covered by any of this

Section 3 permits a committed password because the thing it protects is
disposable. The WireGuard private key and preshared key in section 2.6 are the
opposite: they authenticate this device to a network that is not this bench and
is not disposable, so they get none of that latitude and are handled as ordinary
secrets.

They live in exactly two places — `/etc/wireguard/wg0.conf` on the device, mode
`0600 root:root`, and the peer file the developer holds outside the repository.
The peer file arrived as `DDP_Ubuntu.conf` in the working tree, untracked but
not ignored, one `git add -A` away from being committed permanently; `.gitignore`
now excludes it and any `wg*.conf`. Keep it out of the tree entirely rather than
relying on that.

If either key is ever committed, ignoring or deleting the file does not undo it.
Rotate the peer on the WireGuard server, exactly as section 3 says about the
device password.

Devices added to this document later are not covered by the reasoning above.
Each one needs its own decision, and the default for anything that is not
disposable is that its credentials stay out of the repository.
