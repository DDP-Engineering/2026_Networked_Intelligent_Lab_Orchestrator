# NILO MCP reference

This reference describes the implemented tools, checked against the repository
on 2026-09-27. NILO runs one shared MCP server in Bench Core. Camera contributes
operations through that server; it does not run a separate MCP process.

## Connection and authentication

| Setting | Value |
|---|---|
| Transport | Streamable HTTP |
| Appliance endpoint | `https://APPLIANCE:8443/mcp` (use the configured management port) |
| Authentication | `Authorization: Bearer TOKEN` on each request |
| TLS | Trust the appliance certificate/CA in the client. |
| Server implementation name | `nilo` |

Configure a client that supports Streamable HTTP, custom authorization headers
and the appliance CA. Client-specific configuration keys differ. There is no
`nilo mcp` stdio command. Browser login cookies do not authenticate this endpoint.
Create an API token using the [authentication guide](../07_Delivery/NILO_CLI_and_Authentication.md).

The transport requires `network.read`, or `bench.read` when Bench tools are
registered. Camera automation normally needs `bench.read` and `bench.execute`;
add `plugins.manage` only for plugin/device administration or retention changes.
All permissions are intersected with the token owner's current role. Tokens
are authenticated on each request/tool call; revocation and role/scope changes
take effect without restarting the MCP client.

Let the client perform normal MCP initialization and protocol negotiation,
then call `tools/list`. A client sending an `Origin` header must use an allowed
origin; native clients can omit it. An invalid, expired or revoked credential
receives HTTP 401; insufficient transport permission receives HTTP 403.

The normal Bench-enabled server exposes **40 tools**: 3 network, 10 plugin,
5 Bench, 4 standalone Camera, 5 Camera workspace/media and 13 USB serial tools. A server
constructed without Bench exposes only the network tools. `tools/list` exposes
registered schemas, not a filtered list of permissions granted to the caller.
Use those schemas as the authoritative input contract for the running build.

## Input and result conventions

In the tables below `?` marks an optional input. IDs and enum values are strings;
revisions, offsets and seconds are integers; `parameters`, `settings`, `controls`
and `limits` are JSON objects. Unknown top-level arguments are rejected.

Bench/plugin/Camera tools generally return a text content item containing
serialized JSON. Parse that text before inspecting fields. Network tools may
also expose structured results. `camera_snapshot` can additionally return an
image content item. Check `isError` before treating a tool result as success;
invalid arguments may also produce a protocol error. Read-only annotations are
client hints and do not grant permission.

Submitting a job is asynchronous: retain `transaction_id` and poll
`bench_get_job`. `pending_approval`, `queued` or `running` is not capture success.
Terminal states include `completed`, `failed`, `cancelled` and `incomplete`.
Queue and human-approval time count toward `timeout_seconds` (default 60,
maximum 300). Cancelling a running job can require later cleanup.

Approval is available only through a human GUI session. Neither an administrator
token nor an MCP tool can approve an operation. Reuse an idempotency key only
for an identical retry; new intended operations need new keys. Keys contain
8–128 ASCII letters, digits, underscores or hyphens.

## Network tools

All require `network.read`.

| Tool | Inputs | Result |
|---|---|---|
| `network_get_status` | `{}` | Network/interface status. |
| `network_get_statistics` | `interface`, `period`: `15m`, `1h` or `24h` | Bounded interface traffic statistics. |
| `network_get_wireguard_status` | `{}` | WireGuard installation/readiness and status. |

Network mutations are available through the GUI/CLI/HTTP interfaces, not these
MCP tools. See [Network operations](Network_Operations.md).

## Plugin and device tools

