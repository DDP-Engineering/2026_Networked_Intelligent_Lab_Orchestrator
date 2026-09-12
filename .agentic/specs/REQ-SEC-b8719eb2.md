<!--
Copyright (c) 2026 DDP Engineering
This software is licensed under the PolyForm Noncommercial License 1.0.0.
Commercial use requires a separate commercial license.
See the LICENSE.txt in the .agentic folder for full license details.
SPDX-License-Identifier: LicenseRef-PolyForm-Noncommercial-1.0.0
-->

# REQ-SEC-b8719eb2 — Exclusive physical adapter roles

## Behavior

A physical adapter has one stable identity and may hold one exclusive role:
`management` or `device`. Ethernet, Wi-Fi and USB Ethernet adapters participate.
Loopback and virtual interfaces, including a NILO-owned WireGuard interface,
do not acquire a physical-adapter role; they remain valid management or ingress
paths under their existing contracts.

The device role is claimed by every saved device network's `device_interface`,
regardless of whether that network is disabled, enabled, waiting, conflicting,
or temporarily absent. A create, update, or enable operation also claims every
device-side identity that could become durable or be restored until the
operation reaches a terminal state. A device network's `local_interface` and
WireGuard access profile are ingress paths and do not claim the device role.

The management role is claimed by the active management configuration. While a
management change awaits confirmation, both its proposed configuration and its
last-known-good rollback configuration claim their physical adapters. This
prevents a device assignment from making rollback unsafe.

Before either mutation path writes intent or contacts a firewall, forwarder,
listener, NetworkManager, or privileged helper, one process-local coordinator
resolves stable identities, evaluates durable claims and pending reservations,
and reserves the requested identities. Conflicting concurrent requests cannot
both pass. A reservation is released when an operation fails or is cancelled;
durable state becomes the claim when it is saved. The coordinator contains no
network-administration capability and never sends a raw command to the helper.

For device-network operations, the first durable applying record contains the
pre-change inventory, the complete old/proposed claim union and, for creates, a
deterministic created-record ID. This record precedes helper preparation and
mutation. After restart, an operation without a helper checkpoint is settled
from that metadata: an exact durable create is confirmed, an absent create is
failed, and other pre-helper attempts fail after restoring the saved inventory.
If helper preparation succeeds but persisting its checkpoint fails, the helper
checkpoint is restored with a fresh bounded context before the operation is
settled.

Management requests continue to accept Linux interface names at the API and
console boundary. For a present physical adapter, Bench Core resolves the name
to `Interface.AdapterID` before validation. A physical binding stores that
adapter ID and its last-observed name. A temporarily absent binding keeps both
values and its claim. A present adapter with the same ID and a different name
updates the observed name and status without changing its role. Virtual
management interfaces store an empty adapter ID and their interface name.

On startup, the management and device-network stores are loaded before the
management firewall and HTTPS listener are activated. Legacy name-only
management entries are resolved to stable IDs whenever the named adapter is
present. An unresolved legacy entry stays name-bound until the adapter appears;
it is never used to admit a newly observed address until role reconciliation
has resolved and validated it.

If a persisted conflict exists and a non-conflicting usable management path
remains, device-facing intent wins. Reconciliation removes the conflicting
physical binding from both the active and last-known-good management
configurations, saves the result as `recovered`, applies the resulting
management firewall, and only then permits the destination-address gate and
listener to use it. Recovery detail identifies the adapter and device network
without addresses or secrets.

If every usable management path conflicts, startup fails before opening the
HTTPS listener or accepting application traffic. It neither installs device
forwarding nor widens management firewall access. The console error identifies
the adapter and roles and instructs the administrator to establish a separate
management path locally. A later startup with a non-conflicting path performs
the ordinary recovered flow.

Maintenance reconciliation uses the same ordering before adding an address for
a returned or renamed interface to the live destination gate. Pending,
conflict, failed, and recovered outcomes remain observable until superseded by
a later persisted state.

## Data and interface

The internal management selection is:

```text
ManagementInterfaceBinding {
  adapter_id: string       // stable physical identity; empty for virtual/legacy-unresolved
  last_known_name: string  // validated Linux interface name
}
```

`ManagementAccessConfiguration` owns a canonical, duplicate-free list of these
bindings. Physical duplicates compare case-insensitively by `adapter_id`;
virtual or unresolved bindings compare by `last_known_name`. The SQLite
management-access singleton stores the active and last-known-good binding lists
as JSON. Migration of the existing name arrays is lossless and creates
unresolved bindings; startup resolution fills stable IDs before the listener is
opened when the adapters are present.

