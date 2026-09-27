# USB to Serial plugin implementation plan

Status: proposed implementation baseline, 2026-09-27.
Planning is performed without AEGIS, as requested. Implementation will use AEGIS.
No hardware is currently connected. Deployment to the Raspberry Pi and physical
acceptance will start only when the user requests deployment and provides the adapter.

## 1. Confirmed scope

- Add a USB to Serial plugin using the Camera administration and device-navigation
  patterns, with a terminal adapted to serial communication.
- Administrator page: plugin Enable/Disable, approval policy, Discover ports,
  per-device names and enablement, version/update controls and View activity.
- Each configured, enabled port has RealTime View and Sessions destinations.
- Configure baud rate, parity, data bits, stop bits and flow control. Receive text
  or hex, show line timestamps, send ASCII or hex, and save sessions.
- Ask policy authorizes a connection session, including its transmissions;
  individual messages within that approved session do not request new approval.
- Retention defaults per device: 100 saved logs, 2 GiB total, 100 MiB per log file,
  automatic rotation and oldest-completed-file removal when needed.
- Adapters attach to the NILO appliance. Windows COM ports attached to a browser
  computer are outside the first delivery.
- GUI, CLI and the shared NILO MCP server use the same Core operations.

The limits and session approval above are user decisions. Other numeric bounds
below are proposed engineering defaults to validate during implementation.

## 2. Existing architecture and required changes

Observed baseline: project 0.10.0, commit `d2bb8be`.

| Existing component | Reuse / necessary change |
|---|---|
| `05_Software/core/internal/bench/adapter.go` | Register a `serial` family with explicit operations, schemas, resource ownership and runner preparation. |
| `internal/bench/managed_devices.go`, `packages.go`, `manager.go` | Generalize Camera-specific device lookup, readiness and busy hooks narrowly; retain Camera behavior. |
| `internal/store/sqlite` managed-device repositories | Reuse stable persisted records; add serial connection/log metadata and schema migrations as needed. |
| `internal/bench/camera_workspace.go` | Reference for owned long-lived sessions and unconditional stop; serial sessions get their own bounded transport and authorization. |
| `05_Software/integrations/sdk/python` | Reuse the supervised runner protocol, configuration, cancellation and errors. |
| Frontend shell, Camera administration and workspace | Reuse navigation-only groups, per-device destinations, split sizing, collapsible sections and feedback banners. |
| Core HTTP, CLI and MCP | Add serial operations with shared validation and permissions; retain existing Camera and Sigrok contracts. |
| `02_Tools` build/packaging/test tooling | Add serial integration discovery, locked dependencies, bundle validation and target permissions. |

No USB terminal implementation was found in the current integration families.
The existing Camera identity requirement is a useful design reference, but its
capture-endpoint wording needs a serial-specific requirement during implementation.

## 3. GUI

### System → Plugins → USB to Serial

Administrator-only page (`plugins.manage`), matching Camera:

- Header: USB to Serial plugin, View activity, current version and Update plugin.
  Update is enabled only for a newer verified local bundle.
- One frame with a wrapping row: Enable, Disable, inline approval policy and
  Discover ports. Actions reflect the current plugin state.
- Device rows in that frame: user name, port description, supported identity
  details when useful, Save, Enable and Disable.
- Discovery observes ports without opening them or changing modem lines.
- Connected disabled/unconfigured ports remain available for administration.
  Absent saved records remain stored for stable identity; explain reappearance
  and failures without cluttering healthy rows with internal readiness labels.
- No adapters yields a clear empty state, not generated devices.
- Plugin/device disable closes owned connections and finalizes active logs;
  do not silently leave hardware operating behind a disabled menu entry.

### Instruments → USB to Serial → named port

USB to Serial is an expandable group with no redundant overview destination.
Names come from administrator configuration; stable IDs are `serial-N`.

#### RealTime View

Header and wrapping toolbar share a row: device name, connection state,
Connect/Disconnect, Start/Stop logging, Fill workspace and Full screen.
Show recording state, byte counters and lease expiry where applicable.

The central area is a bounded terminal viewport with a compact transmit row
below it. A local display toolbar provides Clear display, Pause display,
auto-scroll, text/hex, timestamp visibility, RX/TX filtering and Find.
Pause display and Clear display do not stop acquisition or erase saved logs.
Scrolling away from the tail pauses auto-scroll until explicitly resumed.

Collapsible configuration sections:

| Section | Controls |
|---|---|
| Port settings | Baud selector plus bounded custom value; 5–8 data bits; parity none/odd/even/mark/space; stop bits 1/1.5/2; flow none/RTS-CTS/XON-XOFF; initial RTS/DTR. |
| Display | ASCII or UTF-8 text / hex, timestamps, word wrap, bounded history and visual RX/TX distinction. |
| Transmit and signals | ASCII/UTF-8 or hex input, append nothing/CR/LF/CRLF, local echo, bounded command history, RTS/DTR and CTS/DSR/RI/CD indicators where supported. |

Default port configuration: 115200 baud, 8 data bits, no parity, 1 stop bit,
no flow control. Unsupported settings return a specific error; the GUI must
not claim a device supports every selectable setting. Apply port changes while
disconnected; reconnect under a fresh session approval. Display changes are local.

Text display escapes unprintable bytes and handles split UTF-8 sequences without
modifying stored bytes. Hex view groups bytes with offsets. Line timestamps use
the first received byte/chunk belonging to that displayed line; partial lines
appear promptly and are bounded even when a device never emits a newline.
Terminal content is rendered as text, with no HTML or terminal escape execution.

The transmit row includes input, input encoding, line ending and Send. Validate
the full payload before transmission. Hex accepts complete byte pairs with
whitespace separators; reject malformed or odd-length input before any write.
Apply the selected line ending exactly once to the encoded bytes. Show sent,
partial, failed or delivery-unknown status; local echo is a TX display choice,
not a received acknowledgement from the target.

Manual RTS/DTR is unavailable while the corresponding flow-control mode owns
the line. Unsupported modem signals show unavailable, rather than a false low
value. Opening a serial device can affect control lines; include the selected
initial state in the connection summary and approval description.

#### Sessions

- Table/list of saved logs with device name, start/end, status, duration,
  RX/TX byte counts and disk size. Serial data uses a text preview rather than
  image thumbnails.
- Paginated saved-log viewer supporting text/hex, direction and timestamps.
- Export structured JSONL, readable text, RX bytes or TX bytes. Direction-specific
  raw exports avoid implying that a mixed RX/TX stream is one received message.
- Delete one or selected completed/incomplete logs through the existing
  confirmation pattern. Refuse deletion of active recordings or leased downloads.
- Administrator retention controls; ordinary owners may view/export/delete
  their authorized logs. Metadata changes must preserve ownership restrictions.

The same responsive rules apply as Camera: minimum usable controls, wrapping
toolbars, collapsing sections, and an internally scrolling terminal. Verify
800×480, 1024×600 and desktop; also check 320-pixel reflow without page overflow.

## 4. Physical discovery and runner

Proposed integration: `05_Software/integrations/serial/`, using the existing
Python integration SDK and a pinned pySerial dependency. Pin the exact archive,
hash and license in the lock/bundle process during implementation.

Enumerate USB-backed Linux tty endpoints using USB metadata and validated sysfs
ancestry. Include USB CDC ACM devices as well as USB serial converters. Exclude
on-board UARTs, console ttys, arbitrary user paths and serial-over-network URLs.
Show the actual Linux endpoint as supporting detail; public calls use `serial-N`.

Resolve identity by trustworthy USB vendor/product/serial plus interface/port
identity. Multiport adapters need a separate stable record for each port. Use a
persistent physical-port fallback for missing/duplicate serials; moving such an
adapter must not silently grant an old record's authority to different hardware.
Re-check the endpoint and physical identity immediately before opening it.

Use one supervised runner per connected port, with finite read/write timeouts,
exclusive Linux opening and Core resource ownership. pySerial's exclusive option
alone does not prove exclusion against every foreign application: verify Linux
exclusive tty handling and the external-busy failure path separately.
The service receives only the required serial-device permissions through the
appliance packaging/udev boundary; do not run the integration as root.

Official implementation references:

- [pySerial API](https://pyserial.readthedocs.io/en/latest/pyserial_api.html):
  port parameters, bounded I/O, exclusive mode and modem-line caveats.
- [Port enumeration](https://pyserial.readthedocs.io/en/latest/tools.html):
  USB identity and interface metadata from `serial.tools.list_ports`.

## 5. Session authorization, lifecycle and transmission

### Session lease

Connect is a finite Bench job subject to current plugin policy. Approval binds
the actor, credential, physical device identity, device/configuration revisions,
port settings, initial control-line state, allowed actions and expiry.
Once approved, the resulting Core-owned lease permits RX, bounded sends,
logging and declared line changes for that connection without repeated approval.
No client-provided `approved` flag or guessed session ID grants this authority.

Initial connection limit: 60 minutes, displayed before connection and in the
workspace. Expiry closes the port and finalizes logging. Renewing requires a
new connection authorization. This duration is distinct from Bench's short
command queue/deadline and from recording file rotation.

- Revalidate actor, credential, permissions, plugin/device enablement and lease
  on every mutating API call; detect revocation during background receive/logging
  within a bounded interval (initial target: 1 second).
- A second controller cannot transmit through another session. A second tab
  with the same authorized credential can attach explicitly and shares ordering.
- Owner or administrator can disconnect/stop without a fresh operation approval.
  Administrator inspection does not transfer the original owner's sending lease.
- On unplug or service restart, close the session and retain recoverable logs.
  Reappearance changes availability; reconnect is explicit and requires new
  authorization. Never replay previous TX or automatically resume a stale lease.
- Browser disconnection does not stop an active log; it continues within the
  lease and quotas. An unrecorded session with no attached reader/controller
  closes after 60 seconds. Renewing a reader attachment is authenticated.
- Initial ceiling: four serial connections per appliance. Long-running readers
  must not occupy the finite Bench workers needed by Camera or other devices.

### Send semantics

Serialize TX per port and bound payloads to 4 KiB per send, with a 64 KiB queue
and finite write timeout (initial target: 2 seconds). Reject queue overflow
explicitly. Reserve per-request status before writing; idempotent retries return
the original result rather than transmitting again. A restart/timeout after an
uncertain partial write returns delivery-unknown and never auto-resends.
Report bytes accepted by the OS separately from acknowledgement by the target;
NILO cannot establish target execution from a successful serial write alone.

Use `bench.read`, `bench.execute`, `bench.approve` and `plugins.manage` with the
existing ownership model. Settings that enlarge approved hardware authority
require disconnect/reapproval. Do not weaken approval rules for other plugins.
Activity records IDs, operation metadata, byte counts and outcomes; raw serial
payload belongs only in the explicitly retained log, not general audit records.

## 6. Streaming, recording and quotas

Core owns storage paths and client access. The runner uses a private bounded
channel to supply byte events; stdout remains the integration protocol channel.
The public reader API uses cursors and bounded long-poll responses, which also
work in CLI and MCP. A cursor gap reports omitted display data explicitly.

Proposed bounds: 4 KiB event chunks, 1 MiB live ring per connection, responses
up to 64 KiB and long-poll waits up to 10 seconds. GUI history is capped by
both bytes (2 MiB) and rows (10,000); DOM rendering is virtualized. Slow clients
cannot grow server memory or block physical reception indefinitely.

Canonical saved format: versioned JSONL header and ordered events containing
sequence, UTC timestamp, monotonic elapsed time, direction, byte count and
base64 bytes, with separate state/gap/error events. Chunk timestamps describe
host observation, not electrical per-byte timing. Record port configuration and
device identity in the header; do not persist credentials.

Connection sessions and saved logs have separate IDs. Starting logging captures
future RX/TX until stopped. A connection can produce multiple logs; rotation
creates another library entry linked to the same connection. **The 100-entry
limit counts retained log files, including rotated parts**, so long connections
cannot produce an unbounded collection of hidden segments.

| Retention rule | Default / behavior |
|---|---|
| Retained completed log files per device | 100 |
| Total per-device storage | 2 GiB, counting active, completed and incomplete files |
| Individual file | 100 MiB on disk, including structured encoding/header overhead |
| Rotation | Before an event would exceed the file limit, finalize the current part and open the next. |
| Cleanup | Remove oldest eligible completed files during admission/rotation; protect active, incomplete and download-leased files. |
| Disk reserve | Reuse the appliance rule: at least 2 GiB or 10% of the storage filesystem, whichever is larger. |

Reserve storage before accepting a recording segment; account for concurrent
ports and pending writes. All owners' logs count toward a device quota; the
administrator's policy permits automatic pruning across owners. Ordinary reads
and deletes still enforce owner/admin access. Export generation must be streamed
or bounded, without producing an unlimited second on-disk copy.

If space cannot be reclaimed, stop logging and show a persistent Warning;
the terminal connection may continue within its lease. Never silently pretend
that acquisition is being recorded after a disk or writer failure. Bounded
ingestion overflow produces an explicit gap/error and incomplete recording.
On crash recovery, validate complete records, retain the valid prefix and mark
the log incomplete. Do not silently label a truncated log completed.

## 7. Public interfaces and source boundaries

Proposed HTTP namespace: `/api/v1/serial-devices/{id}` for status/connect/read/
send/disconnect/logging, and `/api/v1/serial-logs/{id}` for authorized viewing,
exports and deletion. Reuse generic plugin/device lifecycle/configuration routes.
Exact schemas will be specified under AEGIS before implementation.

`nilo serial` will provide status, connect, read/follow, send, disconnect,
logging start/stop, logs, export/delete and retention. Credentials continue to
use token files. Transmit payload files/stdin avoid requiring message contents
in shell history; strict ASCII/UTF-8/hex options are explicit.

MCP will expose corresponding finite tools through Core's shared `/mcp` server,
including bounded cursor reads and session-authorized send. Provide a direct
workflow for each user operation instead of relying exclusively on an
undocumented generic operation. Large exports use authenticated HTTP/CLI;
MCP returns authorized metadata/export instructions, not unbounded log content.

Add `06_SoftwareDocu/USB_to_Serial_Integration.md`, extend the CLI and MCP
references, and add serial component knowledge when implementation exists.
Keep proposals in this planning document until then. The project has one release
version; no independent product version is introduced for this plugin.

## 8. Implementation work packages using AEGIS

| Order | Deliverable | Verification required |
|---|---|---|
| 1 | Frame the implementation from this plan; specify testable serial requirements, session authority and schemas; record technical design. | Trace UI/API actions to permissions, lease bounds, identity, storage and failure behavior. |
| 2 | Serial integration skeleton, dependency lock, discovery, identity reconciliation and offline bundle support; narrow device hooks. | Empty discovery; multiport/duplicate/missing serial metadata; schema/bundle rejection; Camera regression checks. |
| 3 | Owned connection lifecycle, approval-bound lease, bounded RX/TX and control-line capability handling. | Linux pseudo-terminal exchange; busy ports; concurrent owners; expiry/revocation; timeouts; unplug-like closure; TX deduplication. |
| 4 | Persistent logs, rotation/retention, recovery, pagination and exports. | Byte-exact RX/TX, rotation boundaries, count/space limits, download leases, disk-full handling and truncated-file recovery. |
| 5 | Administration and terminal/Sessions UI, CLI and MCP, translations and documentation. | Shared service behavior; Unicode/hex and line-ending cases; keyboard access; responsive terminal and GUI component tests. |
| 6 | Integrated validation and review. | Required repository suites and configured review; report absent physical acceptance separately; prepare the candidate for later deployment. |

Follow the applicable AEGIS implementation stages and configured review rules;
the planning phase creates no AEGIS task, requirements or stage transitions.
Record implementation verification in the task journal and linked tests; this
plan introduces no new `.agentic/evidence` folder requirement. Commit, push,
version assignment and deployment remain separate user-directed operations.

## 9. Acceptance without and with hardware

Without an adapter, use Linux pseudo-terminal pairs to exercise actual serial
I/O and controlled peers. Test-only injected metadata/failure seams may cover
USB discovery and error paths, but do not introduce a fake production plugin,
generated instrument data or a user-visible simulation mode for this feature.

Automated checks must cover split CR/LF and UTF-8, arbitrary binary including
NUL, no-newline streams, malformed hex, high-rate receive, partial/unknown sends,
slow readers, disconnect and reconnect, cross-owner denial, expired/revoked
credentials, stale device/configuration revisions and retention concurrency.
Pseudo-terminals do not establish USB discovery, baud timing, electrical signal
behavior or modem-line support on the eventual adapter.

After the user requests Pi deployment with an adapter connected, identify its
driver/USB identity, verify discovery/naming, open/close, receive/transmit with
an agreed loopback or known peer, supported serial settings, logging/export,
unplug/replug and bounded sustained load. Verify control lines only on supported
hardware with appropriate wiring. Save a rollback backup and preserve existing
Camera/Sigrok settings and retained data during deployment.

Initial exclusions: browser-PC ports, TCP/RFC2217 bridges, firmware upload,
protocol-specific Modbus/SCPI interpretation, scripting/macros, automatic
periodic transmission, ANSI terminal emulation and RS-485 direction control.