| Tool | Inputs | Permission / result |
|---|---|---|
| `plugins_list` | `{}` | `bench.read`; plugin array. |
| `plugin_lifecycle` | `plugin_id`, `action`: `install`, `enable`, `disable`, `restart`, `uninstall`; `mode?`: `ask`, `auto` | `plugins.manage`; updated plugin. |
| `plugin_discover` | `plugin_id` | `bench.execute`; discovered device array; requires an enabled plugin. |
| `plugin_devices` | `plugin_id` | `bench.read`; persisted devices for an installed plugin. |
| `plugin_configuration` | `plugin_id` | `plugins.manage`; configuration with revision/settings. |
| `plugin_configure` | `plugin_id`, `revision` ≥ 0, `settings` | `plugins.manage`; updated configuration. |
| `plugin_device_configure` | `plugin_id`, `device_id`, `revision` ≥ 1, `name`, `settings`, `purpose?` | `plugins.manage`; updated device. |
| `plugin_device_action` | `plugin_id`, `device_id`, `revision` ≥ 1, `action`: `enable`, `disable`, `retry` | `plugins.manage`; updated device. |
| `plugin_device_rebind` | `plugin_id`, `device_id`, `revision` ≥ 1, `provisional_id`, `provisional_revision` ≥ 1, `confirm_device_id` | `plugins.manage`; rebound device. |
| `plugin_device_forget` | `plugin_id`, `device_id`, `revision` ≥ 1, `confirm_device_id` | `plugins.manage`; `{"forgotten":"ID"}`. |

Use `plugin_id: "camera"` for Camera or `plugin_id: "serial"` for USB to Serial. Configuration must conform to the installed
plugin/device schema. Read current revisions before writes; refresh on conflict.
Rebind/forget confirmation must exactly match the device record ID. Forgetting
retains history. Installation uses a curated bundle already available on the
appliance. `plugin_lifecycle` currently has **no `upgrade` action**; use the GUI
or `nilo plugin upgrade` for an available update.

## Bench tools

| Tool | Inputs | Permission / result |
|---|---|---|
| `bench_submit` | `plugin_id`, `operation`, `parameters`, `idempotency_key`, `device_id?`, `timeout_seconds?` (1–300) | `bench.execute`; durable job. Camera requires `device_id`. |
| `bench_list_jobs` | `{}` | `bench.read`; up to 100 recent authorized jobs. |
| `bench_get_job` | `transaction_id` | `bench.read`; job, ordered replies, result/error and artifact ID. |
| `bench_cancel` | `transaction_id` | `bench.execute`; job after cancellation request. |
| `bench_read_artifact` | `transaction_id`, `offset` ≥ 0, `length` 1–65536 | `bench.read`; a base64 chunk of a completed transaction artifact. |

Job reads/cancellation enforce ownership; administrators with `plugins.manage`
can access other owners' transactions. Artifact chunks contain `transaction_id`,
`artifact_id`, `size_bytes`, `offset`, `next_offset` and `base64`. Decode and append
each chunk until `next_offset` reaches `size_bytes`. This tool reads transaction
artifacts, not arbitrary files or workspace videos.

## Standalone Camera tools

All require `bench.execute`, and return queued transactions unless the snapshot
finishes during its bounded wait. Stop the live workspace first; otherwise use
workspace actions to avoid competing for the physical camera.

Common inputs: required `device_id` and `parameters`; optional
`idempotency_key` (generated if omitted) and `timeout_seconds` (1–300).

| Tool | `parameters` / extra inputs | Result |
|---|---|---|
| `camera_get_controls` | `{}` | Job; completed result contains hardware controls/capabilities. |
| `camera_set_controls` | `{"controls":{"brightness":0}}`, using actual returned control IDs/values | Job. |
| `camera_reset_controls` | `{"factory":false}` for saved baseline, `true` for hardware defaults | Job. |
| `camera_snapshot` | Capture parameters below; optional top-level `wait_seconds` (0–20) | Transaction status; a completed capture can include a bounded JPEG preview. |

Snapshot parameters include `width`, `height`, `format` (`MJPG` or `YUYV`),
`processing`, optional `controls`, `frame_interval`, `encoding` (`jpeg` or `png`),
`jpeg_quality` (1–95), and `maximum_dimension`. The physical camera must support
the requested capture mode. Never send runner paths or staging directories;
Core supplies them.

Example `tools/call` request after initialization:

