# Temporary Remote PC Worx Access — ILC 151 GSM/GPRS

## Status and scope

This is a temporary, case-specific operating note for the Raspberry Pi
`ddp-remote` and the ILC 151 GSM/GPRS currently addressed as
`192.168.0.130`.

It does **not** define, narrow, replace, or restrict the purpose, product scope,
architecture, requirements, or future capabilities of NILO. It is not a
general deployment standard. Update or remove it when this temporary setup is
retired or its addresses and credentials change.

Recorded and checked on 2026-09-08.

## Address and access identifiers

| Item | Identifier for this case | Notes |
|---|---|---|
| DCP server on the home LAN | `192.168.1.15` | WireGuard hub |
| DCP server WireGuard address | `10.205.148.1` | VPN-side hub address |
| DCP server SSH command | `ssh dpasare@192.168.1.15` | Home-LAN access |
| DCP server SSH username | `dpasare` | Temporary case credential |
| DCP server SSH password | `Cyborg806` | Temporary case credential; do not reuse elsewhere |
| WireGuard public endpoint | `ddp-engineering.go.ro:51820` | UDP 51820 |
| Laptop WireGuard peer | `MP_PC` | Peer recorded on the DCP server |
| Laptop WireGuard address | `10.205.148.10/32` | Verify after activating `MP_PC` |
| Phone WireGuard peer | `MP_PHONE` | Present on the DCP server; not needed for PC Worx |
| Phone WireGuard address | `10.205.148.9/32` | Separate from the phone hotspot function |
| Raspberry Pi hostname | `ddp-remote` / `ddp-remote.local` | `.local` works only where mDNS is available |
| Raspberry Pi NILO WireGuard profile | `WORK_Ubuntu` | Selected for startup in NILO |
| Raspberry Pi WireGuard interface | `nilo0` | NILO-managed tunnel |
| Raspberry Pi WireGuard address | `10.205.148.8/32` | Stable VPN identity for this Pi |
| Raspberry Pi PLC-side interface | `eth0` | Direct Ethernet connection to the PLC |
| Raspberry Pi PLC-side address | `192.168.0.1/24` | No default gateway on this interface |
| Physical PLC address | `192.168.0.130` | Used only on the Pi-to-PLC Ethernet segment |
| Remote translated PLC address | **`10.71.1.130`** | Address to use through WireGuard |
| PC Worx target address | **`10.71.1.130`** | Enter this address in PC Worx |
| RustDesk ID for the Pi | `309803716` | Temporary out-of-band desktop access |
| Current local NILO URL | `https://ddp-remote.local:8443` | Local bench only; depends on name resolution |
| Current numeric NILO URL | `https://192.168.1.103:8443` | Current bench DHCP address; not valid as a field invariant |

The hotspot SSID and password have not been recorded in this note. Add them
only when the field profile is selected. The hotspot network must not use
`192.168.0.0/24`, because that would overlap the directly connected PLC
network.

## Address to enter in PC Worx

Enter this controller address:

```text
10.71.1.130
```

Do **not** enter `192.168.0.130` when connecting remotely. That is the PLC's
physical cabinet-side address and may conflict with the laptop, hotspot, or
another site's local network.

NILO translates the address as follows:

```text
PC Worx -> 10.71.1.130 -> NILO DNAT -> 192.168.0.130
PLC reply -> 192.168.0.1 on the Pi -> NILO/WireGuard -> PC Worx
```

NILO also source-NATs the forwarded connection to `192.168.0.1`, so the PLC
does not need a route or default gateway back to the WireGuard network.

## Confirmed live state

The following was observed directly on 2026-09-08:

- the Pi could ping the physical PLC at `192.168.0.130`;
- the PLC accepted TCP connections from the Pi on ports `21`, `80`, `1962`,
  and `41100`;
- the Pi's `nilo0` tunnel had a recent WireGuard handshake with the DCP server;
- the NILO device network was enabled and its nftables DNAT, SNAT, and
  forwarding rules were active;
- the NILO rule exposed only `10.71.1.130` and rejected direct forwarded access
  to `192.168.0.130`;
- the DCP server's Pi peer already had
  `10.205.148.8/32` and `10.71.1.130/32` in its `AllowedIPs`;
- the DCP server route for `10.71.1.130` selected `wg0`;
- TCP port `1962` on `10.71.1.130` was reachable from the DCP server through
  the Pi to the PLC;
