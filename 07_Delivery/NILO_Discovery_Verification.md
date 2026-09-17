# NILO discovery verification

## Candidate contract

Each appliance derives and persists one printed `nilo-xxxxxxxxxx` name, keeps
its ECDSA private key in the root helper, and serves the helper's current public
certificate through a constrained signer. Serial mDNS/DNS-SD, optional recovery
and friendly aliases, SSDP, eligible owned DHCP clients, the provisioning Wi-Fi
path, authenticated About, the loopback status page, and CLI discovery all use
that identity. Publication remains limited to current management interfaces.

The first administrator remains `admin` / `admin_password` during provisioning.
It is a published bootstrap credential and must be replaced before
commissioning. The image contains no appliance TLS private key: the helper
generates a distinct key on first boot, stores it outside Bench Core, and never
exports it. Browsers still show a signer warning for the self-signed `.local`
certificate; verify the printed permanent name before accepting it.

## Automated evidence routes

- Go unit and contract tests: `go test ./...` from
  `05_Software/nilo_assistant` through the pinned toolchain.
- GUI component and production build: `npm run test:ci` and `npm run build`
  from `05_Software/nilo_gui` through the repository wrapper.
- Disposable privileged networking: `02_Tools/tests/network/check.sh`.
- Appliance-profile browser and listener boundary:
  `02_Tools/tests/appliance/check.sh`.
- Repository strict gate: `python .agentic/aegis.py check --task
  TASK-2361ce94 --strict --run` during AEGIS validation.

## Hardware and client observations

The following are acceptance evidence only after they are observed and dated;
source code or a lower-level test is not a substitute.

| Client or boundary | Observation to record | Bound |
|---|---|---|
| Windows 11 | Network device appears, opens the HTTPS presentation URL, then disappears after withdrawal | 60 s |
| macOS | `nilo discover`, printed `.local` URL, and `nilo open` agree on UUID and URL | configured timeout |
| Ubuntu | mDNS and SSDP independently find the same device; direct Ethernet works through IPv4LL | configured timeout |
| Router | DHCP lease carries the serial hostname and NILO vendor identity; router DNS behavior is stated | router-specific |
| Phone | provisioning SSID is visible, WPA2 join succeeds, DHCP/DNS stay on the AP, captive probe redirects | provisioning only |
| Raspberry Pi | short, 3 s, and 10 s releases produce Identify, recovery, and reset respectively; discovery refresh timestamps are captured | documented gesture bounds |
| Isolation | device, WireGuard, loopback, uplink, and non-allowlisted namespaces receive no presence/captive traffic | zero packets/responses |

Record packet timestamps, interface identities, before/after host routes, and
the exact candidate revision. Keep credentials, keys, cookies, packet bodies
containing secrets, and raw hardware identity sources out of this report.

## Early target-hardware signal

The following 2026-09-16 run is a build-stage signal, not the complete client
acceptance matrix. ARM64 binaries built from the uncommitted
`feature/identify_nilo` candidate were run on the documented Raspberry Pi 4
with throwaway copies of Core and helper state. The installed NILO 0.7.0
services were stopped for the run, restored active afterward, and the remote
temporary state was removed.

| Boundary | Observed result |
|---|---|
| Permanent identity | Candidate reported `nilo-n0hzmysj9v`; the copied commissioned state and helper-derived identity agreed. |
| mDNS/CLI | Same-host `nilo discover --timeout 5s --json` returned the permanent serial, UUID and `https://nilo-n0hzmysj9v.local:8443/` from mDNS in 5.000 s with no diagnostic. |
| Local status and label | Loopback status showed the same serial, commissioned state and management URL; its rendered label contained that exact URL including port 8443. |
| Helper-backed TLS | A loopback-bound appliance listener using the real privileged helper presented a self-signed certificate with SANs `nilo.local` and `nilo-n0hzmysj9v.local`; no private key crossed into Core. |
| Management scope | The live copied configuration selected only `wlan0` and `nilo0`. An attempted temporary reassignment of `eth0` was correctly refused because that adapter owns the device role. |
| Installed appliance | After cleanup, both installed services were active and `/usr/local/bin/nilo version` still reported 0.7.0. |

No GPIO claim is made because the button/LED wiring is not confirmed. A
separate Windows client observation was not possible in this run: the saved
DDP Wi-Fi profile did not authenticate, the DDP WireGuard tunnel was inactive,
and the direct Ethernet adapter is intentionally a device-role link. SSDP,
Windows Network view, withdrawal timing, DHCP/router, phone provisioning and
physical button timing therefore remain open acceptance evidence.

## Integrated 0.8.0 Raspberry Pi deployment

On 2026-09-17, after merging current `master` into
`feature/identify_nilo`, the integrated ARM64 core, helper and production GUI
were deployed to the documented Raspberry Pi 4. Existing Core/helper state,
administrator data, NetworkManager profiles and TLS material were preserved.
The root-only rollback archive is
`/var/backups/nilo/pre-identify-0.8.0-20260917T063707Z/runtime-and-state.tar.gz`
with SHA-256
`d15585cf44f2db9bbde4361d1263947d9e6eaa5eb39e1b8af06997216aa8454a`.

