<!--
Copyright (c) 2026 DDP Engineering
This software is licensed under the PolyForm Noncommercial License 1.0.0.
Commercial use requires a separate commercial license.
See the LICENSE.txt in the .agentic folder for full license details.
SPDX-License-Identifier: LicenseRef-PolyForm-Noncommercial-1.0.0
-->

# REQ-FUNC-2f8d63b5 - Device networks and appliance audit

This spec carries the contracts the Device networks and audit extension is
built against: the domain vocabulary, the persistence entities, the audit event
envelope, the diagnostic scopes, the permission additions and the HTTP surface.
It extends `REQ-FUNC-a6495c6d.md`, which remains the authority for the
operation lifecycle, the preview digest, the helper wire protocol, the
statistics schema and the MCP tools. Where the two overlap, the older spec wins
and this one says only what it adds.

The source is `00_Planning/PLAN_DEVICE_NETWORKS_AND_AUDIT.md`. That document is
a plan; this one is the contract the code is written to.

## Vocabulary

The product term is **Device networks**. The feature is never labelled Cabinet,
PLC network or PLC gateway: a PLC is one acceptance example of a device, not the
domain model.

| Term | Meaning |
|---|---|
| Device-side interface | The appliance adapter facing the devices |
| Device subnet | The IPv4 subnet the devices live on |
| Target IP address | One device address inside the device subnet |
| Exposed address / subnet | The address or prefix a remote client actually uses |
| Location | An optional operator label; never an identifier |
| Access method | `local`, `wireguard`, or both, each explicitly selected |
| Addressing | `original` or `translated` |

## Design defaults recorded at reconciliation

These were proposed by the plan rather than previously implemented, and are
adopted here. They are recorded as RULING-006 in the task journal.

1. One NILO-managed WireGuard tunnel is active at a time; zero or one profile
   is selected for startup. External tunnels stay observable and read-only.
2. Device forwarding is IPv4 only in this milestone. IPv6 is observed and
   preserved on management and uplink paths, and is denied for the exposed
   scope so it cannot bypass the IPv4 access policy.
3. Access modes are `local`, `wireguard` and both. Both is the union of two
   explicitly configured paths, never an automatic fallback.
4. Source NAT to the appliance's device-side address is the default. Source
   preservation is an advanced setting that documents its return-route
   dependency.
5. Retention targets: audit 90 days, operational events 14 days, debug 24
   hours, each under its own size budget rather than an unlimited promise.
6. Debug sessions are timed at 5, 15 or 60 minutes, with 60 the maximum. There
   is no permanent global debug switch.
7. Three new fixed permissions are added: `activity.read`, `activity.export`
   and `diagnostics.manage`, granted to the administrator role only.

## Appliance identity

A NILO appliance has a stable random identifier generated once, at first
initialization of its data directory, and never afterwards. It is stored in the
appliance database. Cloning a commissioned appliance therefore duplicates the
identity, so the operations guide must require re-provisioning; the software
detects nothing about a clone and must not claim to.

Each start of the process generates a separate **boot identifier**. Name and
location are mutable labels, and every event carries the label as it read at
event time as well as the stable identity, so a rename does not rewrite history
and correlation still works.

The identity is generated before it is used as the identity of any event. A
service that cannot read or create it does not start.

## Audit event envelope

`audit_events` is extended additively. Existing rows remain readable with their
original meaning: every new column is nullable or carries a default that reads
as "not recorded by the writer of this row", never as an asserted value.

| Field | Meaning |
|---|---|
| `id` | Existing event UUID |
| `sequence` | Existing monotonic ordering, now allocated from a persisted counter |
| `actor_id` | Existing actor UUID |
| `action` | Existing stable action code |
| `subject_type`, `subject_id` | Existing target type and identity |
| `payload_json` | Existing safe structured details |
| `created_at` | Existing UTC timestamp |
| `envelope_version` | Integer; `1` for rows written before this milestone |
| `appliance_id` | The appliance that recorded the event |
| `boot_id` | The boot during which it was recorded |
| `category` | `audit`, `event` or `debug` |
| `component` | Function scope, for example `network.device_networks` |
| `actor_kind` | `user`, `token`, `system` or `anonymous` |
| `outcome` | `requested`, `applied`, `confirmed`, `recovered`, `failed` or `denied` |
| `severity` | `info`, `warning` or `error` |
| `operation_id` | Correlation to a network operation, when one is known |
| `target_label` | The event-time human label of the target, for reading |
| `time_source` | `synchronized` or `unsynchronized` at write time |

