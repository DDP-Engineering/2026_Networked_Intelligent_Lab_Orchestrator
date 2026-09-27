# USB serial sessions and logs

Status: active. The requirement links are listed in
[USB to Serial integration](../knowledge/serial-integration.md).

## Ownership and lifecycle

The plugin ID is `serial`, its display name is `USB to Serial`, and managed
device IDs follow `serial-N`. Discovery is read-only and lists USB devices only.
Naming and configuration use the existing revisioned managed-device API.
Connection settings are explicit in the immutable approval request; changing
them requires reconnecting. The public Bench operation `connect` changes state.
`discover_devices` is read-only. `stream` is private and is rejected by public
submission validation. The manifest declares these three operations and no
artifacts.

Core permits one connection per port and four per appliance. Each connection
belongs to its requesting user and exact browser-session or API-token ID. Core
retains credential IDs only. Every mutation checks authority. Background checks
run once per second and close the connection on revocation, expiry, user disable
or permission loss, plugin disable, or a change to device identity or revision.
Administrators may inspect or stop a connection but cannot send as another owner.

A session lease lasts 3600 seconds. An unrecorded connection closes after
60 seconds without an authenticated read or control request. Restart never
restores a connection or replays TX. Disable stops acquisition and logging.

## Public data

All JSON field names below are exact. Absent optional data is null or omitted.
Times are UTC RFC3339Nano strings; byte sizes are integers. Existing errors and
Bench Job, Device and Plugin objects retain their shapes.

- `SerialSettings`: `baud_rate` is an integer from 50 to 4000000 (default 115200);
  `data_bits` is 5..8 (default 8); `parity` is `none|odd|even|mark|space`
  (default `none`); `stop_bits` is 1, 1.5 or 2 (default 1); `flow_control` is
  `none|rtscts|xonxoff` (default `none`); `rts` and `dtr` are booleans (default
  `true`). Unsupported hardware combinations fail explicitly.
- `SerialConnect`: `revision` is the device revision; `settings` is
  `SerialSettings`; `idempotency_key` contains 8..128 safe characters;
  `timeout_seconds` is the optional Bench timeout. The approval description
  includes the settings, initial line states and 60-minute lease.
- `SerialStatus`: `device_id`, `connected`, `connection_id`, `owner_id`,
  `expires_at`, `settings`, `logging`, `log_id`, `rx_bytes`, `tx_bytes`,
  `warning`, `signals` and `can_control`. The `signals` object contains
  `rts`, `dtr`, `cts`, `dsr`, `ri` and `cd` as nullable booleans.
- `SerialEvent`: `sequence` is a positive integer; `timestamp` is a timestamp;
  `elapsed_ms` is a number; `direction` is `rx|tx|state|gap|error`;
  `data_base64` contains the bytes; `byte_count` records their count;
  `message` is optional. Byte events contain at most 4096 raw bytes.
  `SerialRead` contains an `events` array, `next_cursor` and a `gap` boolean.
  Each runner and Core ring is bounded to 1 MiB of raw bytes and 4096 events.
  Responses are limited to 64 KiB encoded; waits are 0..10000 ms.
  Cursor gaps are explicit.
- `SerialSend`: `connection_id`, `idempotency_key`, `encoding`
  (`ascii|utf8|hex`), `data` (string) and `ending` (`none|cr|lf|crlf`). The
  entire payload, including its ending, is limited to 4096 bytes. ASCII rejects
  non-ASCII input; hex permits whitespace-separated pairs. `SerialSendResult`
  contains `idempotency_key`, `status` (`sent|partial|failed|unknown`),
  `bytes_written` and an optional `error`. Idempotency binds the connection,
  key and payload digest. Core persists the reservation before writing and never
  retries unknown delivery. The queue is bounded to 64 KiB, writes are
  serialized, and the queue and write deadline is 2 seconds. OS acceptance does
  not indicate acknowledgement by the target.
