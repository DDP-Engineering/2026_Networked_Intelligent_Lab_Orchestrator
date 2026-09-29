# Modbus master/client plugin development plan

Status: proposed implementation baseline, 2026-09-28. Planning and issue creation
only; no Modbus implementation or deployment is included in this change.
Repository evidence: `master`, commit `a89db2fa89b0fdf8a77f24833555b617122cb344`.

## 1. Scope, decisions and terminology

**STATED by the user:** build the next NILO integration as a Modbus application,
inspired by Radzio Modbus Master Simulator and Modbus Poll. Include configurable
and saved sessions, retained logs, Find, exports, register definitions, controls,
Modbus RTU, TCP and RTU over IP. Follow Camera/USB to Serial's separation between
System plugin administration and Instruments workspaces. Implementation will use
Codex and GPT models. Save the plan in `00_Planning` and create Gitea tickets.

**Confirmed answers:** first delivery is master/client only; configurations and
register maps are shared across the appliance, with one owner controlling a live
session. Slave/server behavior is permitted only in isolated test fixtures.

**PROPOSED, awaiting scope acceptance:** named typed register maps with scaling,
units and CSV/JSON interchange; interpret RTU over IP initially as RTU over TCP;
read polling receives a bounded session approval, while each write is a separate
typed operation subject to the plugin policy. These proposals are not approvals
to implement new behavior. If further user answers arrive, reconcile this plan.

Terminology:

- **Endpoint:** one appliance-attached serial adapter or configured network peer.
- **Profile:** saved connection settings and target name, independent of a socket.
- **Register map:** reusable named points describing device data and write access.
- **Workspace definition:** saved poll groups, map revisions, grid/chart layout.
- **Live session:** credential-bound ownership of an endpoint and its scheduler.
- **Recording:** immutable session metadata and retained traffic/sample log parts.

Opening a saved definition never connects, starts polling, replays a write, or
resumes a previous hardware session automatically. Saved recordings open offline.

## 2. Reference application analysis

The following findings come from vendor documentation viewed on 2026-09-28,
not from executing downloaded Windows programs. Links support specific features;
the NILO design in later sections is an original proposal. No reference program
source, binary, artwork or proprietary workspace format is copied.

