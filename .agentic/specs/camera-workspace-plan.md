# Camera workspace implementation plan

Status: implementation candidate, 2026-09-26. Product scope and storage defaults
are agreed. The candidate is deployed to the Pi for live testing. Physical
normal-video/time-lapse probes and the main browser capture flow passed; the
evidence record identifies manual and extended acceptance still outstanding.
Work proceeds without the AEGIS workflow, as requested.

## 1. Agreed outcome

Each configured, enabled camera keeps its assigned name and stable `cam-N` ID,
and offers two work destinations:

| Destination | Responsibilities |
|---|---|
| RealTime View | Live image, Take snapshot, Start/Stop recording, normal/fill-workspace/fullscreen display; collapsed Camera settings, Overlays and Hardware controls |
| Recordings | Image/video output settings, normal-video/time-lapse settings, thumbnail library, playback, deletion, export and retention controls |

The user accepted the name Recordings for the combined image/video library.
The Instruments Camera entry is an expandable navigation group only; it has no
overview page. Its configured devices open RealTime View and Recordings directly.
System > Plugins > Camera retains administrator discovery, naming, device and
plugin enablement, and approval-policy management. Disabled or unconfigured
devices remain discoverable there rather than becoming instrument pages.

Effects and overlays must be embedded in saved snapshots and videos as well as
shown live. Both normal video and time-lapse are required. Audio capture is
outside this request. No generated camera feeds substitute for physical tests.

## 2. Baseline before implementation

Sources observed when this plan was agreed (the implementation below supersedes
these baseline gaps):

- `05_Software/integrations/camera/manifest.json` exposes discovery, snapshot,
  get/set/reset controls and only JPEG/PNG artifacts. Snapshot timeout is 35 s.
- `src/nilo_camera/v4l2.py` inside that integration opens a stream, discards two
  frames, returns the third, and closes the stream. It supports MJPG/YUYV and
  validates advertised size and frame interval. Repeating this operation is
  not the proposed live-preview or recording design.
- `src/nilo_camera/processing.py` already provides bounded crop/zoom, rotation,
  flips, grayscale, invert, markers and text rendering with a bundled font.
  Dynamic timestamp/name overlays and selectable text backgrounds need work.
- `05_Software/core/internal/bench/camera_adapter.go` provides stable managed
  camera discovery and capability validation. Current image paths cap decoded
  images at 2,000,000 pixels; do not advertise larger usable modes without
  revisiting every capture, processing and preview bound.
- `camera_preview.go` generates bounded previews of retained still artifacts;
  this is not a live transport. Bench job reads are owner scoped with existing
  administrator access. Retain these boundaries for the new library.
- `05_Software/frontend/src/components/pages/CameraSection.tsx` combines
  snapshot configuration, controls, image viewing and transaction history.
- `.agentic/evidence/camera/pi-webcam/README.md` records physical MJPEG/YUYV
  snapshots and control/cancellation tests on Pi 4 with FaceCam 1000X. It does
  not establish sustained preview, encoder throughput, latency or thermal limits.
  HDMI and Ubuntu target cells were not run.

New work therefore spans the Camera runner, Core lifecycle/API/storage, frontend,
offline dependency packaging, CLI/MCP access and physical verification.

## 3. Workspace behavior

### RealTime View

- Maintain aspect ratio and show connection/stream state clearly. Never present
  the last frozen image as a continuing live feed; label it stale on failure.
- Normal view centers a bounded image; fill-workspace uses the right pane while
  retaining navigation; fullscreen uses the browser fullscreen mechanism.
  Escape restores the previous mode. Capture/stop controls remain reachable in
  every mode and wrap on field displays. Unsupported fullscreen has a clear fallback.
- Display active recording mode, elapsed acquisition time, bytes written and
  remaining limits. Start is idempotent and Stop remains available during encoding
  or a busy camera; repeated Stop requests have the same outcome.
- Snapshot while recording consumes a frame from the same session, with its own
  artifact identity, permissions and approval decision.
- Opening live preview requires the normal authorization and policy decision
  once per bounded session, not once per frame. A visible Stop live control
  allows releasing the device without closing the page.

### RealTime View settings

- One resolution selector shows only supported width-by-height pairs for the
  selected camera format. Keep capture format separate from image/video encoding.
