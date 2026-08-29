# Networked Intelligent Lab Orchestrator (NILO Assistant) - Planning Overview

## Purpose

Create an electronics bench appliance that gives AI clients, such as Codex Desktop and Claude Code, controlled access to physical instruments. The system combines electronic measurements, embedded debugging, visual inspection, and carefully gated physical control so that an AI can collect evidence, reason about it, and help perform debugging work.

The objective is not an autonomous robot lab. A human remains responsible for wiring, safety, and approval of consequential actions. The system should make the bench easier to inspect, operate, extend, and troubleshoot.

## Product Model

The product has three distinct interfaces:

```text
Human GUI ───────┐
MCP AI clients ─┼── Bench Core ── Plugin host ── Instruments and adapters
Admin CLI ───────┘
```

- **Human GUI:** A browser-based configuration and monitoring application hosted by the bench appliance. It manages device configuration, networks, plugins, safety limits, relays, and status.
- **MCP API:** The northbound interface for AI clients. It exposes clear, typed, goal-level tools rather than arbitrary shell, SCPI, OpenOCD, or relay commands.
- **Plugin API:** The internal, versioned contract for instrument integrations. Plugins are not separate MCP servers in the initial design; the Bench Core owns sessions, authorization, artifact storage, audit logs, and safety policy.

## Primary Hosts

The first product platform is Linux, with one shared application design across two supported CPU architectures:

| Appliance | Architecture | Delivery approach |
|---|---|---|
| Raspberry Pi 4 | Linux ARM64 | Dedicated appliance image |
| Dedicated mini PC | Linux x86_64 | Installer bundle, later an appliance ISO/image |

Windows is not a primary platform in the first version. If an instrument later requires Windows-only vendor software, add a narrowly scoped Windows bridge plugin rather than creating a second complete product.

## Instrument Strategy

Each plugin declares a manifest, configuration schema, capabilities, health check, typed operations, artifact outputs, and a safe shutdown operation.

Initial plugin families:

- **Logic analysis:** sigrok/libsigrok and `sigrok-cli` for device discovery, capture, protocol decoding, analysis, and export. PulseView remains an optional human-facing viewer; it is not the MCP control layer.
- **Embedded debug and flashing:** OpenOCD, J-Link, CMSIS-DAP, ST-Link, or other supported probe backends. Initial tools should cover target discovery, verified flashing, reset, logs, and read-only inspection before permitting write-level debug commands.
- **RTT and SystemView:** RTT logs are a useful early integration. SystemView is a later specialist plugin because it needs target-side instrumentation and should be verified on the selected Linux ARM distribution.
- **Programmable test equipment:** SCPI/VISA instruments, including power supplies, multimeters, oscilloscopes, and signal generators. Begin with discovery and read-only measurements.
- **Visual evidence:** A board camera for physical inspection and an optional HDMI-to-USB capture device for instrument-screen capture. Direct waveform and instrument data is preferred over interpreting screen images.
- **GPIO and relays:** A relay board can switch named, documented signals such as power, reset, or a UART disconnect. It must start in a defined safe state.

## Evidence and Sessions

Every operation should produce a durable, addressable result where appropriate:

- a capture identifier;
- raw artifacts such as `.sr`, CSV, VCD, firmware logs, or trace files;
- structured measurements and protocol-decode events;
- model-friendly plots, waveform images, or camera images;
- timestamps, instrument identity, configuration, and audit information.

Long-running acquisition, flashing, and debug actions need explicit session ownership, status reporting, cancellation, and cleanup. Only one conflicting hardware operation should run on an instrument at a time.

## Networking and Remote Access

The Raspberry Pi may be installed at a remote location using its local Wi-Fi or Ethernet uplink. It establishes an outbound WireGuard client tunnel to the DCP server, giving it a stable private VPN address.

```text
Codex / Claude host ── WireGuard ── DCP server ── WireGuard ── Bench appliance
```

The MCP service and GUI should bind to the WireGuard interface or be protected by an authenticated proxy. They must not be exposed directly to the public internet.

### PLC and Remote Ethernet Networks

The Pi can use a USB-to-Ethernet adapter as a second, isolated network interface for a PLC or other local industrial device. This is a routed topology, not an Ethernet bridge.

