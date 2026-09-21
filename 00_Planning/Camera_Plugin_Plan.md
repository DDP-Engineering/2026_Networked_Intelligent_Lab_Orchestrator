# NILO camera plugin plan

**Status:** Proposed. Brainstorm consolidated on 2026-09-18 and architecture
review incorporated on 2026-09-19; no implementation has started. Open product
decisions are collected in section 12 and must be settled before their dependent
work package starts. This document is temporary: delete it once the work
packages are delivered, because its durable outcomes then live in requirements,
the knowledge pages and `06_SoftwareDocu/`.
**Date:** 2026-09-19
**Scope:** The first feature plugin after sigrok: USB cameras on the appliance,
including HDMI-to-USB capture devices, which Linux exposes the same way. It
covers device discovery and naming, snapshots, device controls, image
processing and overlays, later a live preview, and the generalisation of the
bench core that a second plugin family requires. Humans in the browser, the
command line and AI agents through MCP get the same operations.
**Relationship:** Step 7 of the delivery sequence in `Overview.md`. Extends the
bench core along the "Extension boundaries" of
`06_SoftwareDocu/Sigrok_Integration.md`. Depends on PKG-04 of
`Packaging_and_Delivery_Plan.md` for the self-contained integration runtime;
camera device access and its udev policy are owned here. Reuses the navigation
and naming direction of
`.agentic/specs/plugin-workflow-plan.md` without implementing that plan. Does
not touch the waveform workspace.

## 1. Decision summary

- **One plugin package, many managed devices.** A NILO may carry several
  cameras. Each has a stable identity, an administrator-assigned name, its own
  configuration, its own enable state, a presence state and its own lease.
  Managed devices are a generic core feature; sigrok migrates onto it too.
- **Snapshot first, through the existing job and artifact model.** A snapshot
  is one queued operation that produces one image artifact. It needs no new
  transport. Live preview is phase 2 with its own transport decision.
- **Processing happens in the runner.** Digital zoom, rotation, flips, invert,
  grayscale and overlays are applied by the integration, and that rendered
  image is the canonical retained evidence. Browser and MCP previews may reduce
  only resolution and JPEG quality under documented bounds; they do not
  reinterpret the processing. The browser may preview zoom and marker placement
  locally before asking for the retained snapshot.
- **Four value sources with explicit semantics.** Factory defaults, the
  administrator's configured baseline, current hardware state and one request
  are distinct. A reset establishes the baseline; `set_controls` and snapshot
  control overrides then update current state. Processing and encoding use
  request values over device defaults and plugin limits. Mounting correction is
  invariant and composes with, rather than being replaced by, request transforms.
- **Control state persists until a device reset point.** A control an operator
  or agent changes stays changed. The core re-applies the configured baseline
  at server startup, plugin or device enable, a configured-baseline change, a
  discovered reconnect and explicit reset. Browser authentication is not a
  hardware event and never resets a shared camera. A device is not ready for
  user work until required initialization has completed.
- **Agents receive pictures when the job completes.** The MCP snapshot tool
  waits for a bounded interval and returns image content when available. If the
  job still awaits approval or execution, it returns a structured pending result
  with the transaction id; losing the MCP call never cancels accepted work. Full
  resolution stays reachable through the artifact reader.
- **Runner in Python on V4L2**, with Pillow and a binding selected by the
  CAM-00 feasibility spike, packaged self-contained. A pure-Python binding is
  preferred but not promised before it has captured and controlled the target
  hardware on `arm64` and `amd64`. A deterministic simulation backend is
  mandatory because the development host has no camera access.
- **No new approval model.** Camera operations are `auto_eligible`; the
  plugin's Ask or Auto mode remains the only approval gate for user-submitted
  jobs. Core-owned initialization resets restore administrator configuration,
  run as the system actor without an interactive prompt, take the normal device
  lease and are audited. Opening a camera requires `bench.execute` and is
  audited, never `bench.read`.

## 2. Goals and non-goals

### Goals

1. Enumerate UVC video devices on the appliance, group their video nodes into
   physical cameras, distinguish supported capture endpoints from discoverable
   but unsupported ones, and let an administrator name a stable managed record,
   describe its purpose, correct its mounting and set its defaults.
2. Take a snapshot from a named camera with chosen resolution, device controls,
   digital zoom, rotation, flip, invert, grayscale and overlay lines, and keep
   it as a digest-verified artifact with its effective parameters.
3. Read, set and reset the supported typed controls the camera exposes:
   brightness, contrast, saturation, exposure, gain, white balance, focus and
   others, with reported choices, limits, flags, dependencies and actual
   read-back values. Unsupported control types remain visible with a reason and
   are never exposed as arbitrary ioctl passthrough.
4. Offer the same operations in the browser, the command line and MCP, with
   role and policy behaviour identical to sigrok's.
5. Generalise the bench core so the third plugin family needs no further
   surgery: a per-plugin adapter registry, generic managed devices, plugin
   configuration and artifact media types.
6. Phase 2: a bounded live preview of one or more cameras in the browser.

### Non-goals

- WebRTC, audio, video recording, motion detection or time-lapse.
- Computer vision, automatic measurement or calibration wizards. A manual
  millimetre-per-pixel factor and a scale bar are the whole measurement story.
- Pan, tilt or optical zoom beyond what a camera exposes as a UVC control.
- Arbitrary V4L2 passthrough. Every control and format is a discovered, typed
  capability; no raw ioctl, node path or command reaches the public interface.
- Automatic hotplug monitoring in phase 1. Presence changes on explicit or
  startup discovery; "reconnect" in this plan means the first reconciliation
  that observes the device again.
