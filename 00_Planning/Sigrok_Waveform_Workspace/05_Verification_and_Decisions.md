# Verification and decisions

This file describes future acceptance evidence. The research itself did not implement or run NILO tests, deploy a new build, or validate hardware electrical behavior.

## Test coverage to carry between tasks

| Dimension | Required cases and reason |
|---|---|
| Device diversity | Demo plus digital, analog and mixed fixtures; channels other than 16; shared hardware groups; enumerated and ranged settings; unknown/unsupported values |
| Real hardware | ATK on the authorized Pi for acquisition integration; later representative non-ATK/analog hardware for device claims that fixtures cannot prove |
| Connections | USB reconnect/identity change, explicit serial/network connection where supported, unreachable target, busy device and bounded discovery |
| Triggers | Every advertised match type, supported combinations, pre-trigger limits, timeout before trigger and effective-setting readback; distinguish software conversion |
| Signals | Known edges/glitches, static channels, analog extremes/noise, missing clock metadata, large indices, multiple frames and gaps |
| Analysis | Reference UART/SPI/I²C and stacked decoder data, binary outputs, measurement fixtures, threshold/Schmitt conversion, math invalid values and graph cycles |
| Files | `.sr` samples and metadata, declared `.pvs` subset, own-document round trip, unsupported metadata reporting, corrupt/truncated/oversized input |
| Users and interfaces | Viewer/operator/admin, owner/other user, token scope restrictions, HTTP/CLI/MCP equivalence, unauthorized event replay and downloads |
| Concurrency | GUI and agent capture same device; two devices; analysis during capture; discovery during lease; competing metadata edits; deletion during decode/export |
| Delayed operations | Retry after lost acceptance response, queue expiry, approval expiry, late worker reply, browser reconnect, server/worker restart and stale viewport result |
| Lifecycle | Busy restart/disable/uninstall, operator versus admin authority, intentional uninstall surviving startup, package upgrade and saved-capture retention |
| Storage | Quota admission, full disk, interrupted finalization, temporary cleanup, pinned captures and byte reclamation after readers release references |
| UX | EN/DE/RO, light/dark/system, tooltip/help consistency, keyboard-only operation, 200% zoom, small desktop and wide monitor |

Maintain a feature coverage register with feature ID, available backend/build, GUI support, CLI/MCP support where meaningful, fixture evidence, live hardware evidence and known limitations. “Driver advertises it”, “NILO sends it”, and “hardware behaved correctly” are separate evidence levels.

## Performance investigation

Agree a measurable browser/Pi baseline during SW-04/SW-06. Proposed investigation workloads: 16-channel 10-million and 100-million-sample digital captures; multi-channel analog data; dense decoder output; and incremental acquisition while navigating old data. These are benchmark inputs, not claims that every appliance or driver supports those acquisition sizes/rates.

Record browser/host/Pi model, available RAM/storage, network throughput, cold/warm cache and dataset characteristics. Measure time to first useful waveform, pan/zoom latency, frame time, peak client/server memory, query payload sizes, disk/index overhead, decoder throughput and control-command latency under load. Initial interaction targets to assess are smooth navigation around 30 frames/s and useful cached viewport responses within roughly 100–200 ms. Confirm or revise these targets from measurements before making product guarantees.

Pass conditions must include bounded resource growth and correct raw/detail results, not just a visually smooth decimated preview. Downsampled display must preserve evidence of narrow pulses and analog extremes. Network throttling should lower preview detail/rate without silently losing recorded samples.

## Decisions already fixed

- Native NILO browser waveform workspace; no embedded/streamed PulseView desktop as the primary solution.
- Appliance-wide installed/enabled plugin state and shared GUI/CLI/MCP services.
- Admin install/enable/disable/uninstall; operator restart with appropriate busy protection; explicit uninstall confirmation.
- Capability-driven sigrok support, with ATK only one validation device; PWM outputs excluded.
- Named waveforms and analysis data are separate from asynchronous operation history.
- Durable transaction IDs, bounded command/reply handling, concurrency protection and timeout/reconnect behavior remain essential.
- Preserve NILO navigation, RBAC, help, localization and themes; prioritize waveform area and avoid redundant headings.

## Remaining decisions with proposed defaults

These are scoped implementation decisions, not reasons to stop the current planning work. Resolve them in the owning package and record the resulting contract.

| Decision | Proposed default / evidence needed | Owner |
|---|---|---|
| Structured worker technology | Native libsigrok/libsigrokdecode helper behind the existing process boundary; compare available bindings using a working slice | SW-01 |
| Non-waveform sigrok classes | Inventory all deployed classes, expose waveform-relevant ones here, and assign other instrument classes to explicit future surfaces; no arbitrary output passthrough | SW-01/02 |
| Capture ownership/sharing | Preserve owner-scoped access with admin oversight initially; add explicit sharing and viewer read access without implying global data visibility | SW-08 |
| Temporary retention and quota defaults | Appliance-admin policy; show expiry, pin on Save, never silently evict saved captures; choose numeric defaults from appliance storage budgets | SW-04/08 |
| Restart interruption policy | Ordinary operator restart rejects busy state; separately confirmed admin stop-and-proceed when necessary | SW-03/05 |
| Bundled uninstall/upgrade behavior | Persist intentional uninstall and enablement choices; preserve capture files; define migration/recovery before upgrade support | SW-05/15 |
| Continuous capture mode | Stream to reserved disk with explicit duration/byte limits first; rolling overwrite only as a separately labeled mode | SW-03/04 |
| `.pvs` compatibility | Publish tested field subset and unsupported-field report; preserve original file; never imply all PulseView settings round-trip | SW-14 |
| Math engine and transforms | Restricted, bounded expressions with reproducibility; select after matching relevant local PulseView examples | SW-13 |
| GUI rendering technology | Canvas 2D plus workers first; use measured evidence to decide whether WebGL adds value | SW-06 |
| Experimental PulseView features | Record build flags and availability; decide explicitly whether each belongs in shipped scope or future work | SW-01/15 |
| Physical hardware coverage beyond ATK | Use fixtures now for integration; schedule representative analog/other-driver acceptance before claiming those devices verified | SW-15 |

## Research completeness and next handoff

Completed for this planning pass: upstream help consultation, local manual analysis, targeted local source inspection, current NILO gap analysis, live Demo settings/channel panel/capture/cursor inspection and the work-package plan. The full source tree was not audited and not every PulseView gesture was exercised.

The next implementation task should start at SW-01, reread the current repository code and this folder, and establish the worker/capability contracts. Use the workflow authorized for that implementation phase; this planning folder intentionally creates no AEGIS journal or formal requirements. Product requirements derived later should describe observable product behavior, not repository administration tools.