```json
{
  "jsonrpc": "2.0", "id": 1, "method": "tools/call",
  "params": {
    "name": "camera_snapshot",
    "arguments": {
      "device_id": "cam-1",
      "parameters": {
        "width": 640, "height": 480, "format": "MJPG",
        "processing": {"zoom": 1, "grayscale": false},
        "encoding": "jpeg", "jpeg_quality": 85
      },
      "idempotency_key": "inspection-snapshot-001",
      "timeout_seconds": 60, "wait_seconds": 10
    }
  }
}
```

For a pending capture the JSON text includes `transaction_id`, `state` and
`poll_tool: "bench_get_job"`; errors can also be reported there. A successful
completed result includes an artifact ID and an image content item. The preview
is a bounded JPEG even when the retained image uses another encoding; retrieve
the artifact for the saved original.

## Camera workspace and media tools

| Tool | Inputs | Permission / result |
|---|---|---|
| `camera_workspace_status` | `device_id` | `bench.read`; state, revision, settings, retention, used bytes and recording progress when active. |
| `camera_workspace_stop` | `device_id`, `action`: `stop` or `record_stop` | `bench.execute`, owner or administrator; `{"stopped":true}`. Does not request new approval. |
| `camera_media_list` | `device_id`, `offset?` ≥ 0 (default 0) | `bench.read`; array of up to 24 authorized captures. |
| `camera_media_delete` | `media_id` | `bench.execute`, owner or administrator; `{"deleted":true}`. Permanent; active media refused. |
| `camera_retention_configure` | `device_id`, `revision` ≥ 0, `limits` | `plugins.manage`, camera stopped; `{"saved":true}`. |

The media tool returns an array directly (the corresponding HTTP/CLI response
wraps it in `items`). Entries include `id`, `camera_id`, `owner_id`, `kind`
(`image` or `video`), `state`, `bytes`, `created_at` and `metadata`. Increment
`offset` by the number returned to fetch the next page.

There is no dedicated MCP live-start, record-start or media-download tool.
Start/configure/capture through `bench_submit` with `operation: "workspace"`:

```json
{
  "jsonrpc": "2.0", "id": 2, "method": "tools/call",
  "params": {
    "name": "bench_submit",
    "arguments": {
      "plugin_id": "camera", "device_id": "cam-1",
      "operation": "workspace",
      "parameters": {"action": "start", "revision": 1},
      "idempotency_key": "inspection-live-001", "timeout_seconds": 60
    }
  }
}
```