Ordering is the persisted `sequence`, not `created_at`: a wall-clock adjustment
must not reorder history. The sequence allocator is a single-row table, so a
retention sweep that deletes every row cannot make the next event reuse a number.

The existing append-only repository stays append-only. It is not tamper-proof
against a host administrator and must never be described as forensic evidence;
remote independent storage is a later feature.

### Actors that are not people

A system action carries `actor_kind = "system"` and the fixed system actor
identity. A failed unauthenticated request carries `actor_kind = "anonymous"`
and the fixed anonymous actor identity. Neither is ever rendered as a person,
and an IP address is never turned into an identity.

### What never enters an event

Passwords, private keys, preshared keys, authentication headers, cookies,
tokens, raw request bodies and credential-bearing subprocess output are
excluded at every level, including debug. Configuration events carry safe
before and after fields, not whole imported files. Untrusted names and error
strings are sanitized, and CSV export neutralizes a leading `=`, `+`, `-` or
`@`.

## Diagnostic scopes

Scopes are product functions, not code functions:

```
network            network.interfaces   network.wifi
network.wireguard  network.device_networks
network.recovery   identity             system
```

Selecting `network` covers its subfunctions. An optional target identifier
narrows a scope to one interface, profile or device network. One effective
policy exists per scope and target; the narrowest match wins, and two sessions
covering the same scope both expire on their own deadlines.

Expiry is measured on monotonic time within a boot, so a clock change cannot
extend a session, and no session survives a restart: an interrupted session is
recorded as interrupted and the scope returns to normal.

## Persistence

New tables are added by new migrations. Migrations `001` through `005` are
applied and are never edited; their digests are checked at startup.

| Table | Content |
|---|---|
| `appliance_identity` | Single row: stable ID, name, location, commissioned-at |
| `audit_sequence` | Single row: the next sequence number to hand out |
| `network_operations` | Operation ID, actor, action, target, safe parameters, revision, state, deadline, helper reference, failure, timestamps |
| `network_interface_settings` | Adapter identity, NILO connection UUID, IPv4 method and values, DNS, Wi-Fi metadata, secret reference, revision |
| `vpn_profiles` | Stable ID, name, provider, public configuration, helper and NM references, revision, startup selection |
| `device_networks` | Stable ID, name, location, interface reference, appliance address and prefix, scope, target, addressing, exposed address, access method, permitted sources, service policy, startup intent, revision |
| `diagnostic_sessions` | Scope, target, actor, started-at, expires-at, stop reason |
| `retention_policy` | Per-category age and size budgets and the observed retained watermark |

No secret value is stored in any of them. A secret is held in root-owned helper
storage and referenced by an opaque identifier.

Observed state is persisted only with the time it was observed, and is never
restored after a restart as current truth.

## Permissions

Existing: `network.read`, `network.manage`, `vpn.manage`, `vpn.install`.

Added: `activity.read`, `activity.export`, `diagnostics.manage`, all three on
the administrator role only. Account and security events are not disclosed to
viewer or operator through a network permission. A token remains the
intersection of its owner's role and its scopes, re-evaluated on every request.

## HTTP surface

All paths are under `/api/v1`. Existing routes and DTOs keep their meanings;
this milestone adds discriminated parameters rather than changing them.

| Method and path | Behavior |
|---|---|
| `GET /network/operations` | Recent and pending operations, cursor-paginated, for reload and restart reattachment |
| `POST /network/interfaces/{id}/settings/preview` | Preview an IPv4 method, address, gateway, DNS or ownership change |
| `GET /network/interfaces/{id}/wifi` | Wi-Fi capability and scan observations |
| `POST /network/interfaces/{id}/wifi/preview` | Preview a join; the password never returns |
| `GET /network/device-networks` | List saved device networks and their observed state |
| `GET /network/device-networks/{id}` | Read one saved device network |
| `POST /network/device-networks/preview` | Preview create, update, enable, disable or delete |
| `POST /network/device-networks/{id}/checks` | Bounded connection check against the configured target only |
| `GET /network/device-networks/{id}/access-instructions` | Machine-specific instructions and unmet prerequisites |
| `GET /activity` | Filtered, cursor-paginated history |
| `GET /activity/export` | Bounded streamed JSONL or CSV export |
| `GET /diagnostics/sessions` | List active and recent debug sessions |
| `POST /diagnostics/sessions` | Start a timed session |
| `DELETE /diagnostics/sessions/{id}` | Stop a session |
| `GET /diagnostics/settings` | Read retention, quota and logging policy |
| `POST /diagnostics/settings/preview` | Preview a policy change |

