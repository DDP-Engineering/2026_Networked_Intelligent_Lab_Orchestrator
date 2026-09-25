<!--
Copyright (c) 2026 DDP Engineering
This software is licensed under the PolyForm Noncommercial License 1.0.0.
Commercial use requires a separate commercial license.
See the LICENSE.txt in the .agentic folder for full license details.
SPDX-License-Identifier: LicenseRef-PolyForm-Noncommercial-1.0.0
-->


# Project profile

## Purpose and users

NILO, the Networked Intelligent Lab Orchestrator, is an electronics bench
appliance that brings instrument access, appliance networking and captured
evidence into one application for both people and AI agents.

Three audiences reach the same services: a bench engineer or technician through
the browser interface, automation through the `nilo` command line with a scoped
API token, and an AI client through the MCP endpoint. A human stays responsible
for wiring, safety and the approval of consequential actions; every actor holds
one of three fixed roles - Administrator, Operator or Viewer.

## Product responsibilities

- **One shared core behind three interfaces.** The Go Bench Core owns
  orchestration and persistence; the React browser interface, the `nilo`
  command line and the `/mcp` endpoint are clients of the same authenticated
  `/api/v1` services. The command line is an HTTP client even on the appliance.
- **Identity and access control.** Argon2id password verifiers, expiring
  `HttpOnly` browser sessions with a CSRF token, and named API tokens whose
  scopes are intersected with the owner's current role. The three roles map to
  a compiled-in permission set; there is no custom-role editor. One persisted
  appliance password policy - a minimum length and a minimum count of distinct
  character classes, above a compiled floor - governs every password being set,
  on the HTTP routes, the command line and the browser, and is never consulted
  when a stored verifier is checked.
- **Policy and approvals.** Ask, Auto and Full Access resolution over an
  integration's declared operation policy, immutable policy snapshots, and
  approvals bound to one canonical request digest.
- **Managed integrations.** Curated bundles are installed, enabled, disabled,
  restarted and uninstalled explicitly. The sigrok adapter exposes ATK-DL16
  discovery, finite digital capture on D0-D15 and UART decoding of saved
  captures; PWM is refused at both the core and the runner.
- **Instrument transactions.** Durable transaction identities, a bounded queue,
  two executing workers with per-analyzer serialization, absolute deadlines,
  ordered bounded replies, cancellation, and digest-verified retained artifacts.
- **Appliance networking.** Adapter state and settings, Wi-Fi scan and join,
  managed WireGuard profiles and routed device networks, each through preview,
  acknowledgement and a confirmation window with automatic restore.
- **First-boot commissioning.** A clean installation records a commissioning
  lifecycle state, derives one reset-stable printed `nilo-<short-id>` identity,
  prepares safe Ethernet links with IPv4LL and an owned Wi-Fi access point,
  answers its permanent name plus `nilo.local`, and serves management access on
  every eligible physical adapter. Commissioned appliances advertise the
  permanent DNS-SD service and interface-scoped SSDP presence; clients can
  discover/open them by serial or collision-checked friendly name and print an
  exact management-URL QR label. Authenticated About keeps the permanent
  identity and controls prominent, groups software/API/protocol, Linux/kernel/
  architecture/storage and management facts, and embeds the label as its only
  hardware-address presentation without exposing host paths. A
  user signs in to a newly imaged appliance
  with the administrator credential published with the image, over a
  helper-generated certificate that carries both first-contact names and is
  signed by nobody a browser trusts; completing
  commissioning is refused while that published password is still in place, so
  the credential cannot outlive first boot. The Raspberry Pi delivery may add a
  dual-duration GPIO button for a
  bounded, post-link-transition recovery path or a network-only return to
  commissioning; generic Linux hardware has no GPIO assumption.
- **Activity.** One filtered, paginated audit and event history with bounded
  JSONL and CSV export, an administrator retention policy, and bounded
  housekeeping.
- **Appliance power.** Confirmed restart and power off from an Operator or
  Administrator browser session, dispatched through the privileged helper.

## Supported operation

- NILO runs as an appliance: an unprivileged HTTPS Bench Core using the
  helper-owned per-appliance signing key, a built interface directory, an absolute data
  directory and an explicit listener, beside the privileged Linux helper that
  owns networking and key material.
- Appliance platforms are Linux ARM64, the first target and a Raspberry Pi 4,
  and Linux x86_64. Windows is a development host and a portable compile
  target, never an appliance platform.
- The browser interface offers tabbed workspaces, light, dark and system
  themes, and English, German and Romanian.
- Page-scoped View activity shortcuts sit immediately beside the page title
  in one left-aligned, wrapping header group on every page that offers them.
  Supporting descriptions and list controls follow that group; record-scoped
  actions remain with their records (REQ-UX-b75fbbc3).
- One layout serves the whole display range. Small low-resolution panels are
  supported from 800x480 up, with a seven-inch 1024x600 as the usual field
  size, and so are large high-resolution displays, where content is bounded to
  a readable width rather than stretched across it. Three viewport classes
  decide the shell; no operator-facing text falls below 14px and every
  interactive target is at least 44 by 44 pixels at any size.
- Persistence is SQLite through a pure-Go driver with digest-checked embedded
  migrations, plus a confined artifact root below the data directory.
