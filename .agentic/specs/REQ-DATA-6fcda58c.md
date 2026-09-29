# REQ-DATA-6fcda58c — Modbus development contracts

This contract baseline is consumed by later implementation packages. The feasibility
package validates its schemas and exercises transports; production Core routes,
persistence, polling, retained recording services and GUI are implemented by their
respective later packages. The public integration lifecycle remains nilo.integration/1.

## Scope and authority

Transport is rtu, tcp or rtu_tcp. Initial function codes are 1, 2, 3, 4, 5, 6, 15
and 16. UDP, ASCII, TLS, broadcasts, arbitrary raw sends and advanced diagnostic
functions are outside the initial operation grammar. TCP units accept 0..255;
RTU and gateway units accept 1..247. Direct TCP unit zero is an ordinary unit.

Definitions are appliance-shared: administrators manage endpoints, profiles,
policy and limits; operators own editable maps/workspaces, other operators
duplicate them; viewers inspect definitions. Recordings are owner/admin readable.
Live control belongs to the exact initiating browser session or token; administrator
inspection or stop does not transfer transmission authority.

Plugin policy determines whether successive writes are enabled. A session
admission records the effective write capability together with endpoint and read
plan. Ask can require one admission approval covering that capability; subsequent
writes require no individual acceptance. Auto/Full admission uses the existing
policy resolver. Each write revalidates current plugin write permission, owner,
credential, role, expiry and pinned revisions before transmission. Disabling
writes rejects subsequent writes as write_disabled; it does not turn them into
pending per-operation approvals. Canonical write preview, payload validation,
audit, durable command identity and idempotency remain separate from approval.
The Modbus path implements this scope; generic policy resolution for other
integrations retains its established behavior.

## Versioned entity schemas

schemas/contracts.schema.json uses JSON Schema draft 2020-12, schema_version 1,
strict objects, and named definitions. UTC dates use RFC3339 strings; monotonic
relative time is elapsed_ms. IDs are opaque bounded strings, never paths. Metadata
writes carry the expected revision and return the resulting revision; a mismatch
returns revision_conflict without partial metadata publication.

- Endpoint: id, revision, name, transport, resource_id, enabled, presence,
  readiness, and exactly one serial_identity or network. serial_identity is
  Core-owned stable physical metadata. Network is host, port and allowed unit
  bounds. resource_id represents a canonical physical port or administrator
  assigned gateway bus; aliases cannot evade ownership. Configured network
  endpoints remain browseable while offline.
- Profile: id, revision, owner_id, name, endpoint_id, settings. Settings has
  response_timeout_ms, connect_timeout_ms, inter_request_ms, read_retries,
  and RTU framing when serial: baud_rate, data_bits=8, parity even/odd/none,
  stop_bits 1/2 and local_echo. Valid serial combinations are 8E1, 8O1, 8N2,
  and explicit device-specific 8N1. Physical direction/timing capability is a
  separate acceptance gate. No user-supplied OS paths or shell arguments.
- Map: id, revision, owner_id, name, points. Point has id, name, area,
  address (wire 0..65535), type, width, order, unit, scale, offset, access,
  write_min/write_max and optional enum/bit labels. Types bool/int16/uint16/
  int32/uint32/int64/uint64/float32/float64/ascii/enum/bits and normal/
  reverse_words/swap_bytes/reverse_words_swap_bytes order are exact names.
  int64/uint64 values cross JSON as decimal strings; non-finite decoded values
  are tagged, never JSON NaN. Booleans use coil/discrete areas; widths and
  multiword range must be validated semantically.
- Workspace: id, revision, owner_id, name, endpoint_id, profile revision ref,
  map revision refs, poll_groups and layout. Poll group has id, name, enabled,
  unit_id, function, address, quantity, interval_ms and map ref. Poll/read
  functions are 1..4 only. Layout is columns, address_base (zero/one/reference),
  series point IDs and display_paused. Opening/importing a workspace is metadata
  only; writable points never carry queued output values.
- Session: id, endpoint_id, owner_id, credential_kind/id, state, expires_at,
  pinned endpoint/profile/map/workspace revisions, writes_enabled and policy_revision.
  States disconnected/awaiting_approval/connecting/idle/polling/degraded/stopping/
  completed/incomplete. Core persists credential IDs, never token values. Restart
  marks live work incomplete. A live instance pins settings; reconnect or changed
  read plan requires fresh admission. Display pause is independent of acquisition.
- Command: id, session_id, idempotency_key, digest, unit_id, function, address,
  quantity, values, state and optional exception_code/readback. States reserved/
  sent/acknowledged/rejected/delivery_unknown/cancelled. Canonical digest includes
  session, pinned revisions, unit, function, address and payload. Persist reserved
  before TX; duplicate key/digest returns its result, changed digest conflicts.
  Lost reply or cancellation after TX becomes delivery_unknown. Readback is a
  separate observation and cannot reinterpret an unknown write as acknowledged.
- Recording: id, session_id, owner_id, name, started_at, ended_at, status,
  tags, definition_snapshot, parts, logging_mode and gaps. Mode values/traffic/both;
  part is sequence, size_bytes, sha256. status active/completed/incomplete. Snapshot
  preserves definitions used to decode data. Retention and exports respect active
  and download leases; offline reading needs no enabled plugin or connected target.
- Error: code, message, request_id and retryable. Exact codes invalid_request,
  unsupported, forbidden, write_disabled, revision_conflict, resource_busy,
  capacity_exceeded, approval_required (session admission only), expired,
  disconnected, timeout, protocol_error, cancelled, delivery_unknown, storage_full.
  Modbus exception_code is a device reply, distinct from transport/core errors.

