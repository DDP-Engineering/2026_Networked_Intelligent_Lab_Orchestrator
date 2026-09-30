# Networked Intelligent Lab Orchestrator (NILO) — Product Overview

## NILO: A Bench Companion

NILO is an electronics bench companion that allows people and AI agents to connect to physical equipment, inspect its behavior and carry out controlled operations. It brings measurements, embedded debugging, board programming, serial communication, visual inspection and equipment control into one system.

NILO stays beside the bench on a home or corporate network. A user works with it from a browser or an agent application, either on the same network or remotely through a VPN. The user remains responsible for physical connections, the equipment under test and approval of operations that require human supervision.

NILO makes the bench accessible through a consistent interface while plugins handle the differences between instruments, adapters, protocols and vendor tools.

## People, Agents and Equipment

NILO provides three ways to interact with the bench:

- **Browser GUI:** People configure the appliance, connect equipment, operate plugin workspaces, inspect results and manage access.
- **MCP interface:** AI agents discover available tools, inspect equipment state and request authorized operations through the Model Context Protocol.
- **Administration CLI:** Administrators configure, diagnose and automate appliance management through a command-line interface.

```text
Browser GUI ────┐
AI agent / MCP ─┼── NILO Bench Core ── Plugins ── Bench equipment
Admin CLI ──────┘
```

The Bench Core coordinates access, equipment ownership, operation policies and results. Plugins expose their capabilities through the shared NILO interfaces, so people and agents work with the same equipment configuration and permission rules.

NILO supports agent applications such as Codex, Claude Code, OpenCode and Qwen Code through their MCP integration. Extensible agents such as Pi use an appropriate MCP integration, and applications such as LM Studio connect as MCP hosts when operating in that role. Client-specific setup is available from NILO's GUI.

The agent and its model can run in different places. A laptop agent can use a local model through LM Studio or a cloud model endpoint such as Scaleway while communicating directly with NILO. The machine executing MCP calls has network access to NILO; the model provider does not need its own connection to the bench in this arrangement.

## Appliance and User Computers

NILO runs on Linux appliances with a shared product design across ARM64 and x86_64 systems:

| Appliance | Typical use |
| --- | --- |
| Raspberry Pi 4 / ARM64 appliance | A compact companion installed beside the bench. |
| Dedicated x86_64 mini PC | A bench host with additional processing, storage or connectivity capacity. |

Users access NILO from Windows, macOS or Linux computers using a browser or a compatible agent application. Equipment that depends on vendor software on another host connects through an appropriate integration bridge.

NILO runs the bench services and equipment integrations. The user chooses where the agent and AI inference run, independently of the appliance.

## Equipment and Plugin Capabilities

Plugins connect NILO to different equipment families. Each plugin describes its devices, configuration, available operations and operating limits. The GUI presents an equipment workspace, while the MCP interface exposes the corresponding agent tools.

NILO's plugin capabilities include:

- **Logic analysis:** Discover logic analyzers, capture digital signals, decode protocols such as UART, I2C and SPI, inspect waveforms and export captures. Integrations use backends such as sigrok/libsigrok; users can also inspect compatible captures in tools such as PulseView.
- **Board programming and embedded debugging:** Discover targets, flash and verify firmware, reset boards, inspect memory and registers, and collect debug output through supported OpenOCD, J-Link, CMSIS-DAP, ST-Link and other probe integrations.
- **Serial communication:** Connect USB-to-serial adapters, configure communication parameters, exchange data, inspect terminal output and retain communication logs.
- **Modbus communication:** Configure endpoints and register maps, read values, poll equipment and issue controlled writes over supported serial and TCP/IP transports.
- **Programmable instruments:** Discover and operate power supplies, multimeters, oscilloscopes, signal generators and other equipment through SCPI/VISA and device-specific integrations.
- **Runtime tracing:** Collect and inspect embedded runtime information through RTT, SystemView and compatible tracing integrations, with the required instrumentation on the target.
- **Cameras and visual inspection:** View boards and bench setups, capture images, adjust camera controls, record activity and inspect instrument displays through cameras or HDMI-to-USB capture devices.
- **GPIO, switching and relays:** Control named signals such as power, reset and communication-line connections, with configured operating limits and defined safe states.

Supported operations depend on the selected plugin, connected hardware and device capabilities. Plugins provide measurements, images, waveforms, logs and other results through the same NILO workspace used by people and agents.

## Local and Remote Connections

### On the Home or Corporate Network

NILO connects to the local network through Ethernet or Wi-Fi. People open its browser interface, and agents connect to its MCP endpoint using a scoped access token.

```text
Laptop: browser / agent ── Home or corporate LAN ── NILO ── Bench equipment
```

A stable private address or internal DNS name identifies the appliance. The same saved endpoint and access token work locally and remotely when the VPN provides the corresponding route and name resolution.

### Away from the Bench

NILO supports remote access through WireGuard, OpenVPN and compatible home, corporate or provider-managed VPN services. The VPN can terminate at the network's router or gateway, or NILO can participate directly as a VPN client using a supported connection profile.

