# Camera integration

The Camera integration uses physical USB cameras exposed by Linux V4L2. It
discovers capture nodes and hardware controls, keeps a stable managed record for
each camera, captures a JPEG or PNG snapshot, and retains the image as a Bench
artifact. Camera has no generated frames or camera replay data.

## Appliance setup

Install the indexed Camera bundle into the integration catalog, then install
and enable **Camera** under **System > Plugins**. The NILO service account needs
access to the selected `/dev/video*` capture node. On the current Pi bench the
service account is in the `video` group. Appliance package staging includes
`60-nilo-instruments.rules` (group `video`, mode `0660`), and its service unit
uses that supplementary group. The current bench bundle requires Python 3.13
on the target. Press **Discover cameras** after
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

The Camera workspace under **Bench** shows configured camera records. The
**System > Plugins** page manages the installed package, camera configuration
and package limits. Overview tiles show a bounded recent image and a quick
snapshot action; two accessible saved images can be compared side by side.
Snapshot width, height, pixel format, an advertised frame interval, crop centre,
digital zoom, rotation, flips, grayscale, invert, JPEG/PNG encoding, output dimension,
crosshair, grid, text, line, rectangle and calibrated scale-bar overlays are
available in the browser. A scale bar requires the camera's millimetres-per-pixel
calibration setting. Text and scale labels use the DejaVu Sans font bundled with
the Camera package, so rendering does not depend on an appliance font install.
Hardware controls are generated from descriptors returned by the physical device. The latest
retained image and per-camera history use authenticated bounded previews; the
viewer provides local zoom, pan and markers. The complete
artifact remains downloadable from its transaction.
Viewer rotation, flips and invert are local to the browser. A placed marker is
mapped through the viewer and requested composition into a bounded crosshair
overlay on the next retained snapshot.
The frame interval selector appears when the physical camera advertises an
interval for the chosen format and size. The runner verifies the selected
interval against V4L2 and rejects a changed or unavailable setting before
retaining an image. The completed result records the capture time, selected
endpoint, effective interval, warm-up frames discarded, requested and actual
controls, output size and configuration revisions.

The generic job route accepts `plugin_id: "camera"`, an operation, a `cam-N`
device ID and a parameters object. Operations are `snapshot`, `get_controls`,
`set_controls`, and `reset_controls`. `discover_devices` is the explicit plugin
control call. The CLI exposes `nilo bench submit --plugin camera` and
`nilo camera snapshot|controls`. MCP exposes corresponding camera tools. Normal
Bench permissions, approval policy, owner scoped job reads and artifact digest
verification apply.

Captured input and output are bounded by package and administrator limits.
Images retain no source EXIF metadata. A preview is a smaller transient view of
the retained image and is not a separate evidence artifact.

## Validation

The Pi hardware probe records USB identity, capture and metadata nodes,
formats, controls, wheel hashes and a real frame under
`.agentic/evidence/camera/pi-webcam/`. The package tests use isolated V4L2 test
doubles; physical acceptance uses attached cameras. See
`02_Tools/tests/camera/` for the discovery and capture probes.