- The complete navigation redesign of `plugin-workflow-plan.md`. This plan adds
  the camera destinations and the shared device administration it needs, and
  no more.
- Surveillance. The camera is bench evidence; it is not streamed to tokens,
  recorded continuously or exposed without a session.

## 3. What the current implementation gives and where it is sigrok-shaped

The package contract, lifecycle, queue, leases, approval, artifacts and the
three transports are reusable as they are. The bench core and the clients are
tied to sigrok at these points, and each is a change this plan owns:

| Where | Today | Needed |
|---|---|---|
| `internal/bench/parameters.go` | Rejects any plugin except sigrok; knows only `capture` and `decode_uart` | Per-plugin typed adapters behind one registry |
| `internal/bench/model.go`, `packages.go` | Device carries channels and sample rates; discovery validates the ATK-DL16 shape and overwrites `devices_json` | Generic managed device with plugin-specific capabilities; discovery reconciles |
| `internal/bench/packages.go` `process()` | Appends `--backend` and `--simulation` only for sigrok | Adapter-supplied runner arguments; configuration delivered through the protocol `configure` call |
| `internal/bench/execution.go` `Persist()` | Accepts only an artifact named `capture`; one artifact per job | Adapter-declared artifact ids and media types; one artifact per job stays in phase 1 |
| `internal/httpapi/bench.go` artifact route | Hardcodes `capture.sr` and `application/octet-stream` | Media type and filename from the artifact record; inline serving for images |
| `internal/httpapi/server.go` device inventory | Every device is kind `logic-analyzer` | Kind from the adapter |
| `internal/mcp/bench.go` | `bench_submit` enumerates two sigrok operations | Operation validated by the adapter; camera tools added |
| `cmd/nilo/bench.go` | Sigrok-named subcommands and flags | Generic submit plus camera sugar |
| `nilo_gui/src/components/shell/AppShell.tsx`, `api/bench.ts` | The sigrok tab is the Plugins page; the device parser requires D0 to D15 | Camera destinations; capability-driven device types |
| Plugin configuration | Manifest and protocol define a schema and `configure` payload, but the supervisor does not send it and there is no route, storage, CLI, MCP or GUI | Versioned plugin and device schemas, configure-before-use runner lifecycle and administrator-only surfaces |
| Device naming and settings | None anywhere | Managed devices |
| Runner failures | Remote errors are reduced to `execution_failed` in the queued job | Preserve validated typed failure codes through job, HTTP, CLI and MCP reads |
| Streaming | None; the browser polls every 1.5 seconds | Phase 2 transport |

Bounds that shape the design: replies are 4 KiB and at most 100 per job, a
terminal result is 1 MiB, an artifact is 8 MiB, and the supervisor passes only
`PATH`, `LANG`, `LC_ALL` and `TZ` to a runner. Images travel as artifacts and
nowhere else.

Before those changes are implemented, CAM-00 freezes the cross-component
contracts: managed-device discovery, physical and endpoint identity, capability
descriptors, plugin and device configuration, operation canonicalisation,
dynamic resources, artifacts and error propagation. The manifest is the source
of truth for declared operation policy, timeout, schemas and artifacts; a
compiled adapter adds only family-specific canonicalisation, dynamic identity,
resources and core-owned paths. The two must be checked for agreement at package
load so a third family does not create another set of hard-coded branches.

## 4. Managed devices

### The rule

A plugin package is installed and enabled once. It manages zero or more
devices. Naming, configuration, presence and leases belong to the device. In
code and API these are **managed devices**; the word "instance" stays in
prose, because `InstanceID` already names the installed package copy.

### Identity

Three identities are kept apart, and this is the recommended policy awaiting
confirmation:

- **The record id** is assigned by a per-plugin monotonic database sequence when
  a device is first seen, readable and never reused: `cam-1`, `cam-2`, and for
  sigrok `la-1`. Names, configuration, leases, audit records and artifacts
  attach to the record id, so rebinding a configured record to other hardware
  keeps its history in one place.
- **The hardware identity** is what the record is currently bound to. The
  runner reports the attributes: USB vendor, product, serial, port path and
  available `/dev/v4l/by-id` names. These identify a physical USB device, not
  necessarily one capture node.
- **The capture endpoint identity** selects the usable V4L2 interface within a
  physical device: interface number, capability set and validated device node.
  A camera may expose capture, metadata and multiple format-specific nodes. The
  runner reports candidates; the adapter validates them against sysfs and V4L2
  capabilities, excludes metadata-only nodes and retains the selected address
  as internal state. No public request can supply a node path.

Matching present hardware to records follows one rule order, and the record
stores which rule applied:

1. Vendor, product and serial, when the serial is present, non-empty and not
   known to be weak. A serial is weak when two devices have ever been observed
   with the same vendor, product and serial, or when it is on a short reviewed
   list of known junk values such as `0` or `0123456789`. Once discovered, that
   classification is persisted; unplugging one duplicate never makes the other
   appear strongly identified on a later discovery.
2. Otherwise vendor, product and physical port path.

A serial-matched camera keeps its record wherever it is plugged in. A
port-matched camera that moves shows up as a new record and its old record
goes absent. The administration page then proposes a rebind when exactly one
absent record and one new record share vendor and product; the proposal is
never applied automatically. Rebind moves the hardware identity onto the
configured record. The provisional record is tombstoned, not reused or silently
deleted. Rebind is permitted only while that provisional record has no accepted
jobs, artifacts or administrator configuration; phase 1 prevents such history
by refusing operations until configuration is accepted. A general record-merge
feature is outside this plan. Two identical cameras with weak serials must stay
in fixed ports, and the user documentation says so.

