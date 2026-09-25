# Implementation work packages

These packages are planning units for later implementation tasks, not automatically created Codex tasks or AEGIS journals. None is marked complete. Keep each task's GUI, service and automation behavior consistent; CLI/MCP parity must not be deferred to an unrelated final rewrite.

## Existing implementation gap

The inspected implementation provides useful plugin supervision, durable operations, device leases and finite capture artifacts. It does not yet provide the requested waveform application.

| Current location, relative to repository root | Observed gap to revisit |
|---|---|
| `05_Software/frontend/src/components/pages/PluginsPage.tsx` | Lifecycle, capture form and transaction listing combined; fixed channel/rate choices and a small SVG sample preview rather than an interactive waveform workspace |
| `05_Software/integrations/sigrok/src/nilo_sigrok/plugin.py` | ATK-specific scan parsing, 16 digital channels, limited rate choices, finite capture limits and a first-256-sample preview; insufficient generic analog/trigger/decoder model |
| `05_Software/core/internal/bench/parameters.go` | Fixed acquisition/driver assumptions and narrow decoder parameters; extend through typed schemas rather than arbitrary unvalidated options |
| `05_Software/core/internal/bench/model.go` | Operational records exist; capture library and analysis-document identities/names/revisions need design |
| `05_Software/core/internal/bench/packages.go` | Restart currently uses general plugin management permission; discovery and lifecycle need consistent busy/resource semantics |
| `05_Software/core/internal/bench/jobs.go` | Queue and device lease foundations exist; history is bounded but lacks the proposed independent capture library, pagination and retention/deletion flow |
| `05_Software/core/internal/auth/role.go` | Add narrowly scoped restart and any approved capture/document permissions without weakening existing token/role checks |

Re-read these files when each task begins; repository changes after this research may alter the baseline. Preserve existing contracts through explicit versioning/migrations as needed.

## Dependency overview

```text
SW-01 worker/capability feasibility
  ├─ SW-02 device and capability contracts ─ SW-03 acquisition/trigger jobs
  └─ SW-04 capture storage and data queries
SW-05 navigation/lifecycle ─┐
SW-03 + SW-04 ──────────── SW-06 waveform rendering ─ SW-07 channels
SW-04 + SW-05 ──────────── SW-08 saved waveform library
SW-06 + SW-08 ──────────── SW-09 cursors/markers/measurements
SW-01 + SW-04 + SW-07 ─── SW-10 decoders ─ SW-11 tables/binary
SW-09 + SW-11 ──────────── SW-12 search/filtering
SW-04 + SW-07 + SW-10 ─── SW-13 derived analog/math signals
SW-08 + SW-10 + SW-13 ─── SW-14 interoperability/export
All relevant packages ─── SW-15 packaged appliance integration and acceptance
```

Some development can overlap after shared contracts are stable. Every package has a demonstrable result; the final acceptance package integrates prior evidence rather than postponing all testing until the end.

## SW-01 — Prove the structured sigrok worker and record coverage

**Depends on:** none. **Areas:** integration runner, native helper boundary, deployed stack manifests.

Build a small feasibility slice using the deployed libsigrok/libsigrokdecode APIs or supported bindings. Demonstrate driver/device metadata, typed settings, digital and analog datafeed packets, trigger/frame events where available, and structured decoder output. Compare this with the existing sigrok-cli bridge and select the implementation boundary. Inventory waveform-related features, unsupported device classes, experimental build options and packaging dependencies.

**Acceptance:** machine-readable capability and data examples from Demo and representative fixtures; one decoder produces annotations without parsing console prose; worker crashes remain isolated from Bench Core. Document versions and constraints. No ATK-only schema and no PWM/output passthrough. Choose the worker language and protocol from evidence.

## SW-02 — General device discovery and capability contracts

**Depends on:** SW-01. **Areas:** integration, bench validation/registry, API/CLI/MCP types.

Implement driver catalogs, device identity, typed explicit connections, USB/serial/network discovery where supported, channel/group metadata, units/ranges/enums and dependent capability refresh. Replace current hardcoded ATK channels/rates with runtime descriptions. Distinguish device limitations from appliance policy limits and connection failure from unsupported capability.

**Acceptance:** digital-only, analog-only and mixed fixtures render valid forms; a shared threshold affects the correct group; invalid/stale settings are rejected server-side. Discover has an operation ID and bounded duration. Two clients cannot scan/open a leased device unsafely. GUI/CLI/MCP receive equivalent descriptors.

## SW-03 — Acquisition, triggers and asynchronous operation guarantees

**Depends on:** SW-02; coordinate result schema with SW-04. **Areas:** jobs, leases, worker protocol, acquisition APIs.

Add supported edge/level trigger configuration, input threshold, pre-trigger ratio, finite/frame/continuous limits and effective-setting readback. Extend queue/reply behavior for progress, idempotent retries, stop versus cancel, timeout phases, delayed replies and reconnection. Fence replaced workers and make lifecycle interruption coherent.

