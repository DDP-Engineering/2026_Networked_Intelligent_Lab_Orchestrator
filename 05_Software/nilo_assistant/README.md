# NILO Assistant Go Bench Core

This Go module is the NILO Bench Core. External Python integrations connect
through the versioned integration boundary rather than becoming part of the
core process. The module provides the portable Increment 1 foundation plus a
loopback-confined development simulation server. It does not yet provide
hardware behavior, simulated inventory, a browser GUI, or an MCP service.

Linux ARM64 is the first target and Linux x86_64 is the later appliance target.
Windows x86_64 is verified only as a portable compile target.

The currently implemented application commands are:

```text
nilo version
nilo serve-foundation-check --data-dir <absolute-path>
nilo development issue-token --data-dir <absolute-path>
nilo serve --profile simulation [--scenario healthy|unavailable] --data-dir <absolute-path> --bind <loopback-address>
```

The simulation server exposes authenticated `GET /api/v1/status`, `GET
/api/v1/integrations`, and `GET /api/v1/devices` routes using the `nilo.http/1`
contract. The optional scenario is deterministic and defaults to `healthy`;
`unavailable` preserves stable identities while changing readiness. Development
token issuance creates a non-login identity, stores only the one-way token
verifier, and prints the plaintext token once. The simulation listener rejects
wildcard, hostname, LAN, and other non-loopback addresses.

For the complete Windows-browser and WSL2-backend workflow, including exact
commands, see `../../06_SoftwareDocu/Development_and_Simulation_Flow.md`.

Run the complete foundation gate from the repository root with
`05_Software\scripts\test-increment-1.ps1` on Windows or
`./05_Software/scripts/test-increment-1.sh` on Linux. See
`05_Software/INCREMENT_1_ACCEPTANCE.md` for observed evidence and the mandatory
Raspberry Pi ARM64 gate.