### Persistence and reconciliation

A new migration adds `bench_devices`: plugin id, record id, hardware and capture
endpoint identity attributes, the identity rule and serial-quality decision,
display name, canonical name key, purpose text, versioned settings, enabled and
configured flags, `unknown`/`present`/`absent` presence, readiness and last-reset
status, revision, last-seen time, created time and optional tombstone target.
It is unique on plugin and record id and on plugin and canonical name key.
`devices_json` on `bench_plugins` is retired; the development compatibility
policy allows a direct replacement while preserving unrelated appliance data.

The canonical name key uses one specified Unicode normalisation and case-folding
algorithm, NFKC Casefold, in the core; SQLite's ASCII `NOCASE` is not the
contract. Every administrative mutation carries the last observed revision and
stale writes fail with a conflict instead of overwriting another client.

Discovery merges instead of replacing. After a discovery a device is exactly
one of:

| State | Meaning | Operations |
|---|---|---|
| unknown | Stored record has not yet been reconciled after startup | Refused with `device_not_ready` |
| present, initializing | Known camera is applying its configured baseline | Refused with `device_initializing` |
| present, configured, ready | Known camera, plugged in and initialized | Allowed when enabled |
| present, unconfigured | New camera with a unique suggested label | Refused until an administrator accepts or replaces the name and configuration |
| configured, absent | Named camera, currently unplugged | Refused with `device_absent`; record and history remain |

Discovery is explicit in phase 1, except for one bounded startup/enable discovery
needed to establish truthful presence and initialization. It merges instead of
replacing and updates all records atomically. A failed discovery leaves earlier
records `unknown`; it does not assert that every camera is absent. A reconnect is
noticed only by a later reconciliation because phase 1 has no hotplug monitor.

Submission requires configured, present, ready and enabled. Uninstall keeps
device records the way it keeps job history. Forgetting is an explicit,
revision-checked administrator action, refuses active work, clears the current
binding and configuration, and tombstones rather than reuses the id. It never
deletes artifacts or makes historical job references dangling.

### Configuration layers

| Layer | Owner | Lifetime | Content |
|---|---|---|---|
| Camera factory defaults | Hardware | Until changed by hardware/firmware | Discovered control defaults; used only by an explicit factory reset |
| Plugin configuration | Administrator | Persistent | Hard maximum dimensions and pixel count, default JPEG quality, preview feature and global preview limits |
| Device configuration | Administrator | Persistent | Name, purpose, invariant mounting rotation/flips, default capture endpoint/format/size/rate, configured control baseline, processing and encoding defaults, calibration with a note, preview limits and enabled state |
| Current device state | Operator or agent, state changing | Until a reset point | Actual read-back control values after configured reset, `set_controls` or snapshot overrides |
| Request parameters | Operator or agent | One operation | Capture size/rate, control overrides, zoom, request rotation/flips, invert, grayscale, overlays and encoding |

For controls, a configured reset establishes current state and request values
override it for that operation while remaining as the new current state. For
capture, processing and encoding, request values override device defaults while
plugin values remain hard limits. Mounting correction is always applied first
and cannot be disabled by a request. The snapshot result records the configured
revision, actual read-back controls and every effective capture and rendering
value, making the artifact attributable and auditable; a real optical scene is
not claimed to be reproducible.

### Reset points

The configured baseline is re-applied to every present, enabled camera:

- after the bounded discovery performed at plugin enable and every server start
  with the plugin enabled;
- when a device is enabled or an administrator changes its configured control
  baseline;
- when an explicit discovery observes an absent camera reconnecting;
- on an explicit `reset_controls`, which an agent or the command line can
  call at any time.

A lifecycle reset is a core-initiated `reset_controls` operation with the system
actor, queued behind the device lease and audited. It restores administrator
configuration, so it is not held for Ask-mode approval. The device remains
`initializing` and rejects user submissions until the reset and read-back
complete. If the camera is absent or busy in another process, the device retains
a structured not-ready status and an explicit Retry action; the failure is not
collapsed into the whole plugin's status and user work is not allowed to proceed
against unknown control state. Browser sign-in, sign-out and token creation do
not affect hardware.

### Names and purpose

Names are metadata, never addresses: operations take the device id, and every
lease, audit record and artifact uses the id. A name is Unicode plain text of
1 to 64 characters, unique per plugin by the canonical key above, and renaming
never changes an id. Discovery creates a deterministic unique suggested label
such as `USB Camera (cam-2)` but does not silently turn it into accepted
configuration. The purpose text, up to 200 characters, is written for the agent
as much as for people: "top-down view of the DUT, 0.05 mm per pixel" is how an
agent picks the right camera among three.

### Authority

Naming, configuring, enabling, rebinding and forgetting a device require
`plugins.manage`; decided 2026-09-18 that operators cannot rename. Discovery,
snapshots and control changes require `bench.execute`. Reading the operational
device list requires `bench.read`; raw node paths and full hardware identity are
administrator diagnostics, not ordinary inventory. Every administrative change
writes an audit record with the plugin and device ids, old and new revisions and
the resulting state.

## 5. The camera package

### Layout

`05_Software/integrations/camera/` with `manifest.json`, `schemas/`, `runner`,
`src/nilo_camera/`, `tests/`, `pyproject.toml` and `uv.lock`, mirroring the
sigrok package. The runner takes `--stdio`, `--simulation` and a
`--simulation-source` directory of images for replay, and nothing else. The
source is selected by the core from a configured core-owned or package-owned
directory; it is never a request parameter.

### Backends