- Show supported frame rates/intervals and the camera's hardware controls:
  brightness, contrast, saturation, gamma, hue, sharpness, white balance,
  exposure, gain, backlight compensation and power-line frequency where advertised.
  Build each control from its reported type, range, step, default and read-only
  state; automatic modes disable their dependent manual controls. Show effective
  values returned by the device, retain Read/Apply/Reset controls, and explain a
  refused or clamped setting. Apply changes to the active session so their effects
  are visible in the live preview without taking a saved snapshot.
  Unsupported controls are omitted or explained. Zoom uses a slider plus a readable numeric value.
- Grayscale, invert, horizontal flip, vertical flip, crosshair and grid are direct
  controls. Preserve existing advanced composition features in a disclosure.
- Date/time, camera name and custom text have independent enablement, corner or
  center-edge placement, margins, text size/color and transparent/solid background
  settings. Use capture-time timestamps with an explicit timezone selection and
  metadata in UTC; time-lapse stamps acquisition time rather than playback time.
- Use one versioned processing configuration. Debounce slider changes, acknowledge
  the applied revision, and render that revision server-side for every output.
  Client-only CSS effects must not make preview differ from saved media.
- An active recording freezes its capture mode and processing configuration.
  Explain that Stop is required before applying those changes; this avoids a
  video whose appearance or dimensions change unexpectedly. When idle, changes
  affect the running preview after the server acknowledges them.
- Saved defaults are revisioned; other clients cannot silently overwrite them.
  Device calibration/baseline remains administrator configuration. Working
  settings require capture/control permission and exclusive controller ownership.

### Recordings

- Separate output settings from the library using compact sections/disclosures.
  JPEG exposes quality; PNG does not show a meaningless lossy-quality setting.
- Video container/codec choices come from verified installed encoder capabilities.
  A browser-playable output is required; MP4/H.264 is a candidate, not a promise
  until Pi performance and pinned offline packaging are verified.
- Normal video has a requested frame rate and quality/bitrate setting. Time-lapse
  separates acquisition interval from playback frame rate and reports both elapsed
  wall time and resulting playback duration. Never build an unbounded still-file queue.
- Paginated thumbnail grid with image/video filters, time, name, dimensions,
  duration and size. Images open a larger viewer; videos support playback/seeking.
  Video thumbnails come from a decoded recorded frame.
- Individual and selected-item deletion show a confirmation and handle partial
  failures. Download/export saves the original authorized artifact using the
  browser's save behavior; the server cannot select a path on the user's computer.
  A browser may download directly according to its own settings rather than prompt.
- Storage usage and administrator retention settings are accessible here. Ordinary
  users see only their authorized items and quota status; they cannot change
  appliance limits or delete another actor's captures.

## 4. Session, media and API architecture

1. Core owns authorization, policy decisions, lifecycle state, quotas and audit.
   The Camera runner owns physical capture, frame processing and encoding.
2. One supervised capture session per camera owns the V4L2 endpoint. A small
   bounded frame pipeline fans out to preview, snapshots and the video encoder.
   Preview drops old frames for slow clients rather than growing a queue.
3. Start with a measured same-origin authenticated JPEG preview transport
   candidate. Media bytes use a dedicated bounded private channel from runner to
   Core; NDJSON remains control/events, not an unbounded base64 video transport.
   Select the exact channel/transport after the feasibility measurements.
4. Live sessions have renewable viewer leases and a short idle shutdown. Recording
   is a durable appliance operation that continues after browser disconnect until
   explicit Stop or its limit. Reopening the page attaches to its real state.
   Preview viewing does not give another viewer control of the camera.
5. Long recordings do not occupy the shared finite-job workers indefinitely.
   Add explicit session management and resource accounting instead of extending
   the existing snapshot timeout to hours. Preserve short-job audit/idempotency.
6. Specify lifecycle states: starting, live, recording, stopping, finalizing,
   stopped and failed. Stop must finalize within a bounded interval; timeout,
   unplug, encoder failure and restart preserve only verified playable output,
   with partial/incomplete status where necessary. Never silently resume recording
   after a process or appliance restart.