- `SerialAction`: `connection_id` and `action` (`start|stop`), for logging only.
- `SerialSignals`: `connection_id` and optional boolean `rts` and `dtr` fields.
  Manual RTS is refused while RTS/CTS flow control owns it. Unsupported lines
  return an explicit unsupported result.
- `SerialRetention`: `files` defaults to 100 and accepts 1..10000;
  `total_bytes` defaults to 2147483648 and accepts 1 MiB..1 TiB;
  `file_bytes` defaults to 104857600 and accepts 64 KiB..1 GiB, no greater than
  `total_bytes`. The response is `{revision,limits,usage_bytes}`. Updating
  `{revision,limits}` requires an administrator.
- `SerialLog`: `id`, `device_id`, `connection_id`, `owner_id`, `started_at`,
  `ended_at`, `status` (`active|completed|incomplete`), `rx_bytes`, `tx_bytes`
  and `size_bytes`. Listing returns `{logs,total,offset}` with at most 100 logs
  per page. The viewer uses `SerialRead`.

## HTTP routes and Go service seam

Routes under `/api/v1/serial-devices/{id}` require `bench.read`. Mutations also
require `bench.execute`, except where another permission is listed. Core also
checks ownership and the credential.

| Method/path suffix | Request | Response | Manager method |
|---|---|---|---|
| GET `/workspace` | None | `SerialStatus` | `SerialWorkspace(ctx, actor, id)` |
| POST `/connect` | `SerialConnect` | `Job` (202) | `ConnectSerial(ctx, actor, id, input)` |
| GET `/read` | Query: `connection_id`, `after`, `wait_ms` | `SerialRead` | `ReadSerial(ctx, actor, id, connectionID, after, waitMS)` |
| POST `/send` | `SerialSend` | `SerialSendResult` | `SendSerial(ctx, actor, id, input)` |
| POST `/disconnect` | `{connection_id}` | `{ok:true}` | `DisconnectSerial(ctx, actor, id, connectionID)` |
| POST `/signals` | `SerialSignals` | `SerialStatus` | `SetSerialSignals(ctx, actor, id, input)` |
| POST `/logging` | `SerialAction` | `SerialStatus` | `SerialLogging(ctx, actor, id, input)` |
| GET `/logs` | Query: `offset` | `{logs,total,offset}` | `ListSerialLogs(ctx, actor, id, offset)` |
| GET `/retention` | None | Retention response | `SerialRetention(ctx, actor, id)` |
| PUT `/retention` | `{revision,limits}` | Retention response | `UpdateSerialRetention(ctx, actor, id, revision, limits)`; requires `plugins.manage` |

Routes under `/api/v1/serial-logs/{id}`:

- GET `/events?after=N` returns `SerialRead` through
  `ReadSerialLog(ctx, actor, id, after)`.
- GET `/content?format=jsonl|text|rx|tx` streams an attachment under a download
  lease. `ExportSerialLog(ctx, actor, id, format)` returns an `io.ReadCloser`,
  filename, MIME type and error.
- DELETE at the root returns `{ok:true}` through
  `DeleteSerialLog(ctx, actor, id)`. It requires `bench.execute` and owner or
  administrator access. Deleting an active or download-leased log returns a
  conflict. Reading requires `bench.read`.

Core types live in `internal/bench/serial_model.go`; CLI clients use HTTP wire
objects. Generic plugin and device endpoints retain their existing bodies.
Configured serial device settings are `{}`: port choices are supplied on
connect and are not persisted in administrator configuration. The GUI remembers
port and display choices through the preferences store. Generic Bench submission
of `connect` has parameters `{revision,settings}` and binds the exact initiating
credential, as the dedicated route does.

## Private runner boundary

The `serial` package uses the Python SDK and pinned pySerial 3.5. Its manifest
version matches the root `VERSION`; the runner is packaged as an executable.
The package provides physical device access without a simulation mode.

