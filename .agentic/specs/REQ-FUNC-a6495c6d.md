<!--
Copyright (c) 2026 DDP Engineering
This software is licensed under the PolyForm Noncommercial License 1.0.0.
Commercial use requires a separate commercial license.
See the LICENSE.txt in the .agentic folder for full license details.
SPDX-License-Identifier: LicenseRef-PolyForm-Noncommercial-1.0.0
-->

# REQ-FUNC-a6495c6d — Track every network management change as a confirmable operation

This spec carries the contracts the whole Network service is built against —
the HTTP family, the operation lifecycle, the privileged helper protocol, the
statistics schema, the command-line surface and the MCP tools — so they are
written once here rather than restated in each requirement.

## Behavior

### Operation lifecycle

Every network change is an operation. A read is never an operation.

A change is submitted in two steps. The caller first requests a **preview** of
the change it intends. The service validates the target, computes the effect
and returns a preview carrying an `operation_id`, the resolved target, the
action, and a `revision` — the SHA-256 digest, hex-encoded, of the canonical
preview JSON defined below. Nothing has changed at this point.

The caller then **acknowledges** that preview by returning the `operation_id`
and the `revision` it was shown. The service refuses the acknowledgement when
the operation is unknown, when the revision does not match the digest of a
freshly recomputed preview, when the operation has already left `previewed`,
or when the acknowledging actor is not the actor the preview was issued to and
still holding the permission the action needs.

An acknowledged operation runs. States, in order:

```
previewed -> applying -> awaiting_confirmation -> confirmed
          |           \-> failed
          |
          \-> (non-connectivity actions) applying -> confirmed

awaiting_confirmation -> rolling_back -> rolled_back
                                      \-> rollback_failed
```

There is no separate `acknowledged` state. Acknowledgement is the act that moves
an operation from `previewed` to `applying`, and a state between the two would
only ever be observed by the goroutine performing the transition.

`awaiting_confirmation` is entered only by a connectivity-changing action:
interface enable, interface disable, VPN activation, VPN deactivation and VPN
switching. WireGuard installation, profile import, profile removal and
autostart selection go straight from `applying` to `confirmed`, because none
of them changes the caller's path to the appliance.

`awaiting_confirmation` lasts 120 seconds. An explicit, authenticated
confirmation moves it to `confirmed`. Anything else — the window expiring, an
explicit rollback request, the service dying — moves it to `rolling_back`.
Reaching `rolled_back` requires the restored state to be observed, not merely
requested; when the observation disagrees, the terminal state is
`rollback_failed` and the operation is reported prominently rather than
quietly.

At most one operation may be in `applying` or `awaiting_confirmation` at a
time, across all callers. A submission arriving while one is in flight is
refused with `network_change_in_progress`.

### Canonical preview JSON

The `revision` digest is taken over this exact encoding, so a preview shown in
the browser and one recomputed at acknowledgement time agree byte for byte:

```json
{
  "schema": "nilo.network_preview/1",
  "action": "<action name>",
  "target": "<stable target identifier>",
  "observed": {}
}
```

`observed` holds the state the preview was computed from — the interface's
current connection, or the currently active tunnel — which is what makes a
preview go stale when the world moves underneath it. It is a string map, so Go's
sorted-key marshalling is already canonical and no bespoke canonicalizer is
needed; typed parameters are included as a `parameters_revision` digest so private
configuration is never exposed. The `policy` package's number and duplicate-key rules do not apply
because no number or nested object appears here.

The operation identity and creation time are deliberately **not** in the digest.
The revision answers "which change, against which world", not "which request",
and excluding them buys two things: re-previewing the same proposal yields the
same revision, so a caller can tell the same change from a different one; and
acknowledgement can recompute the digest from the current world and compare it
directly, which is the whole staleness check. Binding to the request is not
weakened, because an acknowledgement must still carry the operation identity,
come from the actor the preview was issued to, and find that operation still in
`previewed`.

## Data / interface

### HTTP

All routes are under `/api/v1/network` and declare their permission in the
single route table in `internal/httpapi/server.go`.

| Method and path | Permission |
|---|---|
| `GET /api/v1/network/status` | `network.read` |
| `GET /api/v1/network/statistics` | `network.read` |
| `GET /api/v1/network/wireguard` | `network.read` |
| `GET /api/v1/network/wireguard/profiles` | `network.read` |
| `POST /api/v1/network/interfaces/{id}/preview` | `network.manage` |
| `POST /api/v1/network/wireguard/preview` | `vpn.manage` |
| `POST /api/v1/network/wireguard/install/preview` | `vpn.install` |
| `POST /api/v1/network/operations/{id}/acknowledge` | action's permission |
| `GET /api/v1/network/operations/{id}` | `network.read` |
| `POST /api/v1/network/operations/{id}/confirm` | action's permission |
| `POST /api/v1/network/operations/{id}/rollback` | action's permission |

