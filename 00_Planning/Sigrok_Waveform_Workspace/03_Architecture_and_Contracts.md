# Architecture and contracts

Proposed design, not a declaration of existing endpoints. Final names and wire schemas are to be settled in the implementation tasks.

## Service boundaries

```text
GUI waveform workspace     NILO CLI           MCP tools
          \                   |                  /
           Authenticated shared Bench Core services
           RBAC, policy, registry, jobs, events, artifacts
                 |                         |
        Plugin supervisor             Capture catalogue
                 |                    raw chunks + indexes
        Sigrok runner / worker              |
        libsigrok + libsigrokdecode ---- data/query services
                 |
        Supported local or network instruments
```

Keep generic lifecycle, jobs, leases, authorization and artifact retention in the NILO platform so future CAN, flashing and automation plugins can reuse them. Keep driver interpretation, acquisition packet normalization, decoder catalogs and waveform-specific processing in the Sigrok integration. The GUI plugin contributes routes and views through the plugin registry; generic navigation must not contain ATK-specific assumptions.

The existing Python runner and sigrok-cli implementation is a useful starting point. Its finite captures and parsed command output are not a sufficient long-term contract for complete capability discovery, analog data, continuous progress, trigger/frame events and structured decoding. SW-01 should prove a structured worker against libsigrok/libsigrokdecode, preferably a small native helper behind the existing process boundary. Evaluate supported bindings before selecting the language. Avoid binding the whole Go core to native device libraries merely to add one plugin.

Package/deploy the chosen worker, required libraries, decoder data, runtime assets, device permissions and their compatible versions together. The manifest reports available features and dependencies. A minimal appliance may ship fewer packages, but installed capabilities are discoverable and exclusions are explicit.

## Identity and data model

| Object | Identity and lifetime |
|---|---|
| Plugin | Stable plugin ID; installed version, enablement and configuration revision per appliance |
| Driver | libsigrok driver ID and deployed library identity; not the physical device |
| Device | Stable appliance inventory ID plus driver connection fingerprint; reconnect must detect changed hardware |
| Operation | Server transaction ID, client idempotency key, owner, request/configuration snapshot, state and result/error |
| Capture | Stable capture ID; raw data, device/configuration snapshot, segments, clock metadata, completeness and retention state |
| Analysis document | Its own ID/revision; channel presentation, markers, decoder graph, derived signals and view settings referencing a capture |
| Artifact | Immutable stored object with size/type/hash, producing operation and authorized access |

Human names are metadata, never resource addresses. A capture may have several analysis documents. Optimistic revisions prevent one browser or agent silently overwriting another's saved edits. Transient zoom/selection is client presentation state unless deliberately saved; this does not affect the appliance's active plugins.

## Capability contract

Discovery returns device class, channels and hardware channel groups, supported acquisition modes, connection options and typed configuration descriptors. Each descriptor carries:

- Stable key and description; type, units and scope (device/group/channel).
- Readable/writable/listable flags and current/default values when actually available.
- Allowed values or range/step, dependencies and a capability/configuration revision.
- Whether acquisition must be idle, and whether a change affects another channel or resource.
- Availability status with a reason: supported, unsupported, unknown, unavailable in this build or restricted by appliance policy.

Refresh dependent capabilities after configuration changes and reconnect. Generic controls cover recognized typed keys; semantic adapters handle special cases such as a samplerate range or trigger stages. Unknown keys are inventoried and classified, not silently passed through as arbitrary configuration commands. Explicitly block PWM/output-control operations at the registry/validation/worker boundary, including generic passthrough paths.

Expose hardware limits separately from NILO admission limits. A device that supports a high rate does not guarantee that the current appliance can store or stream that capture. Validate the entire acquisition configuration as one snapshot, acquire the lease, apply settings and read back effective values before starting. Driver rejection must produce a structured error tied to the field. Partial configuration application is not silently treated as success; record/recover the resulting state.

Trigger descriptors must express supported match types, sources, combinations/stages and pre-trigger constraints without inventing support. Hardware voltage thresholds retain their actual group scope. Analog-to-logic thresholds belong to a derived analysis node and must never modify hardware configuration.