7. New API contract groups: capabilities/defaults; start/read/stop live session;
   bounded preview subscription; processing update; snapshot; start/read/stop
   recording; paginated media metadata; thumbnail/content/export; delete; and
   retention read/update. Exact paths and schemas are frozen before implementation.
   CLI/MCP invoke the same services for settings, capture, recording and media
   operations; human rendering/fullscreen stays browser-only.
8. Session creation/mutation uses authenticated CSRF-protected requests; read
   streams do not expose credentials in URLs. Enforce ownership, revocation,
   expiry, origin checks, subscriber limits and request-size/rate limits.
   Metadata, thumbnail, range playback and export share artifact authorization.
9. An operator manages their own work; viewers read only permitted media/streams;
   administrator has explicit takeover/stop and retention rights. Sharing another
   user's live feed requires an explicit authorization contract, never inference
   from knowing a camera ID. Final permission names need contract review.
10. Disable/uninstall/rebind must respect active sessions: refuse with actionable
    state or perform an explicitly requested stop-and-finalize before changing the
    device. Streaming cannot bypass existing managed-device readiness/baselines.

## 5. Storage and retention proposal

The user approved these initial defaults on 2026-09-26:

| Setting | Approved initial value |
|---|---|
| Snapshot count per camera | 100 completed snapshots |
| Total media bytes per camera | 2 GiB, including previews and temporary/finalizing output |
| Maximum one video | 500 MiB or 10 minutes acquisition time, whichever occurs first |
| Appliance free-space reserve | Greater of 2 GiB and 10% of the media filesystem capacity |
| Existing captures | Same library and automatic retention as all other captures |

The 10-minute default is wall-clock acquisition time for both modes, not encoded
playback length. A longer time-lapse requires an explicitly larger configured
duration while the byte and free-space limits remain enforced.

- Limits describe disk storage. RAM is separately bounded by queues/frame sizes.
- Count and byte limits both apply. New captures enrolled in an approved retention
  policy evict oldest eligible completed media by capture time, with stable ID as
  tie-breaker. Never evict active recordings or artifacts
  held by an active bounded export/download lease.
- All captures count toward usage and completed files are eligible for oldest-first
  retention. Active downloads and unfinished captures remain protected. If protected
  data prevents fitting a new capture, refuse it with a clear explanation.
- Administrator policy changes show the number and bytes of affected media before
  confirmation. Retention operates across authorized appliance storage; metadata
  and audit state who configured it and why each artifact was removed.
- Reserve storage atomically before admitting a recording or snapshot across
  cameras. Account for thumbnails, metadata, partial files and encoder/container
  finalization overhead. Stop early enough that the final file respects its cap.
  Free-space checks continue during capture because unrelated processes may write.
- Deleting media removes its content and previews, while keeping a compact audit/
  transaction tombstone. Downloads cannot race file removal. Interrupted deletion
  resumes safely after restart; retries do not target a different artifact.
- A recovery sweep reconciles unfinished media, reservations and orphaned temporary
  files inside the dedicated media root. It never follows symlinks or deletes
  unrelated integration evidence. Database/file failures remain visible.

## 6. Implementation order and completion criteria

| Step | Deliverable | Required evidence before moving on |
|---|---|---|
| 1. Physical feasibility | Bounded probe of current Pi camera and candidate encoders/transports | Sustained capture, processed preview plus recording, CPU/RAM/thermal/disk/bandwidth, latency, disconnect and cancellation measurements |
| 2. Freeze contracts | Session/resource/policy model, media schema, settings revisions, retention rules and pinned codec choice | Resolve measured limits and storage defaults; review API/schema against existing roles and integration protocol |
| 3. Capture engine | Persistent stream, shared composition pipeline, normal video and time-lapse, snapshots during recording | Runner/component tests; real Pi videos decode and contain the selected effects/timestamps |
| 4. Core/media services | Lifecycle, authenticated preview/playback, media index, reservation, deletion and retention | Ownership, concurrent admission, disk-full, restart, revocation, race and recovery tests |
| 5. Three-page GUI | Live view modes, settings preview, recording library and output/retention controls | Responsive and accessibility checks, EN/DE/RO, keyboard/touch, actual API lifecycle and error states |
| 6. Package and live acceptance | Reproducible offline bundle, deployment backup and Pi live test | Full impacted suites/build; physical end-to-end acceptance and verified served assets |