Example:

```text
Home LAN:       192.168.1.0/24
WireGuard VPN:  10.205.148.0/24
PLC segment:    192.168.0.0/24
PLC:            192.168.0.130
Pi PLC adapter: 192.168.0.10/24, with no default gateway
```

The DCP WireGuard peer for the Pi advertises the approved PLC destination, preferably `192.168.0.130/32` before expanding to a whole subnet. The Pi forwards VPN traffic to its PLC adapter and source-NATs that specific traffic, so the PLC does not need a new gateway route. For normal home-LAN access, the home router or a selected home computer needs a route for the PLC destination through the DCP server.

Only explicitly allowed PLC ports and destinations should be reachable. Do not provide an AI with unrestricted access to an industrial subnet.

## Safety Model

Physical operations are higher risk than data collection. Safety must be enforced by the Bench Core and hardware policy, not by trusting the AI model or its client UI.

- Read-only actions are the default.
- Power, relay, erase, flashing, reset, memory write, and signal switching actions require an approval gate.
- Tools enforce allowlisted devices, firmware paths, voltage/current limits, timeouts, channel limits, and target identities.
- Risky actions require both a human approval flow and an armed appliance state or physical interlock where practical.
- Relay defaults must be safe after boot, service failure, or loss of network connectivity.
- Every state-changing action is recorded with the request, applied limits, result, and returned evidence.

## Human Configuration GUI

The GUI should make the appliance usable by an electronics professional without requiring routine Linux administration. It should provide:

- network-interface and WireGuard enrollment status;
- PLC subnet/address configuration and reachability tests;
- instrument discovery, enable/disable controls, and plugin configuration;
- a visual relay/signal map with safe-state definitions;
- safety policy, permissions, and required-approval settings;
- capture/artifact browsing, live status, logs, diagnostics, updates, and backup/restore.

The GUI writes validated, versioned configuration through the Bench Core. It does not directly edit firewall rules or invoke instrument commands.

## Installation and Updates

The user experience should be one product, even though installation and runtime have separate security responsibilities.

### Initial delivery: installer bundle

Provide a release ZIP or USB package for supported Linux hosts. A single launcher performs prerequisite checks, installs required packages, configures services, starts the setup wizard, and can repair an existing installation. It should clearly report each action and offer actionable recovery guidance.

### Appliance delivery: Raspberry Pi image

Provide a custom ARM64 Raspberry Pi OS image once the installer is stable. The image includes the Bench Core, GUI, service definitions, WireGuard tooling, firewall templates, and supported dependencies. Raspberry Pi Imager handles first Wi-Fi, hostname, account, and SSH configuration; the Bench Setup wizard completes device-specific enrollment.

Do not place shared secrets in an image: Wi-Fi credentials, WireGuard private keys, SSH credentials, PLC credentials, and API tokens are generated or supplied during first-boot enrollment. Each appliance has its own identity.

Updates must use versioned, integrity-checked release packages with a visible rollback/recovery path. Do not use opaque `curl | shell` installation flows.

## Recommended Delivery Sequence

1. Build the Bench Core, configuration model, artifact/session model, and browser GUI.
2. Add one sigrok logic-analyzer plugin with capture, UART/I2C/SPI decode, export, and rendering.
3. Add one verified debug/flashing backend with strict target and firmware allowlists.
4. Add WireGuard enrollment and the narrow PLC routed/NAT topology.
5. Add a read-only SCPI/VISA integration.
6. Add controlled power and relay operations after safety and interlock validation.
7. Add camera and SystemView integrations.
8. Turn the proven installer into reproducible Raspberry Pi and mini-PC appliance deliveries.

## Decisions Recorded

- The product is Linux-first: ARM64 Raspberry Pi and x86_64 mini PC share one core design.
- MCP is the AI-facing API; the GUI and plugin API are separate concerns.
- The Pi is a controlled hardware gateway, not the primary AI/vision inference host.
- PulseView is optional for people; sigrok/libsigrok is the programmable acquisition foundation.
- The platform uses WireGuard for remote reachability and isolated Ethernet routing for PLC access.
- Extensibility is based on a strict plugin contract, not a generic unrestricted command channel.
- Safety, authorization, and evidence retention are core features from the beginning.