- **Real:** V4L2 through the binding selected by CAM-00, with Pillow for image
  decoding, processing and encoding. The spike must demonstrate discovery,
  streaming buffers, typed controls and cancellation on the Pi 4 and mini PC.
  MJPEG is preferred when offered; an unchanged MJPEG frame may pass through
  only when no processing, resize or encoding change is requested. YUYV is the
  required fallback. Other formats may be discovered as unsupported with a
  reason. OpenCV is not the default because phase 1 needs none of its wider
  computer-vision surface; adopting it requires measured packaging evidence.
- **Simulation:** a synthetic test pattern carrying the device name, a
  timestamp and a frame counter, plus optional replay from image files. Tests
  inject a fixed clock, seed and initial frame number so the encoded bytes are
  deterministic; interactive simulation may use the live clock. Two simulated
  cameras exist by default, one with a weak serial and multiple reported video
  nodes, so identity, endpoint selection and rebind can be exercised. Contract
  tests and the browser suite run against this backend.

### Operations

| Operation | Invocation | Policy | State changing | Runner timeout | Resource |
|---|---|---|---|---|---|
| `discover_devices` | Serialized control call | `auto_eligible`; control call does not enter approval | no | 15 s | plugin discovery lock |
| `snapshot` | Job | `auto_eligible` | yes; optional controls make the operation conservatively state-changing | 30 s | `camera/device/<id>` |
| `get_controls` | Job | `auto_eligible` | no | 10 s | `camera/device/<id>` |
| `set_controls` | Job | `auto_eligible` | yes | 10 s | `camera/device/<id>` |
| `reset_controls` | Job or system initialization | `auto_eligible` for users; no prompt for system initialization | yes | 10 s | `camera/device/<id>` |

The runner timeout bounds execution after a lease is acquired. The public job
deadline separately includes approval and queueing. Discovery remains the
existing explicit, audited and bounded lifecycle/control call: it requires
`bench.execute` but does not create a user job or enter Ask-mode approval.

`reset_controls` takes a target, `configured` or `factory`. The core's own
reset points use `configured`.

`discover_devices` returns, per physical camera: hardware identity, all candidate
endpoint identities, the selected capture endpoint or an unusable reason,
formats with discrete or ranged sizes and frame intervals, and control
descriptors. A control descriptor includes id, stable machine name, display
name, type, range or menu choices, step, default, current value, unit when
known, and read-only, inactive, volatile and automatic/manual dependency flags.
Phase 1 makes integer, Boolean and menu controls settable. Other valid V4L2
types remain discoverable but read-only with a reason. Metadata-only nodes are
excluded from endpoint selection.

`snapshot` takes an optional format, size and frame interval that must match a
discovered capability, optional control values that the runner applies in
dependency order before the frame and leaves in place, a processing block and
an encoding block. After a camera is opened or controls change, the runner
discards the configured bounded number of warm-up frames or waits the configured
settling interval without exceeding the operation deadline, then reads the
actual controls back. The processing block holds zoom as a factor from 1 to 8
with a centre, rotation
in quarter turns, horizontal and vertical flips, invert, grayscale and up to
32 overlays of the kinds crosshair, grid, line, rectangle, text and scale bar.
Overlay coordinates are normalised to the frame after mounting correction and
before zoom, so a marker stays anchored to the physical scene when the zoom
changes; the browser converts from screen coordinates.

The rendering order is fixed: capture, invariant mounting correction, scene
crop/zoom, request rotation/flips, grayscale/invert, output resize, overlay
projection and encoding. Overlay geometry uses scene coordinates; line width and
text size use bounded output pixels. Scale-bar length accounts for calibration,
zoom and output resize. A licensed Unicode font is bundled so text layout does
not depend on the appliance image. The encoding block chooses JPEG or PNG, a
quality and an optional requested maximum dimension.

The result carries the device id and configuration revision, capture time,
selected endpoint identity without a public node path, format, size and frame
interval, effective and actual read-back controls, warm-up behavior, the
processing and encoding actually applied, mounting correction, calibration
revision, artifact size and simulation flag. The artifact id is `image`, its
media type is `image/jpeg` or `image/png`, and its presentation hint names the
file after the device and the time.

Limits enforced in both core and runner: the plugin's maximum dimension, an
administrator-defined decoded-pixel ceiling, an 8 MiB artifact, 32 overlays,
text overlays of 120 characters, and control values within the discovered range
or menu. A requested maximum dimension intentionally resizes output; an input
that exceeds administrator limits without a valid resize request is refused for
JPEG and PNG rather than silently changed. An encoded image over 8 MiB fails
instead of silently reducing quality. Encoders strip EXIF and other source
metadata unless a future requirement explicitly retains it.

### Errors

`device_not_ready`, `device_initializing`, `device_absent`, `device_busy` when
another process holds the node, `unsupported_endpoint`, `unsupported_format`,
`control_out_of_range`, `control_unsupported`, `frame_timeout`,
`invalid_parameters` and `runtime_unavailable`. The core preserves an allowlist
of runner error codes in the terminal job instead of reducing them to
`execution_failed`; immediate HTTP/CLI/MCP submission failures and later job
reads use the same public codes without exposing runner diagnostics.

## 6. Core changes

1. **Manifest and adapter authority.** The manifest remains authoritative for
   operation ids, schemas, policy, timeout, state-changing declaration and
   artifact declarations. An interface in `internal/bench` canonicalises a
   submission, derives hardware and endpoint identity, validates discovery,
   supplies curated runner arguments, derives dynamic resources, resolves
   core-owned paths and reports the device kind. Package load rejects manifest
   and adapter disagreement. Sigrok becomes the first implementation and the
   `if id == "sigrok"` branches disappear.