`GET /api/v1/network/statistics` takes `interface` (stable identifier) and
`period` (`15m`, `1h`, `24h`). Every response carries `api_version`
(`nilo.http/1`) and a `measured_by` field naming `simulation` or the backend
host, so no consumer has to infer where a number came from.

Acknowledge, confirm and rollback are state-changing and therefore carry the
existing CSRF requirement for session credentials. Their bodies are
`{"revision": "<hex>"}` for acknowledge and `{}` for the other two.

Errors reuse the existing documented-code convention. Codes added here:
`network_not_controllable`, `network_change_in_progress`,
`network_recovery_unavailable`, `network_preview_stale`,
`network_already_acknowledged`, `wireguard_unsupported`,
`wireguard_profile_rejected`, `wireguard_profile_active`,
`wireguard_externally_managed`.

### Privileged helper

One binary, `nilo-nethelper`, running as root, installed explicitly by
repository tooling under `02_Tools`. It listens on
`/run/nilo/nethelper.sock`, mode `0660`, owned `root:nilo`. Every connection
is authenticated with `SO_PEERCRED`: the peer's UID must be the configured
Bench Core service account, otherwise the connection is closed before a byte
is read.

The wire format is one JSON object per connection, answered by one JSON
object. There is no shell, no path argument and no command name anywhere in
it. The `op` field must match one of the compiled-in operations:

```
interface.list       change.inspect        checkpoint.create
change.apply         checkpoint.rollback   checkpoint.destroy
recovery.read        recovery.reconcile
wireguard.check
wireguard.install.inspect   wireguard.install.start   wireguard.install.read
```

The implemented interface increment accepts `op`, an operation `id`, and a typed
`change` only for inspection and preparation. Only interface enable/disable are
implemented for connectivity mutation; `wireguard.check` is a parameterless readiness read.
It rejects a change or operation ID, reports the required `wg` tool explicitly
even when absent, and keeps unknown probe results distinct from known negatives.
`readiness_only: true` means profile inventory and management are unavailable;
older/full replies omit it. Consumers must not infer inactive tunnels from this
empty profile array. Unknown kernel evidence makes `reported: false`; details
contain fixed diagnostics, never raw command output. Only `/usr/bin/wg --version`
can execute on this read path, with fixed environment, a two-second deadline and 4 KiB output cap.
The socket accepts
one bounded JSON frame, rejects unknown fields/operations and authenticates the
configured non-root service UID before reading. There is no remotely callable
recovery-clear operation. Complete network audit history remains unimplemented.

Installation inspection is parameterless. Start accepts only an operation ID and
the inspected `revision`; read accepts only the ID. Requests cannot select
packages, repositories, executables, environment or file paths. `install.json`
persists the last helper-owned job. Start returns promptly in `applying`; core
maintenance polls fixed progress phases and the terminal `confirmed`/`failed`
result. The HTTP operation envelope adds optional `progress`; older replies may
omit it. Installation reserves the same mutation slot as connectivity recovery,
without a confirmation countdown. A lost helper response stays unknown until
polled; a helper restart marks interrupted work failed without retrying it.

Debian 13 and Ubuntu 24.04 installation uses fixed APT arguments for only
`wireguard-tools`, fresh signed repositories, no recommendations/removals/upgrades,
candidate version pinning, and a pre-dpkg version-2 transaction guard that rejects
changes to other packages. The helper's fixed startup configuration suppresses
site hooks and trust overrides. Final success requires a fresh readiness probe
with a verified installed `wg` version. No WireGuard keys or profiles are read.

Recovery state lives in `/var/lib/nilo-nethelper/recovery.json`, mode `0600` in a
root-only directory separate from Bench Core's writable data directory. Atomic
write/fsync precedes connectivity changes. The record retains pending or terminal
phase, operation ID, deadline, previous saved connection UUID, device autoconnect
and a normalized digest of observed addresses/routes/DNS; it contains no secrets.
Terminal records also retain previously disabled connection choices. On startup,
pending records are reconciled before the helper listens; Bench Core also requests
reconciliation before serving the appliance API. Successful restoration is verified
against live observations. A lost checkpoint falls back to the OS-owned saved
connection and refuses to claim success when its observed state no longer matches.
Reboot and changed-DHCP acceptance remain outstanding.