Discovery returns `data.devices` entries shaped as
`{id,kind:"usb-serial",display_name,hardware_identity:{vendor,product,serial,port_path},capabilities:{interface}}`.
The `id` is a `ttyUSBn` or `ttyACMn` basename. `vendor` and `product` contain
four lowercase hexadecimal digits. The serial identity combines the raw USB
serial with `/if`, the interface, `/port` and the port number. `port_path` uses
a stable USB location plus `:ifNN:portN`. This distinguishes multiport endpoints;
duplicate identities use the managed repository's port fallback. Discovery lists
only TTY endpoints with USB ancestry and revalidates identity before opening.

Private `stream` parameters are
`{workspace_directory,hardware_identity,endpoint_id,device_id,settings}`.
SDK configuration uses the ordinary revisioned plugin/device envelope. The
runner serves `control.sock` in the Core directory, with directory permissions
0700 and socket permissions 0600.

- GET `/status` returns `{connected,signals,warning,last_sequence}`.
- GET `/read?after=N&wait_ms=N` returns `SerialRead`.
- POST `/send` accepts `{data_base64}` and returns
  `{status,bytes_written,error}`.
- POST `/signals` accepts optional `rts` and `dtr` fields and returns status.
- POST `/stop` returns `{ok:true}`, closes read/write access and completes the
  stream.

The runner emits TX events once in the same sequenced ring as RX, including
partial writes. Core reads this ring for live data and logging, avoiding duplicate
TX logs. Driver reads have a 100 ms timeout and writes a 2-second timeout.
Opening validates physical identity and uses a POSIX exclusive lock plus Linux
`TIOCEXCL`. Unsupported modem lines are nullable. Private errors use
`{error:"invalid_parameters|unsupported_signals|port_busy|device_unavailable"}`.

## Storage and recovery

Core owns `serial-logs/<UUID>.jsonl` files and their SQLite metadata. The first
line is
`{schema_version:1,type:"header",device_id,connection_id,settings,identity,started_at}`,
followed by `SerialEvent` records. Credential secrets and IDs are excluded.

Active logs reserve their per-file budgets across ports. The filesystem reserve
is the larger of 2 GiB and 10%. Before an append would exceed the file limit,
Core closes the completed segment and admits the next. File counts include all
parts. All owners' logs, including active and incomplete files, count toward
usage. Cleanup selects the oldest completed entries and protects active,
incomplete and download-leased files.

Space failure stops logging and sets a persistent visible warning while
reception continues. An ingestion gap marks the log incomplete. On restart,
Core scans active files, retains their valid JSONL prefix and marks them
incomplete. Exports stream JSONL, escaped readable text, or exact raw bytes for
the selected direction.

## Clients and verification

CLI `nilo serial` provides `status`, `connect`, `read` (with `--follow`), `send`
(from `--data-file` or stdin), `disconnect`, `signals`, `logging`, `logs`,
`log-read`, `export`, `delete` and `retention`. It uses the existing token-file
conventions. MCP exposes finite equivalents named `serial_*`; large exports
return authorized download URLs and instructions. Both clients use Bench policy.

The GUI provides a System administrator page and per-instrument RealTime View
and Sessions pages. Controls wrap responsively; Display settings, Port settings,
and Transmit and signals collapse into separate cards beneath the combined
terminal and send panel. Fill workspace and fullscreen allocate remaining height
to the terminal; narrow screens preserve access through wrapping and scrolling.
Fullscreen exposes an exit action and follows browser Escape changes. History is
bounded to 2 MiB and 10000 rows, with a virtualized terminal. Display uses plain
text, incremental UTF-8 decoding, escaped controls and explicit gaps. Status
includes authority, expiry, byte counts and logging warnings. Export and delete
use the established confirmation and permission rules.

Verification covers USB identity metadata; real Linux PTY RX/TX, busy ports and
closure; strict encoding and endings; TX replay prevention; expired or revoked
credentials; cross-owner access; immutable approval; bounded rings and slow
readers; recovery, quotas and download leases; GUI keyboard access and reflow
at widths of 320, 800 and 1024 pixels and desktop sizes; and CLI/MCP parity.
Physical USB, baud-rate and signal acceptance awaits supplied hardware and
user-directed Raspberry Pi deployment.