2. **Managed devices** as in section 4: migration, repository, revisioned
   mutations, atomic reconciliation in `Discover`, explicit readiness and
   presence checks in `Submit`, tombstones and the routes below. Sigrok's runner
   must report sufficient USB identity instead of only its transient connection
   string before its devices can migrate safely.
3. **Configuration contracts.** The manifest gains a confined
   `device_configuration_schema` beside `app_configuration_schema`. A
   `bench_plugin_configuration` record and each device configuration have a
   revision and are validated against their installed package schemas. The
   supervisor sends one versioned `configure` payload containing plugin settings
   and the target device settings after `hello` and before `describe`, discovery
   or execution at every runner start. Health fails if configuration is refused;
   no unconfigured runner handles hardware.
4. **Artifacts.** The adapter validates artifact id and a narrow allowed media
   type; the runner cannot choose an arbitrary browser-active type. Media type
   and sanitised filename come from the registered artifact. The HTTP route
   serves only validated JPEG and PNG inline, keeps `nosniff` and `no-store`,
   and uses attachment disposition for everything else.
5. **Bounded previews.** Phase 1 adds an authenticated preview read for an
   accessible completed image job. It decodes only JPEG/PNG after checking
   dimensions and a decoded-pixel ceiling, applies CPU/memory/time bounds and
   returns a non-evidentiary image no larger than 1280 pixels. Browser tiles and
   history lazy-load this preview rather than the full artifact; CAM-12 may later
   retain materialised thumbnail artifacts.
6. **MCP image content.** `camera_snapshot` waits for a configured bounded
   interval. On completion it returns the bounded preview as MCP image content
   plus the transaction id. When approval, queueing or execution continues, it
   returns a structured pending result with the same id and polling guidance.
   An MCP timeout or disconnect does not cancel the accepted job. The full
   artifact remains available through `bench_read_artifact`.
7. **Queue and initialization.** There is no change to the 32-user-job limit or
   two workers. Two ready cameras can snapshot in parallel; one camera
   serialises. Core initialization resets use the same lease but do not consume
   user approval or permit later user work to overtake them for that device.
8. **Reset triggers.** Bounded startup/enable discovery and later reconciliation
   enqueue a system-actor `reset_controls` for each newly present, configured,
   enabled device whose adapter declares a reset operation. Explicit user reset
   uses the normal policy. The identity service gains no camera observer hook.
9. **Typed failures.** Validated runner codes survive operation finalisation and
   appear in the terminal job. Unknown codes become `execution_failed` while
   full diagnostics stay server-side.
10. **Ownership.** Image history, previews and artifacts retain the existing
    owner-or-administrator rule. Any "last snapshot" query resolves the newest
    snapshot the caller is allowed to read, never merely the newest global row.

## 7. Interfaces

### HTTP, CLI and MCP

| Capability | HTTP | CLI | MCP |
|---|---|---|---|
| List plugins, lifecycle, discover | existing routes | `nilo plugin ...` | `plugins_list`, `plugin_lifecycle`, `plugin_discover` |
| List managed devices | `GET /api/v1/plugins/{id}/devices` | `nilo plugin devices --plugin camera` | `plugin_devices` |
| Name, purpose, configure, enable, disable or retry initialization | revision-checked `PUT /api/v1/plugins/{id}/devices/{device}` and action routes | `nilo plugin device configure --plugin camera --device ID --revision REV --file cfg.json` | `plugin_device_configure` with revision |
| Rebind, forget a device | revision-checked `POST .../devices/{device}/rebind`, `DELETE .../devices/{device}` | `nilo plugin device rebind`, `forget --confirm` | `plugin_device_rebind`, `plugin_device_forget` with explicit device-id confirmation |
| Plugin configuration | revision-checked `GET`, `PUT /api/v1/plugins/{id}/configuration` | `nilo plugin configure --plugin camera --revision REV --file` | `plugin_configure` with revision |
| Submit any operation | existing `POST /api/v1/bench/jobs` | `nilo bench submit --plugin camera --operation snapshot --device ID --parameters-file` | `bench_submit` with a free operation string |
| Snapshot | same | `nilo camera snapshot --device ID [--zoom 2 --center 0.5,0.5 --invert --overlay ...] [--wait DURATION] --out board.jpg` | `camera_snapshot` returning image content or a pending transaction |
| Controls | same | `nilo camera controls get|set|reset --device ID` | `camera_get_controls`, `camera_set_controls`, `camera_reset_controls` |
| Download | existing artifact route | `nilo bench download` | `bench_read_artifact` |
| Bounded image preview | `GET /api/v1/bench/jobs/{id}/artifact/preview` | not separate; used by snapshot wait | returned by `camera_snapshot` |

Camera CLI commands are sugar over the generic submit. Every camera command
requires `--device` once more than one camera is configured; a single
configured camera is the implicit default only in the CLI, never in MCP.
`--out` implies a bounded wait; if the job remains pending, the CLI writes no
file, prints the transaction id and reports the pending state. A lost client
does not cancel the job. Administrative changes require `plugins.manage` on
every surface. Rebind and forget require both the current revision and an exact
device-id confirmation in CLI and MCP; GUI confirmation supplies the same
contract rather than being a client-only safety measure.

### Browser

- **Navigation.** Follow `plugin-workflow-plan.md`: remove the separate
  Instruments cluster for managed plugins. Under Bench, Camera is a page-owning
  group with an overview and one child per configured camera, titled with its
  name. Generalise `parentId` grouping beyond the current core-only Network
  implementation. The working group appears only while the plugin is enabled.
  System > Plugins > Camera holds package and device administration.