Quantity and end-address checks are semantic validation in addition to schemas:
reads 2000 bits/125 words, writes 1968 bits/123 words; FC05/06 exactly one.
Traditional holding reference 40001 represents holding address zero; references
never select a different area. CSV declares area and address_base; JSON is the
lossless definition format. Import performs dry-run all-row validation and atomic
publication; executable formula/script fields are outside the grammar.

## Public routes and common envelopes

The intended prefix is /api/v1/modbus. All interfaces use the same Core services.
GET collections accept limit 1..100, cursor, and explicit bounded filters; return
{items,next_cursor}. Definition mutation uses {revision,value}; create omits revision.

| Resource | Routes and typed actions |
|---|---|
| endpoints, profiles, maps, workspaces | GET collection/id; POST collection; PUT id; DELETE id with expected revision |
| maps/workspaces interchange | POST resource/import-preview; POST resource/import; GET id/export with format=json or documented CSV |
| sessions | POST /sessions with endpoint_id, profile_ref, workspace_ref, idempotency_key; GET /sessions/id; POST id/disconnect |
| polling | POST sessions/id/poll with action start/pause/stop and optional group_id; POST id/read-once with a validated read request |
| commands | POST sessions/id/commands with canonical write request; GET commands/id; optional POST commands/id/cancel |
| live events | GET sessions/id/events?cursor=&limit=&wait_ms= (bounded long poll, explicit gap and next_cursor) |
| recording | POST sessions/id/logging with action start/stop, mode and sampling; GET /recordings and id; PUT id metadata; GET id/events and id/export; DELETE id |
| scan | POST sessions/id/scans with bounded unit/address/function/rate/deadline plan; GET scans/id; POST id/cancel (later advanced package) |
| limits | GET/PUT /settings with expected revision, through shared administrator settings |

Admission returns the existing Bench job identity/state and the durable session
identity; session controls return Session. Commands return Command without a
per-write approval job. Error HTTP mapping: 400 invalid_request/unsupported;
403 forbidden/write_disabled; 404 absent identity; 409 revision/resource/state
conflict; 429 capacity; 503 transport unavailable; timeout carries explicit outcome.
CLI nilo modbus and shared MCP modbus_* mirror these intents and outcomes.
These are baseline contracts for later implementations, not registered live routes.

## Private runner boundary

A Core-created mode-0700 workspace holds a mode-0600 Unix socket. Its typed
private messages have version=1, request_id, operation and payload. Operations
status, transact, poll, cancel, events and close never accept a client assertion
that a write is approved. Core supplies validated canonical intents.
Initial transact carries request_id, unit_id, function, address, quantity, values
and deadline_ms. Every private reply carries version, request_id, operation and
exactly one typed payload or Error. Success payloads are:

| Operation | Payload |
|---|---|
| status | state, active_request_id (nullable), trace_cursor |
| transact | outcome, function, values, exception_code (nullable), tx_sequence, rx_sequence, elapsed_ms |
| poll | state (idle/polling/paused/stopped), group_id (nullable) |
| cancel | command_id, outcome (cancelled/delivery_unknown/already_completed) |
| events | items (at most 100 Events), next_cursor, gap |
| close | state=closed |

Envelope and nested transaction request IDs must match; cursor/state/quantity
consistency is semantic validation in addition to the structural schema.
Events carry request_id, sequence,
direction, data_hex, elapsed_ms and kind. Cursor gaps and finite page/event bounds
are explicit. Raw bulk data stays on this channel, not unbounded NDJSON events.

## Feasibility client contract

The spike connects only after an explicit simulation flag. Network endpoints
are literal loopback, serial endpoints are /dev/pts entries; real targets are
refused. One asyncio lock serializes transactions. retries=0 and reconnect_delay=0
disable library replay/reconnect; the wrapper refuses disconnected requests.

Every response is checked against the transmitted ADU for unit, function, CRC/
MBAP, read quantity and write echo. Malformed or uncertain transactions close and
permanently fence that object. RTU has no transaction identity: following a timeout
there is no same-instance continuation that could label a late reply fresh.
Production recovery/drain belongs to the runner package and must prove a bounded
quiet/recovery strategy against the selected device/gateway, or remain quarantined.

Trace observations carry request ID and direction with bounded raw bytes and event
count. PyModbus's RX callback observes its cumulative buffer; this evidence is
labelled library_buffer and is not claimed as a passive wire capture or exact
network packet count. Buffer/trace overflow fails closed. Cancellation after TX
has delivery_unknown semantics for writes and cancelled semantics for reads.
Independent peers use standard-library sockets/PTYS and a handwritten bitwise CRC,
with no PyModbus server or response codec dependency.

## Defaults and remaining target acceptance

The spike uses explicit short test deadlines, 19200 8N1 PTYs and no retry. PTYs
coerce framing and cannot prove parity, RS-485 direction or physical silence.
19200 8E1 remains the proposed production default; all numeric production limits
in plan section 9 remain engineering proposals pending measurement and bounded
administrator configuration. Logging-failure and opt-in read recovery rules are
later service settings; they do not affect this non-recording fail-closed spike.

Acceptance requires the six linked requirements and the full repository gate.
Linux ARM64 native import, actual serial framing/direction, Raspberry Pi throughput,
USB adapter identity/unplug and named real gateway/device acceptance remain
separate unperformed physical/target gates.