Every mutation goes through the existing preview and acknowledgement lifecycle;
none of the new routes mutates directly. Permissions are revalidated on
acknowledge, confirm and rollback, not only at preview.

Bounds are part of the contract: activity page size 200, export 50 MiB, probe
timeout 10 seconds, debug duration 60 minutes, import 64 KiB per file and 16
files per request. Deadlines are UTC. Pagination is by opaque cursor.

## Acceptance mapping

Plan section 13's identifiers are plan-local. Each maps here to the requirement
that governs it. `+` marks a requirement this milestone extends rather than
creates.

| Plan ID | Requirement |
|---|---|
| DN-01 | REQ-FUNC-3a71c204 |
| DN-02 | REQ-FUNC-5e2b91d7, REQ-FUNC-7c40aa18 |
| DN-03 | REQ-REL-4c07b1ea |
| DN-04 | REQ-FUNC-2f8d63b5, REQ-FUNC-9b17ce40, REQ-DATA-7b62c9f3 |
| DN-05 | REQ-FUNC-64c8f2a9 |
| DN-06 | REQ-FUNC-1d93b76e |
| DN-07 | REQ-FUNC-8e5a02cf |
| DN-08 | REQ-FUNC-46b1d8e3 |
| DN-09 | REQ-SEC-71bd3e94 |
| DN-10 | REQ-SEC-3f8c90a2 |
| DN-11 | REQ-SEC-a05f2c1b |
| DN-12 | REQ-PLAT-6d21f0b8 |
| DN-13 | REQ-REL-8fa3d260, REQ-REL-2e91cb74, REQ-REL-67b09885 + |
| DN-14 | REQ-REL-b6d0e358 |
| WG-01 | REQ-FUNC-0f6ac82d, REQ-FUNC-faf2f697 +, REQ-SEC-15dcc526 + |
| WG-02 | REQ-FUNC-a1584be7, REQ-FUNC-9e05d184 +, REQ-FUNC-16e918cb + |
| WG-03 | REQ-FUNC-1a417267 +, REQ-FUNC-090be3ae +, REQ-FUNC-72e0dc95 |
| WG-04 | REQ-FUNC-e8b3407c |
| AU-01 | REQ-DATA-91e7ab46, REQ-REL-2e91cb74 |
| AU-02 | REQ-REL-59e4f80c |
| AU-03 | REQ-DATA-58f3c012 |
| AU-04 | REQ-DATA-2c8b5f79 |
| AU-05 | Retired: timed debug sessions are no longer offered |
| AU-06 | REQ-SEC-c62481fe |
| AU-07 | REQ-DATA-e4a10d85, REQ-FUNC-6ba25f39 |
| AU-08 | REQ-SEC-d19a7b30 |
| UX-01 | REQ-UX-4e08d371 |
| UX-02 | REQ-UX-b7f24069, REQ-FUNC-97c1e5d8 |
| UX-03 | REQ-UX-0697c0ab +, REQ-NFR-b3f90f07 +, REQ-UX-1c5eab92 |
| IO-01 | REQ-TEST-3d907c14 |
| IO-02 | REQ-TEST-a82f6b51 |
| PL-01 | REQ-PLAT-c3923cad +, REQ-TEST-baa6a1bf + |

## What this milestone does not do

OpenVPN and other VPN implementations; provisioning an external VPN hub;
changing routes on the operator's own computer or router; DHCP server; Wi-Fi
access-point mode; captive portals; enterprise Wi-Fi; Ethernet bridging;
Layer-2 VPN; a general firewall editor; arbitrary nftables rules; public port
publication; multiple active managed tunnels; overlapping device subnets on one
appliance; IPv6 device forwarding; built-in Modbus polling; a vendor
application uploader; packet capture; and central fleet log collection.

NILO routes traffic. It cannot certify that a vendor application download
succeeded or identify a software version from a TCP connection, and it reports
network facts only. An operator's maintenance note is labelled as an operator
statement, never as an observation.