- **Overview.** Every configured camera is a tile with presence, readiness, a
  quick snapshot action and the caller's last accessible snapshot; a two-up
  compare view can use any two snapshots the caller may read. Preview images
  are bounded and lazy-loaded, never full 8 MiB artifacts.
- **Camera page.** A toolbar with Snapshot, Discover, Help and Activity. A
  viewport showing the last snapshot with local zoom, pan, rotate, flip and
  invert, and drag-placed markers, all converted to the normalised overlay
  coordinates and sent with the next snapshot. A controls pane generated from
  the discovered descriptors, with Apply and Reset. A history of this camera's
  accessible snapshots has bounded previews, download and the existing job
  detail. Local transformations are labelled as a composition preview until a
  retained snapshot succeeds; viewer pan is kept separate from the requested
  crop centre.
- **Administration.** Device table with name, identity, presence, enabled and
  the actions rename, configure, rebind and forget, the last two behind the
  established confirmation dialog and revision checks. New unconfigured devices
  are flagged at the top and cannot capture until an administrator accepts or
  replaces their suggested configuration. Plugin configuration sits on the
  same page.
- **Constraints.** Existing tokens, help components, English, German and
  Romanian, all three themes, the 800 by 480 to large-display range, 14 pixel
  minimum text and 44 pixel targets. The image viewport must not force the
  page to scroll horizontally.

## 8. Live preview, phase 2

- **Gate:** Phase 1 does not promise live preview. CAM-10 measures the real
  devices and writes a reviewed preview design with a go/no-go result. Only an
  accepted design starts CAM-11 implementation.
- **Transport candidate:** an authenticated, CSRF-protected `POST` creates a
  bounded preview session and returns an opaque preview id. A side-effect-free
  `GET .../previews/{id}/stream` serves `multipart/x-mixed-replace` to an `img`
  element, and `DELETE .../previews/{id}` stops it. This avoids using a
  state-changing cookie-authenticated GET. WebRTC is not attempted unless the
  feasibility work disproves MJPEG.
- **Authority:** session credential only, `bench.execute`, audited at start
  and stop. Tokens cannot open a preview.
- **Bounds:** frame rate and width from the device configuration, concurrent
  previews and maximum session length from the plugin configuration, after
  which the browser must reopen. Preview holds the device lease.
- **Runner lifecycle:** the present per-operation supervisor is not reused
  unchanged. The design must introduce a bounded preview-session registry and
  one supervised long-lived runner per previewing device. Control remains on
  NDJSON; frames use a core-owned Unix socket with restrictive permissions,
  bounded length-prefixed JPEG frames, maximum decoded/encoded sizes, a
  newest-frame-wins drop policy, slow-consumer backpressure and restart cleanup.
  Frames never enter the reply queue.
- **Snapshot arbitration:** because opener behavior varies by device and driver,
  CAM-10 must measure it rather than assert a universal V4L2 rule. When a device
  requires one streaming opener, the long-lived runner serves a snapshot from
  the same stream under the same lease. Otherwise the design still exposes one
  consistent behavior to clients.
- **Feasibility first.** Measurements cover two cameras concurrently on the Pi
  4 and mini PC, CPU, memory, USB bandwidth, latency, opener behavior, disconnect
  cleanup and snapshots during preview before an interface is committed.

## 9. Device access, packaging and development

- **udev.** This plan, not PKG-04, owns camera device access. CAM-08 adds or
  creates `60-nilo-instruments.rules` with a reviewed `video4linux` policy and
  installs the `nilo` service account in the corresponding narrowly used
  `video` group. The delivered rule keeps device nodes non-world-readable, and
  acceptance verifies the effective group, mode and service supplementary
  groups after reboot. The bench notes record the cameras' USB identities.
- **Bundle.** The camera package is built by the recipe-driven builder of
  PKG-04 with the vendored interpreter and locked wheels for `arm64` and
  `amd64`. Pillow, the selected V4L2 binding and any separately approved font
  asset are pinned, checksum-verified and licence-reviewed; CAM-00 determines
  the actual dependency closure rather than assuming it.
- **Development host.** WSL2 has no UVC support in its stock kernel. The
  simulation backend carries all browser and contract work; real cameras are
  exercised on the bench Pi and the mini PC. A usbipd setup is optional and
  not a prerequisite.
- **Bench hardware needed:** two UVC webcams, one of them without a serial, one
  HDMI-to-USB capture dongle, connected to the Pi 4 and the mini PC in turn.

## 10. Verification

- **Core:** identity derivation and reconciliation across present, absent,
  unknown, initializing, unconfigured and rebind; persistent weak-serial
  classification; refusal to rebind a provisional record with history; adapter
  and manifest agreement with sigrok unchanged in behavior; configuration
  validation and configure-before-use delivery; typed terminal errors; artifact
  media types and bounded preview decoding; ownership and permission matrix on
  every new route and tool. Concurrent stale revisions must conflict.
- **Package:** the common contract suite; runner tests for parameter
  rejection, control persistence after a snapshot, reset to configured and
  factory levels, automatic/manual control ordering, warm-up bounds, endpoint
  selection, overlay and calibrated scale rendering at the boundaries, encoded
  and decoded image limits, metadata stripping and byte-deterministic simulation
  under an injected clock and seed.
- **Interfaces:** CLI tests for the generic submit and the camera sugar; MCP
  tests for completed image content, Ask-mode pending results, disconnect
  without cancellation and image bounds; browser component tests for the
  overview, camera page and administration; owner/admin image visibility,
  destructive confirmations and the browser suite against the simulated runner
  in WSL2 across viewports, themes and languages. Signing in or out in another
  browser must not change camera controls.