**Acceptance:** concurrent requests to one device serialize; independent devices progress within limits; a retried request creates one operation; stale replies cannot change terminal state. Queue expiry, trigger wait expiry, disconnect, worker crash and stop/cancel each have a deterministic result and partial-data policy. Unsupported trigger combinations are never silently accepted. PWM controls remain rejected through all routes.

## SW-04 — Capture model, raw storage and viewport queries

**Depends on:** SW-01; integrates SW-03. **Areas:** artifact storage/catalog, integration datafeed, data retrieval.

Introduce captures/segments/channel metadata, immutable chunks, safe large sample indices, clocks and completeness. Add disk reservations, raw range retrieval, digital transition summaries, analog envelopes and incremental indexing. Keep waveform bulk data separate from operation events.

**Acceptance:** large synthetic digital/mixed captures can be queried without loading the whole recording in browser or server memory. A narrow pulse remains represented in a zoomed-out summary and resolves exactly at full zoom. Unknown sample rate and discontinuous frames are shown honestly. Restart during finalization produces a recoverable partial or complete record, never a misleading complete capture.

## SW-05 — Appliance plugin navigation and administration

**Depends on:** stable registry contracts; can begin alongside SW-02. **Areas:** navigation, routing, lifecycle permissions, localization.

Create installed plugin children under System → Plugins and enabled plugin children under Bench. Split administration from instrument use, introduce operator restart permission, keep admin enable/disable/uninstall, add uninstall confirmation and authorized Activity links. Handle busy operations, disabled/faulted states, open tabs and navigation updates across clients. Preserve project versioning and dependency-version distinction.

**Acceptance:** two identities see the same appliance enablement; operator restart is permitted but management actions are rejected by the API; uninstall removes its submenu after success. Failed/busy lifecycle actions leave coherent state. EN/DE/RO and theme/help conventions are present. Bundled installation honors the selected enablement/uninstall policy after deployment and restart.

## SW-06 — Interactive digital and analog waveform viewport

**Depends on:** SW-03, SW-04, SW-05. **Areas:** new Sigrok workspace, rendering worker, query client.

Deliver the primary waveform layout, synchronized ruler/rail, digital and analog traces, zoom/fit/pan, live progress, follow mode, trigger position and frame selection. Support stale-request cancellation and bounded caches. Keep viewport controls usable with keyboard and accessible summaries.

**Acceptance:** open/capture/inspect works on one screen at the agreed desktop baseline; dense and detailed views remain accurate. Panning away during capture stops follow without stopping acquisition; Resume follow works. Delayed responses cannot jump back to an old capture or zoom. Measure rendering, query latency and memory with representative large recordings.

## SW-07 — Channel presentation, groups and device settings UX

**Depends on:** SW-02, SW-06. **Areas:** channel rail, capability form components, saved presentation schema.

Add label/color/height/order/group controls, bulk visibility actions and analog scale/divisions/autoranging. Make acquisition enablement distinct from visibility. Build compact device/trigger settings panels with tooltips and meaningful unsupported explanations. Preserve channel IDs beneath names and colors.

**Acceptance:** hiding/reordering/renaming channels does not alter raw acquisition identity or break decoder mappings. Shared hardware settings remain correctly scoped. All drag-based operations have keyboard/numeric alternatives; chosen colors remain readable in both themes. Multiple views may inspect one capture without duplicating it.

## SW-08 — Named waveforms, saved analysis and retention

**Depends on:** SW-04, SW-05; presentation schema from SW-07. **Areas:** database/migration, library API, save/open UI.

Create saved capture and analysis-document records, names/notes/labels, optimistic revisions, explicit ownership/sharing, paginated library, temporary-versus-pinned retention, quotas and confirmed dependency-aware deletion. Add a compact paginated Operations drawer with result links and useful timeout detail; remove technical UUID/deadline prominence from daily use.

**Acceptance:** save “SPI memory read — board rev B”, reconnect, find and reopen it; rename keeps the ID; simultaneous edits produce a conflict instead of silent overwrite. Deletion reclaims bytes when references are released and preserves audit. Pinned captures survive cleanup, disable and uninstall under the documented policy. Unauthorized lists/downloads/edits/deletes fail consistently across interfaces.

## SW-09 — Cursors, markers and measurements

**Depends on:** SW-06, SW-08. **Areas:** timeline interactions, numeric measurement service, metadata APIs.

Add cursor pairs, named markers, interval/sample readouts, edge navigation and basic pulse/period/duty measurements with explicit methods. Persist user markers separately from immutable trigger metadata. Provide semantic range/measurement/marker CLI and MCP operations.

**Acceptance:** known fixture edges give correct sample distance and time within defined precision; unknown-clock input omits time/frequency claims. Save/reopen preserves markers and labels. Cursor dragging and numeric entry agree. Cross-frame measurement is rejected or explained when timing continuity is unknown.

## SW-10 — Full decoder catalog, graphs and annotation tracks