```text
Laptop away from home/work
    │
Existing VPN
    │
Home router / corporate gateway
    │
Private LAN ── NILO ── Bench equipment
```

For example, a user on holiday connects their laptop to the home VPN, opens NILO and uses an agent to work with the equipment left on the bench. NILO continues operating on its ordinary home-network address. It does not need to host the VPN itself.

Corporate deployments use the organization's VPN, routing, DNS and access policy. Direct appliance VPN connections provide another option for benches at remote sites. Access remains within the approved local networks and VPN routes, without requiring a public NILO endpoint or a cloud relay.

### Connecting an Agent

The **Connect an agent** flow in NILO's GUI lets the user select a client, choose a reachable endpoint, assign an access profile and generate a token. It provides the client's supported setup or import instructions, connection status and access-revocation controls.

Agents connect directly to NILO. A separate NILO desktop application is not required for clients that support the selected MCP connection. A remotely hosted agent connects from its own execution environment through the appropriate private network route.

NILO provides these connection modes:

| Interface | Connection |
| --- | --- |
| Browser GUI and administration | HTTPS for login, configuration, token management and human approvals. |
| MCP over HTTPS | Encrypted access using a certificate trusted by the client and a matching hostname. |
| MCP over HTTP on an approved LAN | An explicitly enabled MCP-only endpoint with token authentication, accessible locally or through the network's VPN gateway. |
| MCP over HTTP on a direct VPN interface | An explicitly enabled endpoint restricted to the selected VPN interface. |

HTTP MCP requires no certificate installation in the agent. Its traffic is unencrypted on ordinary LAN segments. A VPN protects the traffic between its tunnel endpoints; when the VPN ends at a router, the router-to-NILO segment follows the LAN's HTTP/HTTPS policy. The network administrator selects the appropriate mode. The browser GUI retains HTTPS independently of the MCP mode.

## Network-Connected Equipment

NILO connects to equipment that communicates over TCP/IP, including oscilloscopes, power supplies, cameras, embedded boards, controllers, development systems and other network-enabled devices.

Equipment can share an approved network with NILO or occupy a separate equipment network connected through an additional Ethernet interface or USB-to-Ethernet adapter. NILO manages the interface addresses, routes and permitted connections for that network.

```text
User / agent ── Management LAN or VPN ── NILO
                                        │
                               Equipment Ethernet network
                                        │
                       Instruments, boards and other devices
```

NILO provides two complementary forms of access:

- **Plugin access:** People and agents use NILO's GUI or MCP tools. The integration communicates with the equipment, so the user's computer needs access only to NILO.
- **Direct network access:** Authorized computers reach selected equipment addresses and ports through NILO's routing and forwarding facilities. This supports vendor applications, device web interfaces, TCP/IP debugging and other tools that communicate directly with the equipment.

Routing and, where appropriate, address translation allow access to equipment with fixed addresses or limited gateway configuration. Access rules restrict forwarding to the intended devices and services. The equipment network remains separate from the appliance's management access unless an explicit configuration permits communication between them.

## Access and Operation Control

NILO manages human accounts, roles and scoped agent tokens. Users grant access to inspect the bench, operate equipment or administer integrations according to their responsibilities. Tokens can be named, limited, expired, replaced and revoked from the GUI.

The Bench Core enforces permissions and equipment policies for both human and agent requests. Operations follow the configured approval mode, operating limits and device ownership rules. Actions requiring human approval remain pending until an authorized person approves them.

Plugins enforce relevant limits such as voltage, current, duration, target identity, firmware selection and allowed signal connections. Switching equipment has defined safe states, and physical interlocks are supported where the hardware provides them. NILO records operational activity and makes failures and recovery actions visible to the user.

## Human Configuration GUI

NILO's browser interface makes the appliance usable by an electronics professional without routine Linux administration. It provides:

- appliance discovery, identity and initial setup;
- Ethernet, Wi-Fi, VPN profiles, network status and connection diagnostics;
- management and equipment-network configuration, routes and access rules;
- agent setup, endpoint selection, access profiles and token management;
- instrument discovery, plugin installation, device configuration and health status;
- workspaces for measurements, waveforms, cameras, serial communication and other plugin capabilities;
- relay and signal maps, operating limits and approval controls;
- result browsing, logs, activity history, updates and backup/restore.

The interface presents device capabilities and connection problems in terms that help the user operate the bench. Configuration changes pass through the Bench Core for validation and controlled application.

## Installation and Maintenance

NILO is distributed as an appliance image for Raspberry Pi and as an installer bundle for supported Linux hosts. Packages include the Bench Core, browser GUI, service configuration and the supported integration dependencies.

The setup flow establishes the appliance identity, network connections, administrator access and equipment configuration. Each appliance has its own credentials and private keys. Installations support offline package delivery as well as the configured update source.

Updates use versioned, integrity-checked packages with visible progress and a recovery or rollback path. Backup, restore and diagnostic tools support maintenance of the appliance and its bench configuration through the product's administration interfaces.
