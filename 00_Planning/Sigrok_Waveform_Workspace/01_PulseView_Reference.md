# PulseView reference and evidence

Research date: 2026-09-05. This is an original feature inventory and design interpretation, not a copy of the PulseView manual.

## Evidence vocabulary

- **Manual**: described in the supplied local manual; the corresponding upstream web help was also consulted.
- **Source**: behavior or implementation present in the inspected local source. Build inclusion and runtime behavior may still need verification.
- **Live**: directly observed in the running Windows application during this research.
- **Proposed**: NILO behavior needed to deliver the user's requested workflow; not a claim about PulseView.

## Source register

| Reference | Location and use |
|---|---|
| W1 | [PulseView 0.4.2 web manual](https://sigrok.org/doc/pulseview/0.4.2/manual.html), acquisition, analysis, decoders, files and CLI reference |
| W2 | [Development web manual](https://sigrok.org/doc/pulseview/unstable/manual.html), cross-check of upstream help; this is not evidence of exhaustive current feature coverage |
| M | Local `F:/2026_PulseView/05_Software/pulseview/manual/`: `manual.txt`, `overview.txt`, `acquisition.txt`, `analysis.txt`, `decoders.txt`, `import_export.txt`, `cli.txt` and manual images |
| S | Local `F:/2026_PulseView/05_Software/pulseview/`, commit `bcf7e93414dbca640abf8e4b47802b57c5794b84` |
| L | Existing Windows PulseView session, Demo device; binary path recorded in this folder's README |

The local manual describes an unreleased development snapshot and records a last-change date of 2020-02-29. It is useful but insufficient as the sole inventory of newer source features. Manual screenshots `pv_decoders_2.png` and `pv_cursors_markers.png` were visually inspected. They show aligned protocol annotation tracks and cursor/marker overlays respectively. Do not assume screenshots establish device capability support.

## Window anatomy and behavior inventory

| Area | PulseView reference behavior and evidence | NILO interpretation | Task |
|---|---|---|---|
| Sessions and views | Multiple sessions and views, acquisition toolbar and common timeline (Manual, Source); session tab and trace toolbar visible (Live) | Named capture/analysis tabs inside Bench → Sigrok; presentation views reference one capture without duplicating raw data | SW-05, SW-08 |
| Device selection | Select recognized device or connect using a driver and connection information (Manual) | Discover plus explicit USB/serial/network connection where supported; stable device identity and busy status | SW-02, SW-05 |
| Acquisition settings | Sample rate/count and device-specific controls (Manual, Source); 1 M samples at 200 kHz, frame limit, averaging and pre-trigger ratio visible on Demo (Live) | Typed capability-driven forms; no ATK channel/rate constants | SW-02, SW-03 |
| Channel selection | Per-channel selection, groups and device-dependent options (Manual, Source); analog pattern selectors, D0–D7 and bulk selection controls visible (Live) | Distinguish acquisition enablement from hiding an already captured trace | SW-05, SW-07 |
| Channel presentation | Editable labels, colors, row heights, ordering and visual groups (Manual, Source) | In-place editing and keyboard equivalents; visual groups do not change hardware groups | SW-07 |
| Digital triggers | No trigger, low/high, rising/falling/either-edge choices filtered by device support (Manual, Source) | Query accepted matches and combinations; show a trigger badge on affected channels | SW-03 |
| Input threshold | Voltage threshold and other acquisition controls depend on driver (Source) | Hardware comparator threshold has its actual device/group scope, units and valid range; not a per-channel field by default | SW-02, SW-03 |
| Trigger position | Capture ratio / pre-trigger controls depend on device (Manual, Source); Demo shows 20% setting (Live) | Explain pre/post-trigger relationship and unsupported cases; preserve actual capture trigger position | SW-03, SW-06 |
| Analog traces | Scale, divisions, row height, autoranging and conversions (Manual, Source) | Analog and mixed-signal support is part of the plan; use physical units and channel metadata | SW-06, SW-07 |
| Navigation | Zoom, fit, pan, follow latest, automatic fit while acquiring and multiple views (Manual, Source) | Pointer-anchored zoom, keyboard navigation and explicit resume-follow after inspecting earlier data | SW-06 |
| Dense waveforms | Compact overview renders dense data; detailed view resolves signals (Source); dense analog/digital overview observed after Demo capture (Live) | Indexed, resolution-aware rendering; do not mistake dense overview bands for absent transitions | SW-04, SW-06 |
| Cursors | Two cursor boundaries and interval/reciprocal interval; markers can define timing references (Manual, Source); 2 s/3 s cursor pair with 1 s and 1 Hz readout observed (Live) | Show Δt, sample distance and explicitly labeled 1/Δt; this is only signal frequency if the interval represents a period | SW-09 |
| Markers | Named movable timeline markers and trigger reference (Manual, Source) | Persist user markers; keep acquired trigger position distinct and immutable | SW-09 |
| Decoders | Catalog, channel mapping, options, annotation rows and decoder stacks (Manual, Source) | Runtime catalog across installed libsigrokdecode, not a UART-only form | SW-10 |
| Decoder filters | Hide rows/classes, select annotation output and export a range (Manual, Source) | Separate visibility filters from data deletion and from acquisition filters | SW-10, SW-12 |
| Decoder tables | Tabular decoder view, sample-range filtering, export and navigation back to annotations (Source) | Synchronized optional table drawer, paginated queries and waveform navigation | SW-11, SW-12 |
| Binary decoder output | Separate binary/hexdump presentation and exports (Manual, Source) | Keep binary payloads and their classes distinct from textual annotations | SW-11 |
| Analog-to-logic | Threshold or Schmitt conversion and selection of analog/converted/both (Manual, Source) | Derived signal with reproducible parameters; software threshold is not the analyzer's hardware threshold | SW-13 |
| Math traces | Math signal creation and expression evaluation appear in local source (Source) | Include derived math tracks in the roadmap, with a bounded expression engine and provenance | SW-13 |
| Measurements | Cursors plus measurement-oriented decoders such as timing, counter, jitter and bitrate estimation (Manual) | Measurements cite channel/range/method/units and expose uncertainty or missing clock information | SW-09, SW-10 |
| Save and restore | `.sr` capture files; PulseView setup in a separate same-basename `.pvs` file (Manual, Source) | Preserve raw capture and NILO analysis document separately; define explicit `.pvs` compatibility scope | SW-08, SW-14 |
| Import and export | Input/output modules with format-specific options; raw and annotation exports (Manual) | Runtime format catalog and bounded import/export jobs; preserve transformation provenance | SW-14 |
| Search | Decoder row/class and range filtering are evidenced; comprehensive arbitrary waveform search was not established | Edge/pulse/pattern, annotation text and binary payload search are explicit NILO additions requested by the user | SW-12 |
| Names and deletion | File-oriented session saving is documented; a managed capture library with RBAC/retention was not established | Named saved waveforms, notes, labels, explicit deletion, quotas and separate operation retention | SW-08 |
| CLI/MCP | PulseView CLI supplies launch/load/device/decoder options (Manual) | NILO needs structured acquisition and analysis commands; desktop startup flags are not an automation API | All contract-bearing tasks |

## Important semantic distinctions

1. A digital edge/level trigger, an analog trigger level, a hardware digital input threshold and a software analog-to-logic threshold are different concepts. Show only controls appropriate to the selected device and signal.
2. A decoder is generally post-acquisition analysis. Searching for SPI bytes is not a hardware SPI trigger unless the selected driver actually provides that trigger capability.
3. A channel may be acquired but hidden. Hiding it must not destroy its data or break a decoder that depends on it. Acquisition selection changes affect a future run.
4. A visual channel group does not imply that channels have independent electrical settings. Hardware channel groups can share thresholds or other settings.
5. A frame/segment boundary is not necessarily a continuous clock interval. Do not join separate frames visually or numerically without recording the timing relationship.
6. When sample rate is unknown, use sample indices. Do not invent elapsed seconds, baud estimates or frequency measurements.
7. Filtering can mean acquisition filtering, hiding channels, hiding annotation classes, searching results or transforming sampled data. Give these actions separate names and effects.

## Source map for implementation research

Paths below are relative to `F:/2026_PulseView/05_Software/pulseview/` unless specified otherwise.

| Source | Why the implementer should read it |
|---|---|
| `pv/binding/device.cpp` | Enumerates device configuration keys; GET/SET/LIST handling and typed property bindings; includes trigger/capture and analog configuration |
| `pv/binding/decoder.cpp` | Decoder option defaults, enumerations, numeric and string settings |
| `pv/toolbars/mainbar.cpp` | Acquisition toolbar, sampling widgets, save/load and math-signal entry point |
| `pv/views/trace/signal.cpp`, `trace.cpp`, `logicsignal.cpp` | Channel presentation, names/colors/heights and supported trigger actions |
| `pv/views/trace/analogsignal.cpp` | Scale/divisions and analog-to-logic conversion controls |
| `pv/views/trace/view.cpp`, `viewport.cpp` | Time navigation, zoom, scrolling, follow behavior and input interactions |
| `pv/views/tabular_decoder/view.cpp` | Table selection, sample-range filters, annotation navigation and export |
| `pv/data/mathsignal.cpp` | Derived expressions and saved math-signal state |
| `pv/data/decodesignal.cpp` | Follow-up reference for decoder pipeline and data relationships; not exhaustively audited in this research |
| `pv/session.cpp`, `pv/data/logicsegment.cpp`, `pv/data/analogsegment.cpp` | Follow-up reference for acquisition, session persistence and waveform storage/indexing; requires targeted implementation investigation |
| `../libsigrok/include/libsigrok/libsigrok.h` | Capability access flags, device classes and datafeed event types: logic, analog, trigger, metadata and frame boundaries |
| `../libsigrok/src/hardware/atk-dl16/api.c` | One example of driver-specific trigger/threshold constraints; never the universal schema |

The local CMake configuration includes decoding support and an experimental flow option disabled by default. Experimental/build-disabled features must be recorded in the coverage register; their source presence alone is not a commitment that they are available in every NILO package.

## Live inspection record and limits

The open application was brought forward on the secondary monitor. The Demo device was already selected. Device settings and channel selection popups were inspected without changing their fields. The latter included bulk enable/disable by logic/analog, naming and changing/non-changing categories; these controls were observed, not behaviorally tested.

One finite Demo acquisition was started with the existing 1 M sample / 200 kHz settings. It returned to Run after acquisition and displayed approximately five seconds of mixed analog/digital data. Cursors were enabled and their overlay inspected. The application was left open with this unsaved Demo capture and cursors visible.

The [saved live screenshot](evidence/pulseview-demo-cursors.jpg) records the completed Demo overview and cursor overlay. It is a PulseView reference image, not a NILO design mockup or evidence of hardware capture.

Channel drag/resize, color changes, trigger arming on hardware, decoder execution, marker editing, save/restore and search were not verified live. Their evidence is explicitly manual/source or proposed above. Coordinate-based control of the secondary-monitor window was unreliable; accessible toolbar controls were used for the successful interactions. This is not a full PulseView usability or correctness audit.