Step 1 must not alter persistent camera controls or delete media. Use a bounded
temporary probe directory and stop at a preset duration/space limit. If current
camera work is active, defer capture probing rather than interrupt it. Any new
runtime dependency is pinned and staged in repository-local tooling, not installed
globally without a concrete deployment need.

Pi acceptance includes: live settings response; all view modes; preview/snapshot/
video composition agreement; normal/time-lapse playback; capture during recording;
browser close/reconnect; two viewers and conflicting controllers; camera unplug;
backend/runner restart; quota/count eviction on explicitly disposable test media;
size/time stop with playable output; deletion/export; and 1920px, 1024x600,
800x480 and 320px compatibility. Measure sustained use, not just the first frame.

Existing performance gates for HDMI and Ubuntu remain unverified until those
targets are available. The first implementation can be evaluated on the current
Pi/webcam; it must not advertise acceptance of unavailable target combinations.

## 7. Implemented decisions and remaining acceptance

- Core supervises one process per owned camera session, with a maximum of two
  streams, a 55-minute hard session lifetime and 30-second preview idle timeout.
  Recordings continue without a viewer and stop at the earlier recording/session
  limit. The GUI displays session expiry and actual recording limits. Longer
  continuous time-lapse runs require further lifecycle work.
- Physical V4L2 frames share one bounded processing path. Same-origin authorized
  JPEG polling reads at most 960×720 previews through a private Unix socket;
  source dimensions remain at most 2,000,000 pixels and integer 1–30 FPS.
- FFmpeg libx264 produces fragmented MP4 with bounded finalization. Normal video
  uses wall-clock timestamps/CFR; time-lapse uses acquisition intervals and a
  separate playback frame rate. ffprobe verifies the final duration. The current
  Pi has FFmpeg/ffprobe 7.1.5-0+deb13u1+rpt2. Production self-contained bundles
  require verified encoder binaries in the runtime inputs; the pre-existing
  incomplete production input lock remains unresolved.
- `workspace` commands use normal Bench policy approval, owner scope and settings
  revisions. The Core-only `stream` operation cannot be submitted by users.
  Read routes require `bench.read`, mutations require `bench.execute`, and
  retention configuration requires `plugins.manage`. Owner/admin Stop is direct
  and does not wait for another approval. Another user cannot view an owned feed.
- Migration 021 stores settings, reservations, completed/incomplete media and
  deletion tombstones. All completed media are evicted oldest
  first at admission. Pending/incomplete data and active downloads stay protected.
- Public routes are `/api/v1/cameras/{id}/{workspace,frame,media,stop,retention}`,
  `/api/v1/camera-media/{id}` and its `/content` route for both storage layouts.
  The earlier `/api/v1/camera-legacy/{job-id}` deletion route remains compatible. CLI/MCP call the same services.
- The GUI implements two work pages, hardware controls, advanced composition,
  output settings, paginated/filterable media, selected deletion and retention
  confirmation. Controls wrap for narrow layouts. Physical/browser acceptance
  at the target viewports is still required.

Evidence: `.agentic/evidence/camera/workspace-20260926.md`. A real processed
640×480 normal-video probe produced a playable 19.6-second H.264 file. This is
feasibility evidence only: it does not establish final integrated acceptance,
time-lapse behavior, sustained target performance or other platform coverage.
Deployment to `10.205.148.11` completed after connectivity returned. The final
runner passed bounded normal-video/time-lapse probes, and the GUI passed live
capture, hardware-control readback, snapshot-during-recording and playback checks.
The evidence record contains backup paths, deployed hashes and outstanding manual
fullscreen/export and extended reliability acceptance.

## Unified library correction — 2026-09-26

The user superseded the initial opt-in protection for pre-workspace captures.
All captures now share one paginated, filtered library and one retention policy.
Compatibility with the artifact storage layout is internal; no file migration or
immediate cleanup occurs at deployment. Thumbnail, original export, ownership,
read leases and deletion are handled through the same media routes.

The user also consolidated Settings into RealTime View. Camera settings, Overlays
and Hardware controls start collapsed and open independently. The title and
live capture buttons share a row, wrapping on narrow screens; expanded modes
keep their own visible toolbar. Crosshair, grid and calibrated shapes belong in
Overlays; crop, rotation, zoom and image effects belong in Camera settings.