The appliance listener requires explicit Linux profile selection, an unprivileged
service user, built GUI assets and a certificate/key. Private addresses require TLS;
public and wildcard addresses are refused. The GUI receives the selected profile
as non-secret HTML metadata before login; authenticated status remains authoritative.

### Statistics storage

One new migration adds:

```sql
CREATE TABLE network_traffic_minute (
    interface_id  TEXT    NOT NULL,
    minute_start  INTEGER NOT NULL,  -- Unix seconds, UTC, minute-aligned
    rx_bytes      INTEGER NOT NULL,
    tx_bytes      INTEGER NOT NULL,
    rx_errors     INTEGER NOT NULL,
    tx_errors     INTEGER NOT NULL,
    rx_dropped    INTEGER NOT NULL,
    tx_dropped    INTEGER NOT NULL,
    samples       INTEGER NOT NULL,  -- samples that composed this minute
    measured_by   TEXT    NOT NULL,  -- 'simulation' or the host identity
    PRIMARY KEY (interface_id, minute_start)
) STRICT;
```

A minute with no usable sample is not written. A reader therefore sees a gap
as a missing row, never as a zero row, and `samples` tells it how complete a
present row is. Retention deletes rows whose `minute_start` is older than 24
hours, on write.

Rates are computed from `time.Since` over a monotonic reading, never from a
difference of wall-clock timestamps, so a clock adjustment cannot manufacture
traffic. A counter that decreases means the device reset: the sampler starts a
new interval and writes no row for the straddled minute.

### Command line

```text
nilo network status
nilo network statistics --interface ID --period 15m|1h|24h
nilo network interface enable ID
nilo network interface disable ID
nilo network vpn check
nilo network vpn install
nilo network vpn profiles list
nilo network vpn profiles import --file PATH --name NAME
nilo network vpn profiles remove ID
nilo network vpn enable ID
nilo network vpn disable
nilo network vpn autostart ID --enabled true|false
nilo network operation status ID
nilo network operation confirm ID
nilo network operation rollback ID
```

Global options: `--server URL`, `--token-file PATH`, `--ca-file PATH`,
`--json`. There is no `--token` flag and no environment variable carrying a
token value; the token is read from the file and nothing else.

A mutation prompts on a terminal and refuses on a pipe unless
`--acknowledge` is given. `--acknowledge` supplies only the acknowledgement
step: it never confirms, so a connectivity change still enters its 120-second
window and still needs `nilo network operation confirm ID`.

Exit codes: `0` success, `1` the server refused or the change failed, `2`
usage error.

### MCP

`/mcp`, Streamable HTTP, official Go SDK pinned in `go.mod`. Three tools,
`network_get_status`, `network_get_statistics`, `network_get_wireguard_status`,
each reaching only read methods of the shared service. Authorization is the
existing bearer resolution requiring `network.read`.

## Examples

A disable that is never confirmed:

1. `POST /api/v1/network/interfaces/enp1s0/preview {"action":"disable"}` →
   `200 {"operation_id":"…","action":"interface.disable","target":"enp1s0","revision":"9f2c…","state":"previewed"}`
2. `POST /api/v1/network/operations/…/acknowledge {"revision":"9f2c…"}` →
   `202 {"state":"applying"}`
3. helper writes `recovery.json`, creates an NM checkpoint with a 120-second
   rollback timeout, disconnects `enp1s0`
4. `GET /api/v1/network/operations/…` → `{"state":"awaiting_confirmation","confirm_deadline":"…"}`
5. nobody confirms; NetworkManager rolls the checkpoint back at 120 seconds;
   the helper observes `enp1s0` on its previous connection, clears
   `recovery.json`
6. `GET /api/v1/network/operations/…` → `{"state":"rolled_back"}`

Replaying step 2 after step 3 returns `409 network_already_acknowledged` and
runs nothing.

A stale preview: preview a VPN switch while `office` is active, activate
`backup` by another route, then acknowledge the first preview — the recomputed
`observed.active_tunnel` is now `backup`, the digest differs, and the
acknowledgement is refused with `network_preview_stale`.

## Acceptance

- A preview changes nothing and its `revision` is reproducible from the
  returned fields alone.
- An acknowledgement is accepted exactly once, only for the revision shown,
  only from the actor it was issued to, and only while that actor still holds
  the action's permission.
- Two connectivity changes cannot be in flight at once, and an operation whose
  checkpoint cannot be created is refused before connectivity is touched.
- An unconfirmed connectivity change ends in `rolled_back` with the previous
  connection, routes and resolver observed to match the captured state, or in
  `rollback_failed` when they do not.
- A helper started with `recovery.json` present restores before answering any
  other operation.
- No response, log line, audit record or MCP result on any path above contains
  a private key, a preshared key or raw profile text.