## Durable commands, replies and concurrency

Reuse and extend the current durable job service instead of creating a parallel GUI-only queue. Its currently inspected limits (32 pending, two workers, same-device lease, bounded recent listing) are implementation facts, not final appliance capacity promises.

Command acceptance returns an operation ID promptly. A client-generated idempotency key, scoped to identity and operation family, identifies retries. Reusing a key with different parameters is a conflict. A server operation owns at most one terminal outcome; delayed replies are correlated with the operation ID and runner generation and cannot revive a timed-out operation.

Suggested lifecycle:

```text
accepted → queued / awaiting approval → starting → running → finalizing
                                                       ↘ completed
                                                       ↘ failed
                                                       ↘ cancelled
                                                       ↘ timed out
```

Capture-specific progress may say waiting for trigger or acquiring, but must distinguish a confirmed hardware state from an inferred worker phase. Preserve partial-result metadata independently of terminal status.

Admission and concurrency rules:

1. Bound queues by count and resource reservation. Reject excess work with a retryable busy response and useful limits. Schedule fairly across identities so an agent cannot monopolize the appliance.
2. Lease by physical resource, not driver name. Configuration and capture on one analyzer are exclusive; separate devices may run concurrently within appliance limits. Discovery that opens a device must participate in resource arbitration.
3. Read-only analysis can run concurrently against immutable captures, with bounded CPU/memory/I/O pools. Readers hold artifact references while deletion is pending.
4. Restart/disable/uninstall closes new admission and coordinates with pending/running work. An operator's ordinary restart cannot terminate someone else's capture. Forced administrative interruption is separately confirmed and audited.
5. Plugin runners have a generation/fencing token. After crash or replacement, quarantine uncertain hardware ownership until the old process is stopped and leases are reconciled; do not simply free the lock while the old process may still access USB.

Timeouts distinguish request acceptance/network timeout, queue/approval expiry, overall operation expiry and device inactivity where applicable. The user sees the effective overall limit before submission. Disconnecting a browser or losing an HTTP response does not cancel the operation. An overall deadline continues across reconnect/restart; use persisted wall-clock expiry plus monotonic elapsed timing within a process. Idle timeout must not kill a valid trigger wait or continuous mode merely because no data arrived; those phases need explicit policies.

Stop means request orderly acquisition end and finalize available data. Cancel means abandon the requested work with documented partial-data retention. Both are idempotent and subject to ownership/RBAC. At expiry, request termination, allow a bounded cleanup interval, then stop/quarantine an unresponsive worker. Never publish final artifacts until their metadata and completeness are consistent.

Durable event/reply envelopes include operation ID, event sequence, state revision, phase/progress, timestamps, result artifact/capture references and structured error. GUI event streams and CLI/MCP polling consume the same state. Reconnect resumes after a sequence watermark; if retained events no longer cover it, return a fresh snapshot and explicit gap. Enforce authorization on event subscription and replay. Bound event history and progress frequency; raw samples do not travel through the command/reply queue.

## Waveform storage, queries and rendering

Store raw acquired data immutably in chunks with channel metadata, segment boundaries and clock information. Preserve original precision, analog units/scaling and acquisition trigger position. Use 64-bit sample positions and exact/rational time relationships where possible. Serialize large integer indices safely for JavaScript, for example decimal strings, rather than truncating beyond its exact integer range.

Build reusable indexes for viewport queries. Fetch only visible channels and sample ranges at a requested resolution. Digital summaries preserve evidence of transitions/glitches instead of selecting one sample per pixel; analog summaries use min/max envelopes. At detailed zoom return the original values/edges. Measurements and decoders operate on raw data, not display decimation. Mark timing gaps and incomplete extents explicitly.

Prototype Canvas 2D with worker-based parsing/render preparation and virtualized channel rows. Measure before introducing WebGL. The browser maintains bounded tile caches and cancels obsolete viewport requests. Each response carries capture/analysis revision and request generation so a slow response cannot replace the current view. Bulk data uses binary/range retrieval or downloadable artifacts with normal authorization; CLI/MCP return bounded summaries and references rather than millions of samples in tool text.