| Reference | Verified behavior | Implication for NILO |
|---|---|---|
| [Radzio](https://en.radzio.dxp.pl/modbus-master-simulator/) | Master utility; multiple slave devices; RTU, TCP and RTU over TCP; ASCII/float display; cell colors and frame-counter reset. The page lists its last update as 2017. | Useful simple polling/grid baseline; do not assume undocumented persistence or logging features. |
| [Modbus Poll](https://www.modbustools.com/modbus_poll.html) | Multiple monitoring windows, read/write operations, exception reporting and additional protocol variants. | Multiple poll groups within NILO tabs; explicit mutation actions and useful errors. |
| [Poll manual](https://www.modbustools.com/mbpoll-user-manual.html) | Per-window unit, function, address, quantity and scan rate; one-shot requests, disabling polling and error stop; serial/network connection settings. Saved workspaces contain layout/references and connection/chart settings rather than captured data. CSV export and unit scanning are documented. | Separate definition storage from recordings; distinguish requested poll rate from actual progress; keep stop/read-once controls. |
| [Display formats](https://www.modbustools.com/poll_display_formats.html) | Integer and floating interpretations of several registers with word/byte ordering. | Preserve raw words alongside decoded values. |
| [Scaling](https://www.modbustools.com/poll_scaling.html) and [colors](https://www.modbustools.com/poll_color.html) | Numeric scaling and conditional cell styles. | Named engineering values and accessible conditions; color also has a text indicator. |
| [Traffic](https://www.modbustools.com/poll_traffic.html) | Shows application TX/RX, not passive bus sniffing. | Correlated traffic panel and recording, clearly scoped to NILO requests. |
| [Text logging](https://www.modbustools.com/log.html) | Timestamped value logging. | Structured retained samples plus human-readable/CSV exports. |
| [Charting](https://www.modbustools.com/poll_chart.html) | Selected value series, zoom/pan and bounded/end-of-chart behavior. | Bounded real-time trends plus offline log trends. |
| [Address scan](https://www.modbustools.com/poll-address-scan.html) | Probes register addresses and exports results. | Explicit bounded read scan with progress and cancellation. |
| [Test Center](https://www.modbustools.com/test.html) | Saved test strings, manual sending and checksum options. | Later typed diagnostics; unrestricted raw frames are deferred. |

Search over named points, session metadata and recorded transactions is requested
NILO scope. These pages do not establish a general full-text Find capability in
both reference apps. Excel OLE/VBA automation, desktop printing, ASCII/UDP,
Enron/Daniel variants, TLS Modbus Security and proprietary file import are not
first-delivery requirements. NILO substitutes CSV/JSON export and shared CLI/MCP
operations for desktop-only automation.

## 3. Existing NILO patterns and required seams

**OBSERVED repository behavior:**

| Existing seam | Reuse and required change |
|---|---|
| `05_Software/frontend/src/components/pages/CameraPluginPage.tsx` | Administrator-only lifecycle, inline policy, names, enablement, local verified update and View activity. Currently parameterized for Camera/Serial; create a Modbus-specific page or a small shared administration component without translation-key substitution. |
| `src/components/shell/pages.ts`, `SideNav.tsx`, `AppShell.tsx` | Dynamic instrument groups and named child pages. Add Modbus descriptors and permission filtering; honor sidebar issue #26 when integrated. |
| `SerialWorkspacePage.tsx`, `SerialSessionsPage.tsx`, `SerialTerminal.tsx`, `serialBuffer.ts` | Live connection versus saved-recording pages; logging, bounded display, Find, preview/export/delete patterns. Adapt to transactions/register grids rather than terminal strings. |
| `05_Software/core/internal/bench/{adapter.go,packages.go,managed_devices.go,hardware_monitor.go}` | Curated family registration and revisioned settings. Several hooks explicitly name camera/serial; generalize affected hooks narrowly and preserve those families. |
| `bench/{serial_session.go,camera_workspace.go,scheduler.go,execution.go}` | Dedicated session runners after finite setup jobs; ownership, policy and cleanup. Add a Modbus session manager and avoid network/device waits under the global Manager mutex. |
| `internal/store/sqlite/migrations/019_managed_devices.sql` | Device identity is USB-oriented. Reuse physical serial identity, but do not invent USB fields for TCP peers; introduce endpoint/profile records. |
| `internal/integration/{manifest,protocol,supervisor}` and `05_Software/integrations/sdk/python` | Keep `nilo.integration/1`, executable-plus-argument runners, verified inventories and process isolation. Bulk live data uses a confined session channel, not unbounded NDJSON events. |
| `internal/httpapi`, `internal/mcp`, `core/cmd/nilo` | GUI, CLI and the shared NILO MCP server call the same Core services. |
| `02_Tools`, `07_Delivery/README.md` | Reproducible dependencies and one project release with all integrations, Linux amd64/arm64 targets and separate physical acceptance. |

Issue [#25](http://192.168.1.15:3001/dpasare/2026_Networked_Intelligent_Lab_Orchestrator/issues/25)
proposes configurable operating limits. Adopt its shared setting service when
available; otherwise use one documented revisioned Modbus configuration boundary
that can integrate with it, with no scattered magic numbers.
Issue [#26](http://192.168.1.15:3001/dpasare/2026_Networked_Intelligent_Lab_Orchestrator/issues/26)
proposes the collapsed Instruments/System shell. Modbus does not create a new
Bench navigation section and can be implemented against either shell baseline.

## 4. Transport and protocol coverage

Proposed first-delivery transports:

| Mode | Configuration | Framing and scheduling |
|---|---|---|
| RTU serial | Stable USB adapter; baud, 8 data bits, parity, stop bits, supported RS-485 direction mode, echo handling; device/unit IDs | RTU CRC and serial timing; one transaction in flight on the physical bus, regardless of how many units/poll groups use it. |
| Native TCP | Administrator-approved hostname/IP, port (suggest 502), optional explicit supported source/interface selection, timeouts; unit ID | MBAP framing and transaction correlation; TCP stream reassembly. Start with one request in flight per connection for predictable devices. |
| RTU over TCP | Approved gateway host/port and downstream unit IDs, timeout/turnaround settings | Complete RTU frames including CRC over TCP, without MBAP. Handle split/coalesced socket reads. No inference of serial timing from TCP packet boundaries. |

UDP and RTU over UDP remain follow-ups unless the user confirms that RTU over IP
requires them. No browser Web Serial or direct browser TCP: adapters and network
connections belong to the NILO appliance. No arbitrary serial paths, URLs or
shell arguments are accepted from clients. Configured targets are explicit;
there is no automatic subnet scan. Hostname resolution is bounded and its result
checked against the approved endpoint/network policy before each new connection.
Gateway mode and network policy do not alter appliance management routing.

The normative baseline is the
[Modbus application specification](https://www.modbus.org/file/secure/modbusprotocolspecification.pdf)
and [serial guide](https://www.modbus.org/file/secure/modbusoverserial.pdf).
Use wire addresses 0-65535 internally; display base-0, base-1 or traditional area
references as presentation choices. A holding-register reference 40001 maps to
wire address 0; area is a separate field. Reject end-of-range overflow.

| Function | Planned delivery | Maximum quantity per request |
|---|---|---|
| FC01/02 coils/discrete inputs | Initial | 2000 bits read |
| FC03/04 holding/input registers | Initial | 125 registers read |
| FC05/06 single coil/register | Initial | One |
| FC15 multiple coils | Initial | 1968 bits written |
| FC16 multiple registers | Initial | 123 registers written |
| FC22 mask write; FC23 read/write | Follow-on within this plan | One masked register; FC23 125 read / 121 written |
| FC43/14 identity; selected FC08, FC11, FC17 | Diagnostics increment | Capability-specific, explicitly validated |

Apply smaller device limits when configured. Do not use marketing's generic
125-register claim as the FC16 write limit. Input registers/discrete inputs are
read-only. Validate quantity, area, function, value count and response consistency.
Non-core functions return unsupported explicitly rather than silently emulating.

Serial unicast IDs are 1-247. Serial broadcast ID 0 is a distinct write-only
operation with no response and no success acknowledgement; disable it by default
and defer it pending an explicit scope decision. TCP unit ID is its own 8-bit
setting; direct TCP unit 0 does not automatically mean serial broadcast. Gateway
profiles constrain IDs to their documented downstream behavior.

RTU timing follows the serial guide and the selected framing; a configured
inter-request delay cannot reduce mandatory bus gaps. Prefer adapters with
automatic direction control. Native Linux RS-485/RTS modes must be capability
checked and physically tested; do not copy arbitrary terminal RTS/DTR controls
into a polling bus. The serial guide's standard defaults motivate proposed
19200 baud, 8E1, with valid 8N2/8O1 options and explicitly explained device-specific
8N1 support only if selected during feasibility. Real driver read-back is required.

## 5. Sessions, polling, concurrency and writes

One runner owns each active transport endpoint. One fair scheduler inside that
runner serves multiple units and poll groups on the endpoint. Different serial
adapters/network endpoints progress independently alongside camera and terminal
sessions. Browsing multiple unit pages on one bus never creates competing masters.

**Required shared resource change:** current claims are plugin/device scoped.
Introduce a canonical physical-port lease shared by Modbus and USB to Serial,
bound to stable physical identity and held for the entire live session. Both
plugins must use it; Python/OS exclusive opening is additional protection, not
the admission model. Do not make Modbus depend on the Serial plugin being enabled.
For aliases to one configured gateway, use an administrator-assigned bus/resource
identity; hostname aliases must not accidentally create separate serial masters.

Connect is an approved finite job; the subsequent live lease and polling do not
occupy a finite job slot forever. Session states: disconnected, awaiting approval,
connecting, connected/idle, polling, degraded, stopping and terminal. Poll groups
can be paused independently. Disconnect stops polling, settles in-flight commands,
finalizes recording and releases resources only after descriptor/socket cleanup.
Browser display pause is separate from poll pause. Browser tab closure does not
silently stop explicitly recorded work; idle expiry applies only without logging.

Saved definitions are revisioned. A live session pins target, transport settings,
map/workspace revisions, actor, credential, endpoint policy and expiry. Changing
definitions does not mutate live authorization. Applying changed connection
settings requires reconnect; applying a changed read plan requires a new approval
or a documented authorized revision update. Revocation, disable, expiry, removal,
unplug, replaced runner and shutdown each have deterministic cleanup.
After Core restart, active sessions/recordings are incomplete and never resumed
automatically. Read reconnect is opt-in, bounded and revalidates the lease.

Scheduling rules: monotonic due times; no overlapping requests on a client; fair
rotation of units; bounded queue and read retries; a slow/offline unit cannot
starve the whole bus. Missed periods are counted/coalesced, never accumulated into
unlimited catch-up work. Show requested interval, actual interval, latency, sample
age, missed polls and errors. Do not merge across unit, area, map revision or
undocumented register holes. Multiword values stay in one valid read when possible;
multiple replies are not claimed to be an atomic device snapshot.

After an RTU timeout, establish a bounded recovery/drain and quiet interval before
the next request; an identical unit/function reply has no TCP-style transaction
ID. A late reply cannot be labelled a fresh sample simply because its CRC is valid.
Read scans use an explicit approved function/address plan: some vendor reads have
side effects. A failed probe does not prove a unit absent, and a successful read
does not prove the address writable or reveal its engineering type.

Writes use FC05/06/15/16 first, including explicit multi-register typed encoding.
Editing a cell opens a preview/confirmation; typing never directly transmits.
Preview shows endpoint, unit, area, wire/display address, function, raw words,
engineering value, map revision, permitted range and optional read-back.
Under Ask policy each canonical write job needs its own approval. Auto/Full Access
use the existing resolver; roles and tokens cannot bypass policy. Bulk writes
show each planned request; discontiguous writes have per-request outcomes, not
transactional rollback. No background repeated writes or toggle macros in v1.

Durable idempotency is created before transmission. A lost response is
**delivery_unknown**, not a failed write safe to repeat. Configure the underlying
library with automatic write retries disabled; bound read retries separately.
Restart/reconnect never replays a pending/uncertain write. Device acknowledgement
and optional subsequent read-back are reported separately; cancellation after TX
cannot undo the device change. FC22 mask writes avoid pretending that a host-side
read/modify/write is atomic. Raw frames and disruptive diagnostics stay deferred.

## 6. Named register maps and data representation

Proposed map editor/import contract: stable point ID, name, description, area,
wire address, data type/width, byte and word order, unit, scale/offset, precision,
enum labels, bit labels, read/write designation, write limits, optional supported
write function, and poll group assignment. Unit IDs belong to poll groups, allowing
one reusable map for several identical devices. Maps have immutable revisions.

Types: Boolean, signed/unsigned 16/32/64-bit integers, IEEE float32/float64,
fixed-length ASCII, enums and bitfields. Initial multiword orders: normal words,
reversed words, byte-swap within each word, and both; provide byte examples rather
than ambiguous endianness names alone. Exact raw words remain visible.
64-bit integers use decimal strings over JSON to avoid JavaScript precision loss.
NaN/infinity use explicit tagged values and quality, not invalid JSON numbers.
Strings declare length, padding and decoding; unsupported bytes remain inspectable.

Scaling is a declarative linear transform, with an explicit inverse and rounding
rule for writes. Reject overflow, zero inverse scale, non-finite writes, fractional
integer results when not allowed, truncated multiword values and invalid enum/bit
definitions. No executable formulas, scripts or imported code. Bitfield writes
need a supported mask operation or a clearly non-atomic approved alternative.

Show quality (good, stale, timeout, exception, disconnected, decode_error), last
valid timestamp and current status separately. A timeout does not overwrite a
previous value with zero. Conditional styling includes labels/icons and respects
light/dark/high-contrast presentation. Comparison of maps shows changed fields.

Map import/export: versioned UTF-8 CSV and JSON, dry-run validation, clear address
base, bounded size/row count, duplicate/conflict report and explicit replace/merge
choice. JSON preserves every field; CSV documents its row columns and nested
field encoding. Validate all rows before atomic metadata publication. Treat
spreadsheet-formula-leading text safely in CSV without altering raw register data.

Example proposed map (not a seeded real device):

| Point | Area/address | Type | Interpretation | Access |
|---|---|---|---|---|
| Temperature | Holding / 0 | int16 | raw * 0.1, degrees C | Read |
| Speed | Holding / 2 | uint32 | Normal word order, rpm | Read |
| Setpoint | Holding / 4 | int16 | raw * 0.1, degrees C, bounded device range | Explicit write |
| Running | Coil / 0 | Boolean | On/off | Explicit write if allowed |

## 7. Frontend menus, controls and workflows

### System -> Plugins -> Modbus

Administrator-only, matching the established plugin page:

- Header with Modbus, View activity, project version and verified local update.
- Enable/Disable, policy, state and Install/Uninstall/Restart through the existing
  lifecycle surface; busy operations and confirmations follow shared patterns.
- Discover USB adapters without opening them. Named physical adapter rows retain
  presence/configuration when unplugged; no generated physical devices.
- Add/Edit/Duplicate/Enable/Disable network endpoint profiles; explicit Test
  connection performs bounded I/O through the same policy and service paths.
- Edit transport limits, target ranges, retention and defaults; distinguish
  global platform settings from Modbus-specific limits and per-profile settings.
- Default register maps and device capability overrides; map/profile assignment
  and revision conflicts have clear feedback. Physical identity is supporting
  information, not a user-entered filesystem path.

Discovery of adapters is different from probing Modbus units. Network endpoints
can be configured and enabled without being reachable; keep their instrument
entry available with an offline state so users can inspect saved work.
Disabling a plugin/endpoint stops its sessions and closes logs. Uninstall preserves
recordings/definitions with a supported offline access path, or refuses until an
explicit export/delete choice; never leave retained data unreachable silently.

### Instruments -> Modbus -> named endpoint

Modbus is an expandable navigation group without a redundant overview. Each
configured, enabled endpoint offers **RealTime View**, **Sessions**, and
**Register Maps**. RealTime View includes the saved workspace/profile selector;
the endpoint page can monitor multiple unit IDs. No extra top-level Bench menu.

| Area | Controls and behavior |
|---|---|
| Header/toolbar | Endpoint name, connection state, owner/expiry, Connect/Disconnect, Start/Pause/Stop polling, Read once, Start/Stop logging, Save workspace, Fill workspace/Full screen. Separate physical/polling state from display state. |
| Connection panel | Transport-specific validated settings; network host/port or adapter selection, unit defaults, connect/response timeout, read retry/backoff, inter-request delay; effective settings/read-back. Changes requiring reconnect are explicit. |
| Poll groups | Add/Duplicate/Delete, enable, name, unit ID, function/area, wire address, quantity, interval, timeout override, stop-on-error choice, selected map revision. Individual pause and Read once. |
| Register grid | Address/reference, point name, raw hex/words, decoded value, units, quality, last update and write availability. Select columns, sort/filter, Find next/previous, copy selection, bit view, change highlights; virtualized bounded rows. |
| Write dialog | Selected point/range, engineering/raw input, exact encoded preview, write function, range validation, policy approval state, Submit/Cancel, acknowledged/unknown/read-back result. Accessible button action in addition to double-click. |
| Traffic panel | Timestamp, direction, unit, function, transaction ID, request/response pairing, raw hex and decoded summary, latency and errors. Find hex/text, direction/function/error filters, Pause display, resume tail and Clear display. Clearing never deletes logs. |
| Counters/status | Requests, responses, exceptions, timeouts, CRC/framing faults, queue usage, missed polls, effective rates. Reset display counters is a local baseline, not deletion of durable evidence. |
| Trend panel | Add named numeric series, legend, units/axis selection, bounded history, pause display, zoom/pan/reset, sample gaps/stale markers; export selected time range as CSV and chart as PNG. |
| Tools | Bounded read-only unit/address scan with explicit range/rate/function, progress/Cancel and export; typed diagnostics on supported transports. |

Workspace definitions save poll groups, selected map revisions, columns, display
base, filters, chart series and layout. Save As/Duplicate/Rename/Open/Delete and
JSON import/export support shared bench workflows. Dirty changes and revision
conflicts are visible. Loading definitions containing writable points does not
send values. Imported definitions cannot enable broadcasts or bypass target policy.

Register Maps provides a revisioned table/form editor, Find, validate, duplicate,
import preview, export and assignment. Editing common definitions is visible to
other users but never silently changes their running sessions.

Sessions provides named recordings with endpoint/transport/unit summaries,
start/end/duration, owner, complete/incomplete status, tags/notes, event count and
size; metadata search/filter/sort and paginated viewing. Opening a recording offers
Register samples, Traffic and Trends, all offline and searchable. Missing/removed
hardware does not prevent authorized access to history.

Use existing wrapping toolbars, collapsible configuration, 44-pixel targets,
minimum 14-pixel text, themes and EN/DE/RO catalogs. Verify 800x480, 1024x600,
desktop and 320-pixel reflow; internally scrolling grids do not overflow the page.
Help explains addressing, transport choice, quantity limits, poll versus display
pause, stale values and uncertain writes in operator terms.

## 8. Persistence, logging, Find and export

Proposed metadata entities: `modbus_endpoints`, `modbus_profiles`, versioned
`modbus_maps`/points and `modbus_workspaces`/poll groups, `modbus_sessions`,
`modbus_commands`, `modbus_recordings`/parts and deletion/download leases.
Choose exact tables and constraints at design. Network endpoint identity must be
explicitly separate from USB physical identity; uniqueness uses canonical targets
and configured shared-bus identity, not display name.

Definitions are appliance-shared. Proposed editing rule: administrators manage
endpoints/policy; operators create/edit their own shared maps/workspaces; other
operators duplicate to edit; administrators can manage all. Viewers can inspect
shared definitions but cannot connect, poll or write. Session control remains
credential-bound to its owner. Proposed retained-log visibility is owner/admin,
distinct from shared definition visibility; do not broaden it accidentally.

Live data goes through bounded rings/cursors with explicit gap reporting.
Recording writes append-only chunks under Core-confined storage, plus metadata,
part digests and indexes. Record UTC timestamps and monotonic relative times;
sequence numbers establish order. Include pinned definition snapshots, raw
requests/replies where enabled, sampled raw/typed values, quality/errors and write
outcomes. Packet chunks are not necessarily complete Modbus transactions.

Logging modes: values only, traffic only, or both; every poll, interval sampling,
or change-only value logging with quality transitions. State what can be recovered
from each mode. Start/Stop logging is independent of Start/Stop polling.
Name/tag recordings; automatic rotation uses one logical recording with parts.
Disk-full/retention failure stops logging with an explicit warning; it never
silently drops evidence while claiming a complete log. Continuing polling after
logging failure is a proposed configurable choice, visible in session policy.

Find must cover point names/addresses/values, saved-definition metadata, session
names/tags/dates, and retained traffic hex/function/unit/error plus typed samples.
Use bounded server-side paginated queries/indexes for stored logs; do not download
the entire recording to search it. Show scope, count/cursor, no-match and truncation
states. Initial literal/hex search and structured filters; regex is deferred.

Exports: lossless definition JSON, register-map CSV/JSON, current snapshot CSV,
sample CSV, traffic JSONL/readable text, selected chart CSV/PNG, and a recording
bundle containing metadata/definition snapshots and log parts with digest manifest.
Bulk exports stream with byte/row/time bounds; show incomplete/gap status and exact
selection. JSONL is the canonical lossless log exchange. Document binary/hex
encoding and int64 representation. Deleting completed/incomplete recordings is
confirmed, authorization-checked and refused during active logging/download leases.
Retention removes the oldest eligible completed parts without deleting live ones.

## 9. Core boundary, automation and proposed defaults

Proposed runner: `05_Software/integrations/modbus/`, Python SDK, a pinned Modbus
library evaluated in MB-01, and shared physical serial primitives rather than
unvalidated reuse of the terminal transmit API. Candidate
[PyModbus stable client documentation](https://pymodbus.readthedocs.io/en/stable/source/client.html)
documents serial/TCP clients, configurable framing, tracing/retry hooks and the
need to serialize calls on one client. Do not select a development release by
following the documentation's `latest` URL. Prove RTU-over-TCP framing, exclusive
opening, no-write-retry control, cancellation and dependency licensing before
pinning an exact version/hash in `pyproject.toml`, `uv.lock` and bundle inputs.

Core owns authorization, jobs, metadata, physical leases, idempotency, retention,
export and audit. Runner owns bytes, protocol checks, endpoint scheduler and
bounded acquisition; it never accepts a client claim that a write is approved.
Use a mode-0700 workspace and private mode-0600 Unix socket with bounded HTTP or
typed messages, no public runner listener. Core validates both commands and
runner responses. Never reuse Camera-named transport helpers as a long-term
Modbus contract; extract only truly shared private transport primitives.

Proposed public resources under `/api/v1/modbus`: endpoints, profiles, maps,
workspaces, sessions, poll groups, commands, recordings and scans. Use strict
typed schemas, revision checks, durable command IDs, finite pagination/cursors,
and existing job approve/cancel surfaces. Freeze exact routes in MB-01/MB-04.
CLI `nilo modbus` and MCP `modbus_*` tools expose equivalent list/configure,
connect/disconnect, status, read/poll, write, scan, search, recording and export
semantics. AI clients submit canonical intents; only the existing human approval
surface approves Ask operations. No separate Modbus MCP server or shell passthrough.

Proposed defaults (engineering proposals, not hardware capacity guarantees):

| Setting | Initial proposal | Configuration owner |
|---|---|---|
| Concurrent endpoint sessions | 4 total Modbus endpoints | Administrator; coordinate #25 and shared physical-port capacity |
| In-flight requests | 1 per endpoint | Fixed initial scheduler contract; TCP pipelining deferred |
| Poll groups / mapped points | 32 / 4096 per workspace | Administrator-bounded limits |
| Default poll interval / minimum interval | 1000 ms / 100 ms | Profile/group settings within administrator bounds and transport timing |
| Response / connect timeout | 1000 ms / 3000 ms | Profile settings, constrained by operation deadline |
| Read retries / write retries | 1 / 0 | Read bounded and adjustable; write replay prevention fixed |
| Session lease / nonlogging idle expiry | 60 minutes / 5 minutes | Administrator; expired lease requires fresh admission |
| Live ring | 1 MiB raw data and bounded event count | Administrator ceiling with explicit cursor gaps |
| Retention | 100 recordings, 2 GiB total, 100 MiB per part | Administrator; compare with Serial defaults, measure before acceptance |
| Scan | Explicit unit/address range; 10 requests/sec cap; 60-second default deadline | Administrator-bounded, cancellable and sharing the endpoint scheduler |

Before adopting each value, measure on the Raspberry Pi and amd64 appliance and
define allowed ranges, quota accounting, active-session update rules and overload
behavior. A configured limit cannot override wire protocol bounds or resource
exclusivity. Shared-lock latency and logging throughput belong in acceptance.

## 10. Implementation work packages for Codex

These are sequentially reviewable implementation units, not completed features.
Each future ticket starts its own AEGIS frame/requirement work and rechecks the
current checkout. Tests are part of every package; MB-10 adds integrated proof.
No model/version assignment is required: use the configured GPT model and give
it the relevant ticket, plan sections, repository instructions and acceptance.

| ID | Outcome | Dependencies | Demonstrable acceptance |
|---|---|---|---|
| MB-01 | Resolve scope proposals; protocol/dependency spike; schemas and behavior contracts | None | RTU/TCP/RTU-over-TCP read/write fixtures, fragmented stream tests, no-write-retry evidence, declared unsupported features, dependency lock/license proposal; user decisions recorded. |
| MB-02 | Endpoint/profile persistence and shared serial-bus leases | MB-01 | Stable USB identity and TCP records; schema revisions; Serial/Modbus race yields one owner; entire live lifetime holds canonical resource; disable/crash/unplug cleanup preserves other sessions. |
| MB-03 | Supervised Modbus runner and fair transport scheduler | MB-01, MB-02 contracts | Core functions on all three transports; CRC/MBAP/response validation, timing, unit fairness, late responses, bounded retries/cancel; independent endpoints run together. |
| MB-04 | Core sessions, polling and durable writes, API/CLI/MCP | MB-02, MB-03 | Read approval/lease, per-write policy, request digest/idempotency, unknown delivery, no replay; role/token revocation and equivalent GUI/CLI/MCP results. |
| MB-05 | Register map/workspace schemas, codecs and definition interchange | MB-01, MB-02 | Address conversions, typed endian vectors, int64 precision, scale/inverse bounds, shared revisions and atomic CSV/JSON import; loading never sends writes. |
| MB-06 | Modbus administration and Instruments navigation | MB-02, MB-04 | System plugin lifecycle, profiles/adapters, admin policy/retention controls and named instrument pages; configured offline TCP targets and zero-adapter state work. |
| MB-07 | Live grid, polling/traffic/write controls and trends | MB-04, MB-05, MB-06 | Multiple units/groups, pause distinctions, typed write preview, search/copy, stale/gap states, bounded charts; accessible responsive EN/DE/RO UI. |
| MB-08 | Retained sessions/logs, offline viewer, search/export/retention | MB-04, MB-05; integrates MB-07 | Rotation/disk-full/crash recovery; authorized indexed Find; streaming CSV/JSONL/bundle export; deletion/download leases; offline traffic and trends. |
| MB-09 | Bounded scans and advanced typed diagnostics/functions | MB-03, MB-04, MB-05, MB-07 | Explicit unit/address scans with cancel/fairness/export; FC22/23 and identification/read-only diagnostics; disruptive FC08/raw/broadcast operations remain absent unless approved. |
| MB-10 | Offline bundles, deterministic simulators and appliance acceptance | All prior packages | Complete dependency closure/notices on amd64/arm64; container/contract/browser gates; cross-plugin capture concurrency; named physical RS-485/gateway acceptance kept distinct from simulation. |

Suggested execution prompt for each ticket:

> Implement only MB-xx from `00_Planning/Modbus_Plugin_Plan.md`. Read the root
> AGENTS.md, `.agentic/AGENTS.md`, relevant component pages and the ticket first.
> Reconcile scope decisions and current code before proposing changes. Use AEGIS
> checkpoints and the repository's failing-test-first workflow; keep API, CLI and
> MCP outcomes consistent. Do not commit, push, deploy, connect to real targets or
> broaden function/transport coverage without the corresponding user instruction.
> Report exact verification evidence and any unresolved acceptance requirements.

## 11. Verification and delivery acceptance

Use independent protocol fixtures in addition to the selected library's server,
so a shared client/server defect cannot validate itself. Fixtures run only in
explicit simulation/test environments and never discover real lab targets.

- Protocol: golden CRC/MBAP/quantity/address vectors; split/coalesced TCP data,
  partial serial frames, wrong unit/function/transaction/length, CRC errors,
  exceptions, malformed input, delayed/duplicate replies and reconnect fencing.
- RTU: real Linux pseudo-terminals plus physical USB-RS485 checks for framing,
  direction, turnaround, echo, baud/parity, unplug and identity replacement.
- Concurrent operation: two Modbus serial adapters; multi-unit fairness on one
  bus; several TCP targets; Modbus + Camera + unrelated USB terminal; rejection
  when Modbus/terminal alias the same adapter. Measure starvation/latency.
- Mutation: exact write bytes, 123-register FC16 limit, packed coils, stale map,
  policy denial, expired approval, idempotent duplicate submission, lost reply,
  restart after TX and cancellation. An uncertain write is never resent.
- Data: signed/unsigned/float endian vectors, NaN/int64/string/scale edges, map
  conflicts and import rejection; record snapshot stays reproducible after edits.
- Storage: gap/search index behavior, quotas/rotation, active deletion refusal,
  leased export, incomplete recovery, bounded-memory long runs and CSV safety.
- UI/authorization: admin/operator/viewer, token scopes, ownership/revocation,
  keyboard/screen reader, supported viewports/themes/languages, empty/offline and
  pending/denied/error states; real Core paths for acceptance, not only mocks.
- Packaging: pinned checksum-verified offline inputs under `02_Tools`, Linux
  service permissions, no root runner, one project version and all integrations
  in each release target. Run repository/container and appliance-stage gates.

First physical acceptance needs a named USB-RS485 adapter, one or more Modbus
slaves with known maps, a TCP peer and an RTU-over-TCP gateway, plus safe test
registers/coil outputs. Record hardware/firmware/settings and evidence. No
deployment or device writes are authorized by this planning request.

## 12. Open decisions and implementation handoff

| Decision | Current status / proposed resolution |
|---|---|
| Client versus server | CONFIRMED: master/client only. Test servers stay fixtures. |
| Shared definitions/live owner | CONFIRMED: shared definitions, one live controller. |
| Named typed maps in first delivery | PROPOSED: include CSV/JSON, units/scaling; pending answer/scope acceptance. |
| Meaning of RTU over IP | PROPOSED: RTU over TCP in first delivery; ask whether UDP is needed. |
| Write approval granularity | PROPOSED: each canonical write follows existing policy; no blanket session-write grant. |
| Shared definition edit/log visibility | PROPOSED owner/admin edit, read-shared definitions, owner/admin recordings; confirm at MB-01. |
| Default limits, logging failure action and reconnect | PROPOSED values in section 9; validate on targets, coordinate #25. |
| Broadcast, raw test frames, disruptive diagnostics | Deferred until explicit scope/policy design. |
| Supported adapter/gateway models and real target map | UNKNOWN; hardware acceptance prerequisite, not a blocker for planning. |
| Selected dependency/version/license closure | UNKNOWN until MB-01 spike; documentation is not feasibility proof. |

Implementation issue links are recorded below. The
epic includes the full plan text so it remains reviewable before any Git commit
or push; local source references alone are not an unpublished attachment.

### Gitea implementation backlog

Epic: [#27 - Modbus master/client plugin](http://192.168.1.15:3001/dpasare/2026_Networked_Intelligent_Lab_Orchestrator/issues/27).

| Work package | Gitea issue |
|---|---|
| MB-01 scope, feasibility and contracts | [#28](http://192.168.1.15:3001/dpasare/2026_Networked_Intelligent_Lab_Orchestrator/issues/28) |
| MB-02 endpoints and shared physical leases | [#29](http://192.168.1.15:3001/dpasare/2026_Networked_Intelligent_Lab_Orchestrator/issues/29) |
| MB-03 runner and transports | [#30](http://192.168.1.15:3001/dpasare/2026_Networked_Intelligent_Lab_Orchestrator/issues/30) |
| MB-04 Core sessions, writes, API/CLI/MCP | [#31](http://192.168.1.15:3001/dpasare/2026_Networked_Intelligent_Lab_Orchestrator/issues/31) |
| MB-05 maps, codecs and workspace definitions | [#32](http://192.168.1.15:3001/dpasare/2026_Networked_Intelligent_Lab_Orchestrator/issues/32) |
| MB-06 administration and navigation | [#33](http://192.168.1.15:3001/dpasare/2026_Networked_Intelligent_Lab_Orchestrator/issues/33) |
| MB-07 live workspace, traffic and trends | [#34](http://192.168.1.15:3001/dpasare/2026_Networked_Intelligent_Lab_Orchestrator/issues/34) |
| MB-08 recordings, Find, exports and retention | [#35](http://192.168.1.15:3001/dpasare/2026_Networked_Intelligent_Lab_Orchestrator/issues/35) |
| MB-09 scans and advanced functions | [#36](http://192.168.1.15:3001/dpasare/2026_Networked_Intelligent_Lab_Orchestrator/issues/36) |
| MB-10 packaging, simulation and target acceptance | [#37](http://192.168.1.15:3001/dpasare/2026_Networked_Intelligent_Lab_Orchestrator/issues/37) |