- **Hardware acceptance** on the Pi 4 and the mini PC: discovery of two
  cameras and their capture endpoints, a snapshot from each with controls
  applied and read back, explicit configured reset, restart/enable initialization,
  the rebind flow with the serial-less camera, duplicate/weak serial behavior,
  first-frame quality after warm-up, the HDMI dongle as a camera, external-busy
  recovery and concurrent snapshots.
- **Measurements to record, not guarantees:** snapshot round trip at 1280 by
  720 on the Pi 4, process startup, peak memory, encoded size and warm-up time;
  in phase 2, preview rate, latency, CPU, memory and dropped frames at 640 by 480
  for two cameras at once.

## 11. Work packages

Phase 1 delivers CAM-00 to CAM-09. Contract work precedes both core storage and
the package; after CAM-00, CAM-03's simulation backend may proceed alongside
CAM-01 and CAM-02, but it may not invent a second contract.

**CAM-00 — Contract freeze and camera feasibility.** Specify the managed-device,
hardware/endpoint identity, capability, configuration, operation, artifact and
typed-error envelopes. On the Pi 4 and mini PC, compare candidate V4L2 bindings
with one webcam and the HDMI dongle and measure discovery, node grouping, MJPEG,
YUYV, control enumeration/set/read-back, cancellation, warm-up and packaging.
*Done when* one dependency choice is supported on both architectures, unusable
formats/control types are documented, the contracts are accepted, and any
failed feasibility item has changed the scope before production implementation.

**CAM-01 — Adapter and protocol foundations.** Implement manifest/adapter
agreement, the adapter registry, dynamic resources and artifact declarations,
typed terminal error preservation, the device-configuration schema field and
configure-before-use supervisor lifecycle. Migrate sigrok without changing its
public behavior, remove sigrok branches and generalise artifact serving.
*Done when* the full Go suite and sigrok browser workflow pass unchanged, no
`"sigrok"` branch remains in the bench manager, configuration is sent before
runner use, and hostile adapter/manifest mismatches are refused.

**CAM-02 — Managed devices in the core.** Add the migration, repositories,
monotonic ids, revisions, Unicode canonical names, identity derivation,
reconciling discovery, readiness/initialization, tombstones, submission checks,
device routes and audit records. Extend sigrok discovery with sufficient USB
identity and migrate it to managed records.
*Done when* sigrok's two analyzers can be named, one can be unplugged without
losing its record, duplicate/weak identity and stale revisions are covered, a
provisional record with history cannot be rebound, and every route enforces its
permission.

**CAM-03 — Camera package.** Implement the frozen manifest and schemas, runner,
selected real backend, deterministic simulation backend, physical-node grouping,
`discover_devices`, `snapshot`, typed controls and package tests.
*Done when* the package passes the contract suite and its own tests, simulated
snapshots are byte-deterministic under a fixed clock/seed, and the real backend
passes the CAM-00 capture/control subset on both targets.

**CAM-04 — Camera adapter and generic submission.** The camera adapter in the
core, device initialization/reset orchestration, the free operation string in
`bench_submit`, generic and camera CLI commands, bounded preview reads, and MCP
camera tools with completed-image or pending results.
*Done when* an agent receives a bounded picture from `camera_snapshot`, Ask mode
returns a durable pending transaction, the CLI writes the same retained artifact
after its bounded wait, typed failures survive, explicit reset restores the
baseline, and unrelated sign-in/sign-out events leave controls unchanged.

**CAM-05 — Plugin and device configuration surface.** Storage and revisions,
validation against installed manifest schemas, the routes, CLI commands, MCP
tools and administration forms. CAM-01 already supplies configure delivery.
*Done when* an administrator changes the maximum snapshot dimension and the
next snapshot honours it on all three surfaces, while an operator is refused.

**CAM-06 — Browser camera workspace.** Navigation group, overview tiles,
camera page with clearly separated viewer/composition state, local zoom and
markers, generated controls pane, bounded lazy previews and owner-scoped
per-camera history.
*Done when* the browser suite covers the workflow with two simulated cameras
across viewports, themes and languages.

**CAM-07 — Browser device administration.** The Camera administration page
with the device table, rename, configure, enable, disable, rebind and forget,
with revisions and confirmations, plus the shared components the Plugins page
needs for it.
*Done when* the rebind flow for the serial-less simulated camera passes in the
browser, stale edits conflict, destructive actions require the exact id, and
every action is audited.

**CAM-08 — Packaging and device access.** The camera recipe in the PKG-04
builder, approved dependencies/font, the camera-owned udev rule and group
membership, and the bench notes for camera identities.
*Done when* a packaged appliance discovers a real camera as the `nilo` account
without manual permission changes.

**CAM-09 — Hardware acceptance and documentation.** The acceptance list of
section 10 on both targets, `06_SoftwareDocu/Camera_Integration.md`, the
knowledge page updates, the README capability row and the changelog.
*Done when* the acceptance evidence is recorded and this plan can be deleted.

Phase 2:

**CAM-10 — Live-preview feasibility and design.** Build the measured prototype
and decide go/no-go. Specify preview-session creation/deletion, CSRF and session
authority, long-lived supervision, frame protocol and bounds, backpressure,
cleanup, leases, disconnect behavior and snapshot arbitration.
*Done when* measurements from two simulated and two real cameras exist for both
targets and an accepted design either authorizes CAM-11 or records why preview
is deferred. No browser interface is promised by this package.

**CAM-11 — Live-preview implementation.** Only after CAM-10 approval, implement
the long-lived runner registry, bounded side channel, preview-session routes,
lease handling and browser Live toggle.
*Done when* two simulated and two real cameras preview concurrently within the
accepted bounds, cross-site GET cannot create a preview, disconnect cleanup is
proved, and snapshot-during-preview follows the measured device behavior.