- pinging `10.71.1.130` from the DCP server did not answer because the current
  NILO service policy permits TCP and UDP but not ICMP. This failed ping is not
  evidence that the PC Worx TCP path is down.

The DCP server currently has IPv4 forwarding enabled and UFW reports its routed
default as `allow`.

## Required connection sequence

1. Connect the Pi's `eth0` port directly to the ILC 151 GSM/GPRS.
2. Start the predefined mobile hotspot.
3. Confirm that the hotspot subnet does not overlap `192.168.0.0/24`.
4. Confirm that the Pi joins the hotspot and that NILO activates
   `WORK_Ubuntu`.
5. Confirm a recent handshake for the Pi peer `10.205.148.8` on the DCP server.
6. On the laptop, activate the WireGuard profile `MP_PC`.
7. Confirm that the laptop receives `10.205.148.10/32`.
8. Test TCP port `1962` on `10.71.1.130` from the laptop.
9. Start the PC Worx virtual machine using VirtualBox NAT networking.
10. Test TCP port `1962` from inside the virtual machine.
11. In PC Worx, select direct IP communication and enter `10.71.1.130`.
12. Perform a non-destructive connection/read check before downloading a
    program.
13. Download the new program only with an authorized person present at the
    cabinet and the machine/process in a safe state.

## Laptop checks

After enabling `MP_PC`, run in Windows PowerShell:

```powershell
Get-NetIPConfiguration
Test-NetConnection 10.205.148.1 -Port 22
Test-NetConnection 10.205.148.8 -Port 22
Test-NetConnection 10.71.1.130 -Port 1962
Test-NetConnection 10.71.1.130 -Port 41100
```

Expected for the programming path:

```text
ComputerName     : 10.71.1.130
RemotePort       : 1962
TcpTestSucceeded : True
```

A failed `ping 10.71.1.130` is expected with the current NILO policy. Use the
TCP tests above.

If the laptop WireGuard profile is later converted from full-tunnel to
split-tunnel, its peer `AllowedIPs` must include at least:

```ini
AllowedIPs = 10.205.148.0/24, 10.71.1.130/32
```

## VirtualBox and PC Worx

Use a VirtualBox **NAT** adapter for the first test. NAT lets the VM use the
Windows host's WireGuard route. Do not start with a bridged adapter: a bridged
guest does not automatically inherit the host's WireGuard interface or routes.

Inside the Windows VM, run:

```powershell
Test-NetConnection 10.71.1.130 -Port 1962
Test-NetConnection 10.71.1.130 -Port 41100
```

PC Worx communication relevant to this PLC was observed on:

| Function | Address | TCP port |
|---|---|---:|
| PC Worx controller communication | `10.71.1.130` | `1962` |
| PC Worx debug/OPC communication | `10.71.1.130` | `41100` |
| PLC web interface, if needed | `10.71.1.130` | `80` |
| PLC FTP, if needed | `10.71.1.130` | `21` |

Automatic broadcast discovery may not work through this connection. The path
is routed and translated, not bridged Ethernet. Use the explicit target
address `10.71.1.130`.

## DCP server verification

Connect from the home LAN:

```bash
ssh dpasare@192.168.1.15
```

Then verify without displaying WireGuard secret material:

```bash
sudo wg show wg0 allowed-ips
ip route get 10.71.1.130
timeout 3 bash -c '</dev/tcp/10.71.1.130/1962'
```

The Pi peer must continue to own this route:

```text
10.205.148.8/32
10.71.1.130/32
```

Do not assign `10.71.1.130/32` to a second WireGuard peer.

## Current NILO GUI limitation

The NILO GUI currently listens only on the Pi's bench Wi-Fi address
`192.168.1.103:8443`. It is not currently listening on
`10.205.148.8:8443`, and `192.168.1.103` will normally change when the Pi joins
a phone hotspot.

Therefore:

- RustDesk remains the current out-of-band way to view the Pi desktop;
- the PLC forwarding path can work remotely even while the NILO GUI itself is
  not directly reachable through the VPN;
- before field use, NILO needs a stable VPN-side management listener and a TLS
  certificate valid for that address or its VPN DNS name.

This limitation does not change the PC Worx target address; it remains
`10.71.1.130`.

## Temporary credential warning

The SSH and RustDesk credentials used for this bench are temporary and are
recorded in repository documentation. Treat them as exposed. Rotate them before
using the Pi or DCP server for anything non-disposable, customer-facing, or
production-related, and remove this temporary note when the case is closed.