Continuous acquisition needs streaming-to-disk, reservations, backpressure and explicit storage/duration limits. A rolling buffer is a distinct mode with visible data loss/retention semantics, not an implementation shortcut that silently overwrites samples.

## Decoding, measurements, derived signals and search

The decoder catalog comes from the installed libsigrokdecode runtime. Describe required/optional channels, typed options, input/output protocols, annotation rows/classes and binary output classes. Persist a decoder graph with explicit connections, validate missing mappings and incompatible stacks, and include runtime versions in result provenance.

Annotation records contain sample start/end, decoder/node ID, class/row and text variants. An annotation string is not automatically a structured SPI transaction; protocol-specific semantic output requires a separately verified adapter. Binary output stays typed by class, with export support. Streaming decoders preserve state across chunks and restart from a valid checkpoint or full input when needed; arbitrary viewport slicing must not reset protocol state incorrectly.

Search modes include raw transitions/pulse widths/digital patterns, annotation text/classes/ranges and decoded payload bytes. Return paginated matches with stable ordering, range and navigation references. Define boundary behavior, tolerance, overlapping matches and clock requirements. Bound expensive regular expressions or initially support literal queries; do not allow unbounded work through MCP.

Derived analog-to-logic and math nodes preserve source references, parameters, algorithm versions and timing alignment. Use a restricted expression engine, not arbitrary uploaded executable code. Derived filters must describe delay/resampling/gaps. Measurements record method, units, interval and quality; 1/Δt is not labeled measured frequency without the correct interpretation.

## Persistence, retention and file compatibility

Save associates a human name and retained status with a capture and analysis revision. Temporary captures expire under appliance policy; pinned captures require deliberate deletion. Record byte quotas, reserve space before capture/import, finalize atomically, and recover incomplete writes after restart. Paginate library and operation history independently. Security audit retention is independent and must not be erased by deleting a waveform.

Deletion is authorized and confirmed, idempotent and dependency-aware. Use tombstones/reference tracking so files are reclaimed only when active operations release them. Deleting raw data must not leave an apparently usable analysis document; offer a clear cascade or reject with dependencies. Define the grace period and actual byte reclamation in SW-08.

Support `.sr` as an interoperability target and a versioned NILO analysis document for names/presentation/markers/decoder graph. PulseView `.pvs` is separate setup metadata: import/export only an explicitly tested subset initially, report unsupported settings, and never label NILO JSON as `.pvs`. Preserve source files on import. Catalog other input/output formats and their options from the installed runtime; distinguish raw export from transformed/annotation/binary export.

## GUI / CLI / MCP semantic coverage

These are operation families, not final spellings. Every mutating operation uses the common authorization and job rules.

| Family | GUI | CLI / MCP |
|---|---|---|
| Plugin list/status/install/enable/disable/restart/uninstall | System pages and confirmations | Structured lifecycle commands; explicit destructive intent and matching RBAC |
| Driver/device discovery, connection, capabilities | Device selector/settings | Catalog/query/discover operations with typed results |
| Validate configuration, capture, stop/cancel | Toolbar and progress | Submit/wait/status/cancel with transaction IDs and effective settings |
| Operation events/results | Operations drawer | Poll/resume events; bounded waiting with ID preserved on client timeout |
| Capture list/get/name/save/delete/import/export | Library and dialogs | Structured metadata commands and artifact references; no implicit data destruction |
| Decoder catalog/configure/run/results/binary | Decoder tracks and table | Graph configuration, jobs and paginated typed output |
| Measurement/search/derived transform | Analysis tools | Same range/method/query parameters and bounded result sets |
| Markers and persistent channel labels | Direct manipulation + numeric fields | Get/set named semantic metadata and revisions |
| Zoom/pan/hover/drag/theme/drawer layout | Direct browser interaction | No mandatory command mirror; optional saved-view range is sufficient |

MCP destructive operations must require explicit, attributable user intent through a supported confirmation contract; a tool must not assume the GUI's confirmation happened. CLI can use an explicit confirmation option for deliberate scripting. No interface may bypass appliance approval or role checks by directly launching sigrok tools.