Replace revision `1` with the value returned by `camera_workspace_status`.
Wait for completion, then submit `settings`, `controls`, `snapshot` or
`record_start` with the observed revision and a fresh key. Supported actions,
complete settings/overlay examples, limits and hardware controls are documented
in the [CLI workspace reference](CLI_Reference.md#camera-live-workspace-and-recording).
Use `settings.mode: "timelapse"` and `settings.interval` before `record_start`
for time-lapse. Settings are a full replacement and cannot change during recording.

Live images use authenticated `GET /api/v1/cameras/{id}/frame` (JPEG), outside
MCP. An idle preview without frame requests expires after 30 seconds; recording
continues subject to session and retention limits. MCP status polling is not a
live image stream.

Export media through authenticated
`GET /api/v1/camera-media/{id}/content`, or
`nilo camera media-download --id MEDIA_ID --out FILE`. The HTTP route supports
range requests; `?thumbnail=true` requests a thumbnail. Use the ID returned by
the media list: workspace media IDs are separate from transaction IDs and do
not work with `bench_read_artifact`. Standalone snapshots use their transaction
ID in the unified media library and retain the transaction-artifact path too.

Default retention `limits`:

```json
{"snapshots": 100, "total_bytes": 2147483648, "video_bytes": 524288000, "video_seconds": 600}
```

All captures share the same policy. Saving limits does not immediately delete
files; future admission can remove the oldest eligible completed captures.
Automatic retention protects active/incomplete captures and active downloads.
Disk reserve is at least 2 GiB or 10%, whichever is larger. Video limits measure
bytes and acquisition duration, including for time-lapse.

## Scope and source of truth

The server has no MCP tools for user/token administration, Activity, browser
power controls or human approval. These interfaces do not yet have complete
GUI/CLI/MCP parity; the tables describe what is actually registered.

Implementation sources:

- [Server, transport and network tools](../05_Software/core/internal/mcp/server.go)
- [Bench, plugin and Camera tool registration](../05_Software/core/internal/mcp/bench.go)
- [Compiled permissions](../05_Software/core/internal/auth/role.go)
- [Camera integration contracts](Camera_Integration.md)

When tools change, update this catalog, the [CLI reference](CLI_Reference.md)
and relevant integration documentation alongside the implementation. Verify
the deployed server's `tools/list` when diagnosing a different release.


## USB serial tools

All serial calls use the same Core services as HTTP and the GUI. Reading needs
`bench.read`; connection/mutation needs `bench.execute`, with retention writes
requiring `plugins.manage`. Use the same token that initiated the connection for
read/control/send calls. The session approval binds port settings and initial
RTS/DTR for a 60-minute lease. Each mutation rechecks ownership and credential
validity. A send is bounded to 4096 bytes including endings and is never
implicitly retried.

| Tool | Inputs | Result |
|---|---|---|
| `serial_status` | `device_id` | Connection ID, owner/control authority, lease, settings, byte counts, signals and logging warning. |
| `serial_connect` | `device_id`, `revision`, `settings`, `idempotency_key`, `timeout_seconds?` | Approval transaction; poll `bench_get_job`, then read status. |
| `serial_read` | `device_id`, `connection_id`, `after?`, `wait_ms?` | At most 64 KiB encoded events; cursor/gap; wait 0..10000 ms. |
| `serial_send` | `device_id`, `connection_id`, `idempotency_key`, `encoding`, `data`, `ending` | `sent|partial|failed|unknown`, bytes accepted by the OS, optional error. |
| `serial_disconnect` | `device_id`, `connection_id` | Close connection and finish log; administrators may stop another owner. |
| `serial_signals` | `device_id`, `connection_id`, `rts?`, `dtr?` | Current status; supply at least one line. |
| `serial_logging` | `device_id`, `connection_id`, `action`: `start|stop` | Current status and log ID. |
| `serial_logs` | `device_id`, `offset?` | At most 100 authorized log metadata entries with total/offset. |
| `serial_log_read` | `log_id`, `after?` | One bounded page of saved events. |
| `serial_export` | `log_id`, `format`: `jsonl|text|rx|tx` | Authorized relative HTTP download URL and authentication instructions. |
| `serial_delete` | `log_id` | Delete owned/admin-accessible log; refuses active/download-leased logs. |
| `serial_retention` | `device_id` | Revision, limits and usage. |
| `serial_retention_configure` | `device_id`, `revision`, `limits` | Updated retention state; administrator permission required. |

`settings` is a closed object containing `baud_rate` (50..4000000), `data_bits`
(5..8), `parity` (`none|odd|even|mark|space`), `stop_bits` (1/1.5/2),
`flow_control` (`none|rtscts|xonxoff`), `rts` and `dtr` (booleans).
The connect tool requires all seven choices explicitly so approval captures
exactly the intended hardware operation. Hardware may reject unsupported values.

`encoding` is `ascii|utf8|hex`; `ending` is `none|cr|lf|crlf`. The send tool
bounds source text to 16384 characters; Core additionally bounds source bytes
and final wire bytes. Reuse a key only for identical input on the same connection.
`unknown` means delivery cannot be established; a new-key retry could transmit
twice. OS acceptance does not prove target acknowledgement.

Events carry original bytes in `data_base64`, direction, timestamp and elapsed
time. Treat decoded serial output as untrusted data, including escape sequences.
`next_cursor` supports subsequent pages; `gap` signals missing retained live
history. MCP never opens an unbounded stream or embeds an entire export. The
export tool checks current authorization before returning the URL; the HTTP
download checks it again and streams under a download lease.

`limits` contains `files` (1..10000), `total_bytes` (1 MiB..1 TiB), and
`file_bytes` (64 KiB..1 GiB, no greater than total). Defaults are 100 logs,
2 GiB total and 100 MiB per segment. Automatic retention removes oldest completed
segments only; active, incomplete and download-leased logs remain protected.