**Depends on:** SW-01, SW-04, SW-07. **Areas:** decoder worker, graph schema, configuration UI, aligned tracks.

Expose installed decoders, typed options and channel mappings; implement compatible stacks, annotation rows/classes, progress/errors and decode provenance. Support selected intervals with correct decoder state handling, not naive viewport-only resets. Present warnings for missing channels or required sample rate.

**Acceptance:** UART, SPI, I²C and at least one stacked/measurement-oriented decoder work from common fixtures with expected annotations. Another installed decoder can be configured without adding protocol-specific form code. Catalog limitations are explicit. Changing options invalidates/recomputes the right results. Automation receives stable, paginated annotation records.

## SW-11 — Decoder table and binary output

**Depends on:** SW-10, SW-06. **Areas:** result queries, bottom drawer, artifact export.

Implement synchronized annotation table, row/class visibility controls, range selection, complete text details and binary/hexdump views by output class. Link table selections to waveform intervals and preserve scroll/filter state. Export supported annotation and binary results with clear format names.

**Acceptance:** selecting a table row locates the exact waveform interval; selecting an annotation identifies its table result. Large result sets remain bounded. Binary bytes round-trip through export exactly; arbitrary annotation strings are not misrepresented as typed protocol payloads.

## SW-12 — Waveform search and filtering

**Depends on:** SW-09, SW-11. **Areas:** indexes/query jobs, search UI, CLI/MCP queries.

Implement transition/pulse/digital pattern search, annotation text/class/range search and binary payload search, with Previous/Next and paginated result lists. Add visibility filters such as changed channels and distinguish them from transformations. Define tolerances, interval boundaries, cancellation and resource budgets.

**Acceptance:** fixtures cover overlapping matches, boundary matches, empty results and very large match counts. GUI and command results agree. Search does not mutate raw data. Slow queries can be cancelled and do not block acquisition; stale results do not replace a changed query.

## SW-13 — Analog conversion, derived filters and math traces

**Depends on:** SW-04, SW-07, SW-10. **Areas:** derived-signal engine, graph model, analog UI.

Add threshold/Schmitt analog-to-logic conversion, analog/converted/both display and decoder input from derived logic. Add bounded math expressions and explicitly cataloged transforms with source/parameter/version provenance. Make resampling, latency and invalid-value behavior visible and reproducible.

**Acceptance:** known analog inputs produce expected threshold crossings, hysteresis and derived values; conversions leave hardware settings unchanged. Cycles, invalid expressions and excessive computation are rejected. Reopened analysis and CLI/MCP execution reproduce the same derived results.

## SW-14 — File interoperability and export completeness

**Depends on:** SW-08, SW-10, SW-13. **Areas:** format catalog, import/export workers, compatibility fixtures.

Support `.sr`, versioned NILO analysis documents and an explicitly tested `.pvs` setup subset. Expose installed input/output modules and options. Provide raw/range, annotation, binary and supported transformed exports; retain original inputs and describe metadata that a chosen format cannot preserve.

**Acceptance:** load PulseView captures into NILO and open supported NILO `.sr` exports in PulseView; verify samples/channel metadata, not just successful file opening. Test labels/decoders/markers separately for each setup format. Unsupported `.pvs` fields are reported. Malformed/oversized imports fail with bounded resources and no partial visible library entry; export retries do not create misleading duplicate results.

## SW-15 — Integrated appliance delivery and acceptance

**Depends on:** prior packages relevant to the delivery milestone. **Areas:** package manifest/install scripts, appliance runtime, integration acceptance, user help.

Package the GUI assets, runner/native worker, libraries, decoders and permissions for supported NILO editions. Verify bundled deployment and later install/upgrade/disable/restart/uninstall with retained captures and recoverable state. Run the cross-interface/RBAC/concurrency matrix, accessibility/theme/language checks and browser/Pi performance measurements. Publish the supported feature/device/build coverage register and user-facing help.

**Acceptance:** the authorized Raspberry Pi deployment can complete the named-capture workflow from GUI and equivalent CLI/MCP operations; ATK provides one hardware path and Demo/import fixtures cover broader features. Separate NILO integration acceptance from actual trigger timing/electrical correctness and from unsupported physical device claims. The full inventory has implemented, unsupported-by-device/build, or explicitly deferred status with evidence; intermediate preview-only behavior is not called full parity.

## Suggested delivery checkpoints

| Checkpoint | Demonstrable result |
|---|---|
| A: contracts and appliance flow | SW-01–05: generic capabilities, reliable operations, storage/query foundation and correct navigation/RBAC |
| B: useful acquisition workspace | SW-06–09: interactive digital/analog display, named captures, channel editing, markers and measurements |
| C: protocol analysis | SW-10–12: decoder stacks, tables/binary output and waveform search |
| D: full planned analysis and interoperability | SW-13–15: derived signals, formats/setup compatibility and integrated appliance acceptance |

Each checkpoint can be made reviewable without redefining the final target as its currently implemented subset. Electrical driver acceptance may use later dedicated hardware tasks as the user requested.