- Behaviour is developed and tested against the appliance services on real
  bench hardware; `.agentic/context/environment.md` owns the development host,
  its setup and its commands.

## Established constraints

- Bench Core is authoritative. No interface invokes an integration process,
  edits the database or reaches a device directly.
- Two authorization decisions stay separate: `identity.Service` decides whether
  an actor may *request* an action; `policy.Resolver` decides whether an
  operation may *run* without a human approval. Neither the Administrator role
  nor a fully scoped token touches the second decision.
- No credential is accepted on a command line. Tokens are read from a file,
  passwords from standard input, and no response, log, audit event, export or
  error message carries a password, key or verifier.
- WireGuard key material and wireless passwords exist only inside the
  privileged helper's root-owned keystore; the core knows profiles by identity.
- Temporary network recovery never changes a configured management, production,
  PLC, or device-network interface. Its IPv4LL profile, management admission and
  `nilo.local` alias are helper/core-owned, deadline-bounded and exactly restored.
- The integration boundary is `nilo.integration/1`: correlated NDJSON over a
  supervised child process's stdio, a runner declared as an executable plus an
  argument array rather than a shell string, and a package whose complete file
  inventory is verified against `files.sha256.json`.
- The audit history is append-only against the application and bounded by
  retention. It is not forensic evidence and is never described as
  tamper-proof.
- While NILO is in active development, superseded functionality is removed
  directly; no compatibility aliases, parallel contracts or data conversion are
  added to preserve an earlier development build.
- One project-level Semantic Version lives in `VERSION`, mirrored by
  `internal/buildinfo` and `05_Software/frontend/package.json`.
- The project is licensed under PolyForm Noncommercial 1.0.0.
- **One package per delivery target, integrations included.** NILO ships as
  one software: a release package for the Raspberry Pi and one for the Ubuntu
  mini PC each contain Bench Core, the browser interface, the helper and every
  integration at its current state with all their dependencies, and install
  without internet access. Integrations are functionality of NILO developed
  and released with it, not separately released products; on the appliance
  they remain explicit to install and enable. The Raspberry Pi delivery
  additionally provides a directly writable card image. Each target is produced
  by its own packaging script, self-sufficient from the repository after a
  one-time verified fetch of external inputs.
- **A finite tested release matrix.** The initial delivery targets are Ubuntu
  24.04 LTS on amd64 and Raspberry Pi OS based on Debian 13 on arm64. Each exact
  target owns its offline dependency pool and acceptance result; a newer OS
  release becomes supported only through a separate target, pool, and
  acceptance run. The decisions are recorded in `07_Delivery/README.md`.

## Implementation orientation

- `05_Software/core/` - Go module `nilo.local/nilo`, built without
  CGO. `cmd/nilo/main.go` is the command surface (`version`, `serve`,
  `serve-foundation-check`, `development issue-token`, `admin`, `seed-admin`,
  `maintenance`, `network`, `plugin`, `bench`, `activity`);
  `cmd/nilo-nethelper/` is the privileged Linux
  helper. `internal/httpapi/server.go` holds the one route table where every
  route declares its permission, with `internal/httpapi/bench.go` beside it.
  `internal/identity/service.go` owns identity rules, `internal/auth/role.go`
  the role matrix, `internal/network/operation.go` the managed-change
  lifecycle, `internal/bench/` the plugin and job manager,
  `internal/integration/{manifest,protocol,supervisor}/` the package boundary,
  and `internal/store/sqlite/` the database with its embedded migrations.
- `05_Software/frontend/` - React, TypeScript and Vite. Requests are
  origin-relative under `/api`; `src/api/contract.ts` holds the client
  contract, `src/components/shell/pages.ts` the navigation, and
  `src/preferences/store.ts` is the only module that touches browser storage.
- `05_Software/integrations/sdk/python/` is the supported Python authoring
  surface; `05_Software/integrations/sigrok/` is the current curated package.
  `05_Software/contracts/integration/v1/` is the public, language-neutral
  package contract mirrored by the Go loader. The test-only Go runner in
  `05_Software/tests/fixtures/contract_runner/` exercises the shared boundary.
- Component detail, per-file routing and requirement links live in
  `.agentic/knowledge/`.

## Authoritative references

- `README.md` - product overview, capabilities and repository layout.
- `CHANGELOG.md` and `VERSION` - delivered changes and the shared version.
- `01_Documentation/Code_Style.md` - Go, Python and general code conventions.
- `05_Software/README.md` - software directory map and how the parts relate.
- `06_SoftwareDocu/Development_and_Simulation_Flow.md` - development topology,
  daily workflow and verification levels.
- `06_SoftwareDocu/Find_Your_NILO.md` - plain-language appliance naming and
  discovery choices for common client and network situations.
- `06_SoftwareDocu/Network_Operations.md` - network workflow, recovery and
  appliance preparation.
- `06_SoftwareDocu/Sigrok_Integration.md` - current instrument lifecycle,
  limits, shared interfaces and bundle deployment.
- `07_Delivery/README.md` - packaging and deployment decisions.
- `07_Delivery/NILO_CLI_and_Authentication.md` - command line, roles,
  permissions, sessions and tokens.
- `00_Planning/` - product direction and planned work, including the waveform
  workspace plan.
- `.agentic/knowledge/` - component pages for Bench Core, the browser GUI, the
  integration platform, managed bench integrations and the toolchain.