The installed core and helper SHA-256 values are respectively
`03bd66c1eec715bc17fb7e047fd322e5512af2e6110b98eef0c6bcf178c7486c`
and `8697d1594ae4788765e32fc606fdf860dec6ddd1fcf2d8a06e42e32e128a4de7`.
`nilo version` reports 0.8.0 and `nilo.integration/1`; the core, helper and
recovery timer are active and enabled. The helper-owned firewall returned to
the persisted `wlan0`/`nilo0` allowlist after temporary deployment probes.
From the Windows WireGuard peer, the GUI returned HTTP 200 and the protected
status route returned HTTP 401. The served GUI references the tested
`index-CM43bA9-.js` production asset.

The live authenticated browser showed 0.8.0, permanent name
`nilo-n0hzmysj9v`, commissioned lifecycle, the About identity controls, the
current WireGuard path, and master's password-policy panel. The current product
contract recognizes the permanent serial name, a configured friendly alias and
the provisioning-only shared name. A development-image hostname and a routed
VPN address are transport details, not additional NILO identities or required
certificate names. On that contract, the helper-generated certificate's
`nilo.local` and `nilo-n0hzmysj9v.local` SANs are correct.

The installed core service was subsequently changed to the current
helper-managed TLS mode by removing only its inherited file-backed certificate
and key arguments. The live certificate has subject `CN=nilo.local` and SANs
`nilo.local` and `nilo-n0hzmysj9v.local`; the helper key is root-owned mode 0600,
is unreadable by the Core account and is not open by the Core process. From the
Windows WireGuard peer, the GUI returned HTTP 200 and the protected status route
returned HTTP 401 after the restart. Core and helper remained active with zero
Core restart retries.

Before that service-only change, a root-only rollback was created at
`/var/backups/nilo/pre-helper-tls-0.8.0-20260917T114203Z`; its configuration and
state archive has SHA-256
`7f8a23ffc494f4665228b26b779c1e27fa4aea4186509397af7f3c7109e24c3`.
All NetworkManager profile hashes matched before and after. `wlan0`, `nilo0`
and `eth0` retained their addresses, the WireGuard handshake remained current,
and the serialized ILC151 forwarding plan matched byte-for-byte and remained
installed in nftables. The PLC itself did not answer ICMP or TCP 504 during the
post-change check, so its current availability is not claimed.

The remaining live acceptance finding is:

- **Identify appliance** reported that Identify could not be started. The
  helper remains installed without `--raspberry-pi-recovery`, matching the
  prior appliance configuration, because the BCM17 button/BCM27 LED wiring has
  not been confirmed. No physical Identify or button-duration result is
  claimed.

Consequently the integrated software is deployed and the current VPN transport
is usable. Helper-backed TLS activation in the installed service now has live
target evidence. GPIO acceptance remains changes-required; no legacy hostname
or address compatibility is part of the product acceptance contract.

The printable-label correction was subsequently deployed as core SHA-256
`b01b8032ac711efd94d5fc594f0ff4e697a295ff418290f14a52829a8ab7bd53`.
The QR grew from 25 mm to 27 mm, the direct browser preview renders at 620 by
290 pixels, and print media remains fixed at 62 by 29 mm. The focused QR test
decoded the rasterized generated SVG to the exact permanent management URL;
the live VPN label was refreshed and visually inspected in Chrome. Core stayed
active, with GUI HTTP 200 and protected status HTTP 401. The previous core is
preserved at `/var/backups/nilo/pre-label-qr-20260917T070040Z/nilo`.

The About dashboard correction was then deployed with ARM64 core SHA-256
`76b0ba2d51ac5bd37a51a91d14ac8336a69859b0be68977fdbc3113aa0dea429`
and GUI archive SHA-256
`d2d7a77bb0c184837b1539fa38fb0351d96886d76331a74e204e227339ad1216`.
Rollback is preserved at
`/var/backups/nilo/pre-about-dashboard-20260917T074724Z/`. The live Chrome
page showed version 0.8.0, Debian GNU/Linux 13, kernel
`6.18.39+rpt-rpi-v8`, ARM64, 79 GiB free of 96 GiB, the permanent identity in
the title row, the controls first, management facts without a table, and the
real label embedded beside them. The installed core stayed active; VPN GUI and
protected-status probes returned HTTP 200 and 401 respectively.

The follow-up About grouping correction was deployed as GUI archive SHA-256
`b0ca2bf86ac73bcbe6c425acc98546fa35b5005eeabc85ee3c345b8ba4e3a394`.
The root-only rollback archive is
`/var/backups/nilo/pre-about-groups-20260917T080520Z/gui.tar.gz`, SHA-256
`aa21b29d79e697efea7cc7796908c0c215866549593f83d3a91cdd2c8b01bbf7`.
The live Chrome page showed separate Software, System and Management regions,
the page scroll rail at the workspace edge, and no hardware-address fact beside
the label; the hardware address remains printed once inside the embedded label.
The Pi core stayed active and the VPN GUI/status probes returned HTTP 200/401.
