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