**CAM-12 — Secondary artifacts.** Generalise jobs to more than one artifact so
a snapshot can retain a materialised thumbnail or an explicitly requested
unprocessed frame beside the rendered one. Phase 1 already uses ephemeral
bounded previews and does not download full images into overview tiles.
*Done when* overview/history may use retained thumbnails and artifact ownership,
digest, deletion and download behavior remains unambiguous.

Later, not planned here: sequences, frame comparison, automatic measurements
and calibration workflows beyond the manual factor and scale bar.

## 12. Decisions

### Decided on 2026-09-18

1. **Who may name and configure a device.** Administrators only. Operators
   and agents own the current device state and the request parameters.
2. **Managed devices are a generic core feature**, delivered in CAM-02 before
   the camera package lands, and sigrok migrates onto them.
3. **Control state persists until a reset point.** A snapshot or
   `set_controls` leaves the camera as commanded. The 2026-09-19 review retains
   persistence but reopens the exact automatic reset triggers because browser
   authentication is not a safe boundary for shared hardware; recommendation 8
   below replaces that part if confirmed.

### Recommended, awaiting confirmation

4. **Identity policy.** A core-assigned monotonic record id that history attaches
   to, a separate physical hardware identity matched by a persistently classified
   strong serial first and port path second, and a validated capture endpoint.
   Rebind is proposed but never automatic and tombstones the unused provisional
   id. Section 4 has the full rule.
5. **Unconfigured cameras.** Give a discovered camera a unique suggested label
   but refuse operations until an administrator accepts or replaces its name
   and configuration. This guarantees that a provisional rebind candidate has
   no jobs or artifacts and keeps administrator ownership of device meaning.
6. **Overlay coordinate frame.** Normalised to the frame after mounting
   correction and before zoom, with the fixed transformation and calibration
   order described in section 5.
7. **Preview credential and creation.** Session only; an agent gets snapshots,
   not streams. A CSRF-protected mutation creates a bounded preview session and
   a separate opaque-id GET carries frames.
8. **Reset boundaries.** Startup/plugin-enable discovery, device enable,
   configured-baseline change, discovered reconnect and explicit reset only.
   Lifecycle initialization uses the system actor and blocks the device until
   read-back succeeds. Sign-in, sign-out and token lifecycle never change
   camera state.
9. **Contract-first order.** CAM-00 freezes shared contracts and proves the
   camera binding before managed-device storage or the production package is
   implemented; manifest declarations remain authoritative over duplicated
   adapter metadata.
10. **Pending snapshot behavior.** CLI and MCP waits are bounded. Ask mode or a
    busy queue returns the durable transaction id without cancelling accepted
    work; image content is returned only after completion.

## 13. Risks

- **Cameras without a serial, or with duplicate serials,** are common in cheap
  hardware. Persisted weak-serial classification, the port-path fallback and
  explicit constrained rebind exist for this; the simulation includes such a
  camera so the flow is tested from the start.
- **One physical camera can expose several video nodes.** Treating a node as a
  camera creates duplicate records or selects metadata-only input. Physical and
  endpoint identity are separate contracts and CAM-00 verifies real examples.
- **Shared control state can surprise concurrent clients.** Authentication
  events never reset hardware; leases serialize operations, every mutation is
  audited, actual values are read back and explicit reset is visible.
- **USB bandwidth and Pi CPU.** Two uncompressed streams on one USB 2.0 hub
  will not fit; MJPEG from the camera and configured preview limits are the
  answer, and phase 2 measures before promising.
- **UVC controls vary or misreport their range.** Phase 1 sets only supported
  typed controls, orders automatic/manual dependencies, validates reported
  choices and reads the actual value back into the result.
- **The 8 MiB artifact ceiling** rules out large PNG frames. The maximum
  dimension, decoded-pixel limit and explicit encode failure keep this bounded;
  preview decoding applies its own CPU/memory/time limits.
- **Python camera bindings may not close on both architectures.** CAM-00 proves
  the exact binding and locked dependency closure before the package contract
  depends on it; pure Python is a preference, not an unsupported promise.
- **Scope creep from the plugin workflow plan.** This plan adds only the
  camera destinations and device administration. Named captures, deletion and
  storage budgets remain that plan's scope.
- **Privacy.** A bench camera can see people. The session-only preview, the
  `bench.execute` requirement, owner-scoped image reads, stripped source
  metadata and audit records are safeguards; the user documentation says so
  plainly.

## 14. Removal

Delete this file when CAM-09 is delivered. What survives it: the requirements
each work package creates under AEGIS, the updated `bench-integrations.md`,
`integration-platform.md` and `gui-foundation.md` knowledge pages,
`06_SoftwareDocu/Camera_Integration.md`, the README capability row and the
changelog entries. The phase 2 work packages move into that guide's "planned"
section if they are not delivered together.

## References

- `Overview.md` — instrument strategy and delivery sequence.
- `Packaging_and_Delivery_Plan.md` — PKG-04 self-contained runtime and bundle
  layout; camera udev policy is owned by this plan.
- `.agentic/specs/plugin-workflow-plan.md` — navigation, naming and RBAC
  direction for managed plugins.
- `06_SoftwareDocu/Sigrok_Integration.md` — current lifecycle, limits and the
  extension boundaries this plan builds on.
- `06_SoftwareDocu/Local_Hardware_Access.md` — bench devices and the rule to
  identify USB hardware by vendor, product and serial.
- `05_Software/schemas/integration/v1/` — the package contract.
- `05_Software/integrations/sigrok/` — the reference package this one mirrors.
