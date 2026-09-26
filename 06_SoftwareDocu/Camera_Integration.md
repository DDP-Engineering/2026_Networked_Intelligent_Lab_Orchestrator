# Camera integration

The Camera integration uses physical USB cameras exposed by Linux V4L2. It
discovers capture nodes and hardware controls, keeps a stable managed record for
each camera, and provides processed live preview, JPEG/PNG snapshots and
MP4/H.264 normal video or time-lapse. Camera has no generated frames or camera replay data.

For exact commands and JSON examples, see the
[CLI reference](CLI_Reference.md#camera-live-workspace-and-recording) and
[MCP reference](MCP_Reference.md). These describe the shared Core interfaces,
including generic workspace submissions and current transport differences.

## Appliance setup

Install the indexed Camera bundle into the integration catalog, then install
and enable **Camera** under **System > Plugins**. The NILO service account needs
access to the selected `/dev/video*` capture node. On the current Pi bench the
service account is in the `video` group. Appliance package staging includes
`60-nilo-instruments.rules` (group `video`, mode `0660`), and its service unit
uses that supplementary group. The current bench bundle requires Python 3.13
on the target, plus FFmpeg and ffprobe for recording. Self-contained bundles carry
the encoder tools in their verified runtime. Press **Discover cameras** after
connecting a device. A new `cam-N` record must be configured by an
administrator before capture. Name and purpose are persistent metadata; the
record ID remains the address for jobs and history.

For a self-contained appliance bundle, set `NILO_BUNDLE_CAMERA=1` for
`02_Tools/scripts/build-appliance.sh` and supply architecture-specific
`NILO_CAMERA_RUNTIME_ROOT`, `NILO_CAMERA_WHEELHOUSE_ROOT` and
`NILO_CAMERA_DEPENDENCY_ROOT` directories. The builder checks the target
interpreter architecture and pinned camera wheel digests before indexing the
bundle. These inputs must be prepared and verified before the offline build.
The offline release candidate builder requires a lock-bound
`camera-build-input` archive for each target. Each archive supplies
`dependencies/`, `wheelhouse/`, and `THIRD_PARTY_NOTICES.txt`; the Camera
bundle embeds those notices and the verified target interpreter. The current
release input lock is incomplete, so a production candidate cannot yet be
built from this checkout.

Discovery selects a capture-capable endpoint and excludes metadata-only nodes.
It records available formats, sizes and controls. A camera without a reliable
serial binds to its physical USB port. Keep serial-less cameras in fixed ports;
moving one creates a new record and leaves the earlier record absent.

At service start and after a reconnect discovered by NILO, a configured camera
returns to its stored control baseline before it becomes ready for jobs. A
failed reset leaves the camera unavailable for capture. Camera controls are
state changing; a snapshot may also change controls when it includes overrides.

## Use

The Camera workspace under **Instruments** shows configured, enabled camera records. The
administrator-only **System > Plugins > Camera** page manages plugin enablement,
approval policy, discovery, camera names and camera enablement. Advanced package
and device settings remain available through the authorized configuration API.
The header shows the installed version. **Update plugin** is enabled only when a
newer verified bundle has been staged in the local catalog; it needs no internet
connection. **View activity** opens the Camera package activity filter.

Each enabled camera has **RealTime View** and **Recordings**:

- RealTime View starts an owned live session, takes snapshots and starts/stops
  recordings. Normal, fill-workspace and browser fullscreen modes retain capture
  controls. The normal approval policy applies to start and capture commands;
  Stop needs no new approval. A recording continues after closing the browser,
  until its configured limit or explicit Stop.
- RealTime View includes three initially collapsed sections: Camera settings,
  Overlays and Hardware controls. Its title and capture buttons share one wrapping
  row. These sections provide advertised resolution, capture format/frame rate, a zoom
  slider, effects, timestamp/name/custom-text overlays and the camera's hardware
  controls. Read, Apply and Reset operate on the active physical device.
  Changes are shown in the live preview and embedded in saved files. An active
  recording freezes its settings; stop it before changing them. Advanced
  composition retains crop, rotation, lines, rectangles and calibrated scale bars.
- Recordings provides image quality/encoding, video quality and normal/time-lapse
  settings, a paginated thumbnail library, playback, export and confirmed deletion
  of individual or selected items. Start live view to apply output settings.
  Time-lapse has separate acquisition interval and playback frame rate.

Only modes within the 2,000,000-pixel processing bound and integer 1–30 FPS are
offered. Preview is bounded to 960×720. Actual unique-frame throughput depends
on the camera, resolution and effects; normal video maintains real elapsed time
by duplicating frames when needed. Sessions expire after 55 minutes and idle
previews stop after 30 seconds. At most two camera streams can be active.
The recording limit is shortened when the session has less time remaining.

Administrator retention defaults are 100 snapshots and 2 GiB per camera, with
each video stopped at 500 MiB or 10 minutes acquisition time. Core reserves
storage before capture and protects the greater of 2 GiB or 10% free disk space.
All captures share one library and one retention policy, regardless of capture date
or storage layout. Existing files remain in place and are eligible for the same
oldest-first automatic cleanup. Policy changes show eligible counts
and bytes; eviction happens on subsequent capture admission. Incomplete captures
remain accounted for and can be deleted explicitly. Active downloads block deletion.

The Instruments Camera group expands its configured devices and does not open an
overview page. Open RealTime View or Recordings directly under a named camera.
Hardware controls are generated from descriptors returned by the physical device.
A scale bar requires the camera's millimetres-per-pixel calibration setting.
Text and scale labels use the DejaVu Sans font bundled with the Camera package.
The frame interval selector appears when the physical camera advertises an
interval for the chosen format and size. The runner verifies the selected
interval against V4L2 and rejects a changed or unavailable setting before
retaining an image. The completed result records the capture time, selected
endpoint, effective interval, warm-up frames discarded, requested and actual
controls, output size and configuration revisions.

The generic job route accepts `plugin_id: "camera"`, an operation, a `cam-N`
device ID and a parameters object. Operations are `snapshot`, `get_controls`,
`set_controls`, `reset_controls`, and `workspace`. Workspace commands include
start, settings, controls, snapshot and record_start; their settings revision
binds the approval to the requested configuration. `stream` is Core-owned and
cannot be submitted by callers. `discover_devices` is the explicit plugin
control call. The CLI exposes `nilo bench submit --plugin camera` and
`nilo camera snapshot|controls`. MCP exposes corresponding camera tools. Normal
Bench permissions, approval policy, owner scoped job reads and artifact digest
verification apply.

Workspace endpoints live under `/api/v1/cameras/{id}` (workspace, frame,
media, stop and retention), with authorized media content/deletion under
`/api/v1/camera-media/{id}`. The CLI includes `camera workspace-status`,
`stop-live`, `stop-recording`, `media`, `media-download`, `media-delete` and
`retention`; MCP exposes the corresponding state/stop/media/retention services.
Browser exports use the browser's download/save settings.

Captured input and output are bounded by package and administrator limits.
Images retain no source EXIF metadata. A preview is a smaller transient view of
the retained image and is not a separate evidence artifact.

## Validation

The Pi hardware probe records USB identity, capture and metadata nodes,
formats, controls, wheel hashes and a real frame under
`.agentic/evidence/camera/pi-webcam/`. The package tests use isolated V4L2 test
doubles; physical acceptance uses attached cameras. See
`02_Tools/tests/camera/` for the discovery and capture probes.