Safe role metadata uses the fixed values `management` and `device`. Adapter
inventories consumed by the two selectors expose:

```text
adapter_id: string
role: "" | "management" | "device"
role_available: boolean
role_detail: string
conflicting_resource_id: string
conflicting_resource_name: string
```

`role_available` is relative to the selector that returned the entry. The
resource fields identify a saved device network when one owns the device claim;
they are empty for a management claim. Status and detail fields never include
addresses, SSIDs, passwords, tokens, or WireGuard key material.

Both update endpoints use HTTP `409 Conflict` for a role collision and preserve
the existing error envelope while adding a `details` object:

```json
{
  "error": "network_interface_role_conflict",
  "message": "adapter usb-ethernet-1 is already assigned to the device role",
  "details": {
    "adapter_id": "stable-adapter-id",
    "interface": "usb-ethernet-1",
    "existing_role": "device",
    "requested_role": "management",
    "conflicting_resource_id": "device-network-id",
    "conflicting_resource_name": "PLC network"
  }
}
```

The opposite direction swaps `existing_role` and `requested_role`; resource
fields are empty if no saved resource owns the management claim. Unknown role
values are rejected by internal and frontend parsers rather than displayed as
trusted state.

## Examples

- `wlan0` / stable adapter `wifi-a` is management and `eth0` / stable adapter
  `ethernet-a` is device-facing: accepted.
- `wlan0` and NILO-owned `nilo0` are both management while USB Ethernet
  `usb-a` is device-facing: accepted; `nilo0` has no physical role claim.
- A disabled saved device network references `usb-a`; after replug Linux calls
  it `enx1234`. Selecting `enx1234` for management returns the same conflict.
- A pending management configuration adds `usb-a`. A concurrent device-network
  enable on `usb-a` loses the reservation race and changes neither store nor
  helper state.
- Legacy management bindings contain `eth0`, `wlan0`, and `nilo0`, while a
  saved device network owns stable adapter `ethernet-a` resolved as `eth0`.
  With usable `wlan0` or `nilo0`, startup removes `eth0`, reports `recovered`,
  and serves only through the retained paths.
- If `eth0` is the only usable management path in that legacy conflict,
  startup reports `network_interface_role_conflict` and opens no HTTPS
  listener.

## Browser workflow

Each selector keeps conflicting adapters visible but disabled and associates a
persistent localized reason with the disabled control or option. The reason
names the existing role and the saved device network when applicable. The
device-side selector applies this rule only to `device_interface`; a management
adapter remains a valid `local_interface` ingress choice under the existing
device-network contract.

Repurposing is deliberately sequential and offers no one-click role swap:

1. add a different management path and confirm it through that path;
2. remove the old adapter from management access and confirm the change;
3. select the now-unclaimed adapter as the device-facing interface.

Current-session and last-management-route warnings remain persistent. Conflict
notices raised during preview, acknowledgement, or management update, plus
pending, failed, and recovered outcomes and their safe detail, remain visible
with all dialogs and help closed until refreshed state supersedes them.

## Acceptance

- Focused Go tests prove both conflict directions, absent and renamed adapters,
  last-known-good claims, startup recovery/failure, and zero downstream calls
  on refusal.
- Deterministic concurrency tests prove at most one opposite-role reservation
  succeeds and that terminal outcomes release it.
- SQLite tests prove lossless name migration, stable-ID persistence, restart,
  and secret-free serialization.
- HTTP and frontend contract tests prove the exact conflict envelope and reject
  unsupported role values.
- GUI component and browser tests prove disabled choices, associated localized
  reasons, sequential repurposing, persistent outcome states, authorization,
  supported viewport behavior, and absence of secrets.
- The isolated Linux network test proves a rejected configuration invokes no
  management or forwarding rules and that recovered startup admits the
  management port only on retained interfaces.
- A separately labelled Raspberry Pi early-signal run keeps DDP/WireGuard as
  management, identifies the USB Ethernet adapter by stable hardware identity,
  exercises accepted and rejected role assignments, replug/restart behavior,
  and confirms the management URL remains reachable. This evidence does not
  replace the reproducible release acceptance environment.

## Assumptions and open decisions

The approved recovery precedence is that a persisted device-facing claim wins
over a conflicting management binding. The DDP hotspot and WireGuard URL are
the management/recovery path for the personal Raspberry Pi run, while USB
Ethernet is connected only to an isolated device-facing segment. Exact adapter
IDs, Linux names, link carrier, and client certificate trust are observations
to record during that run, not product constants. There are no open product
decisions.
