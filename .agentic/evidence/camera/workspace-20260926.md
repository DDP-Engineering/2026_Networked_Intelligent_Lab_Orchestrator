# Camera workspace candidate — 2026-09-26

## Delivery state

**Deployed for live testing** on `ddp-remote` at `10.205.148.11:8443` after
connectivity returned. The Core, GUI and catalog/installed Camera bundle were
updated together; migration 021 is present. NILO and the camera report healthy.
Credentials, historical captures, device configuration and the currently observed
`auto` approval policy were preserved. No commit or push was made.

Full rollback backup: `/var/backups/nilo-camera-workspace-20260926-143305`.
It includes the original binary, GUI/catalog and a complete state archive with
ownership/permissions. The one-off installer used a stopped-service bundle
replacement and conditional digest update with the unchanged package state schema.
The first install attempt rolled back safely because its readiness check used a
firewall-blocked loopback address and compared the HTML before Core's runtime-tag
injection. The corrected check verified the served immutable JavaScript from the
workstation and returned its hash over authenticated SSH. Logs remain on the Pi:
`/home/dpasare/deploy-camera-candidate-v2.log`.

Scope and decisions: [camera workspace plan](../../specs/camera-workspace-plan.md).
Requirement `REQ-FUNC-9fe624bc` remains draft pending integrated target acceptance.
Work followed the user's instruction to omit AEGIS stages.

## Local verification

- Complete Go suite `go test ./...` passed during implementation. Following final
  lifecycle/recovery changes, Bench, HTTP API and MCP package tests passed again;
  the final missing-thumbnail recovery test also passed in the Camera subset.
- Offline pinned Linux build environment
  `noble-20260905-01`, image digest
  `sha256:88ec9769aaba4398d64407cf326c84d5c49be8e0f72359ed7382d182f3aef2dc`:
  frontend lint, formatting, all **542 tests in 43 files**, TypeScript and Vite
  production build passed. ARM64 Linux Core cross-build passed.
- Camera package and bundle tests: **27 passed**. Ruff passed for the camera
  package, bundle builder and physical workspace probe.
- `git diff --check` passed. Existing tracked-file line endings were preserved.
- Camera bundle inventory was regenerated and every indexed file hash read back.
- Vite reports the existing class of large-chunk warning: the candidate main
  script is approximately 701 kB before compression. No build failure occurred.

Coverage includes ownership, active-download deletion protection, disk admission,
retention eligibility, reservations, recovery of verified files, quarantine of
corrupt/missing-thumbnail captures, revision-bound commands, path rejection,
hardware-control rendering, bounded resolutions and Stop without new approval.
These tests do not establish physical or visual acceptance of the final candidate.

## Physical feasibility evidence before connectivity loss

Target: `ddp-remote`, Raspberry Pi 4, FaceCam 1000X, physical `/dev/video0`,
MJPG 640×480. Existing target FFmpeg/ffprobe is
`7.1.5-0+deb13u1+rpt2`, with libx264 available. Existing Camera policy remained
`ask`; no historical captures were opted into automatic deletion.

1. A bounded direct FFmpeg capture with flip/inversion produced H.264 video:
   14.933333 seconds, 962159 bytes. Observed temperature was 65.244°C.
2. An early version of the new persistent workspace captured a real grayscale
   frame with a timestamp and recorded until its 20-second acquisition limit.
   ffprobe reported H.264, 15/1 output FPS, **19.6 seconds**, **1161689 bytes**.
   Recording finalized and the session returned to live view.
3. Output used `/home/dpasare/nilo-workspace-probe` on the main filesystem. An
   earlier attempt under `/tmp` was correctly stopped by the minimum free-space
   reserve because that RAM filesystem was too small. It is not a successful
   recording acceptance result.

The successful video ID was `480fe348-0dfe-4cea-a8d4-5ad6656e1667`; the probe
image ID was `6930571a-dd66-44da-8872-d76d93dc0f56`. Probe tooling/code remained
under `/tmp/nilo-workspace-probe`, separate from the installed application.
Normal video uses wall-clock timestamps and may duplicate frames: output FPS
does not mean 15 unique processed frames/second. Observed processing was about
7 unique frames/second at these settings.

The final runner records ffprobe's actual duration in metadata. The following
additional physical tests passed after connectivity returned:

- Normal-video probe: 20-second acquisition limit, final duration 19.133333 s,
  2085904 bytes, ID `45098bbe-b38c-4c7c-a4a2-94117b396f06`.
- Time-lapse probe: 20-second acquisition, two-second frame interval, 15 FPS
  playback, final duration 0.666667 s, 78406 bytes, ID
  `6b844654-ec5f-4221-85d2-a5fe5fb35d46`. Both probes finalized automatically and
  returned to live state without errors.
- Deployed GUI: live physical preview, grayscale/time overlay, brightness 0→5→0
  with actual device readback, snapshot during recording, explicit Stop recording,
  verified media indexing and browser video playback. The GUI video is
  `01a0dd7f-4eb1-7a71-a3b8-48a0bfdb0ae1` (27.466667 s, 2901564 bytes);
  snapshot is `01a0dd7f-a98d-7b1d-bc39-33305a0a611e`. Both remain available for
  the user's live test. Playback advanced with readyState 4 and no media error;
  timestamp and grayscale were visibly embedded in the saved video.
- Grayscale and time overlay were returned to their initial disabled values;
  hardware brightness was restored to 0. Historical captures were not deleted.
- A real-browser check found vertical cropping in Fill workspace. The CSS now
  positions the image within the available image area; the complete frame and
  bottom timestamp were then visibly confirmed. The updated GUI passed formatting,
  TypeScript and production build checks and was deployed. Desktop and 320/800/1024
  pixel layouts were checked; document width matched the viewport at each narrow
  size. Viewport overrides were reset.

Manual acceptance still needed: Chrome denied the automated Full screen request
(`not granted`), and the browser download-event check timed out. Neither is claimed
as passed. Destructive retention/deletion, unplug, conflicting controllers and
long-duration/thermal acceptance remain outside this smoke test. No HDMI or Ubuntu
hardware acceptance is claimed.

## Prepared installation payload

Ignored local artifact:
`02_Tools/cache/containers/outputs/camera-workspace/nilo-camera-workspace-arm64-20260926.tar.gz`.
It contains the Core binary, current GUI assets, the verified refreshed Camera
bench bundle and `candidate.sha256.json`. The bench bundle reuses the downloaded,
inventory-verified installed target dependencies. It requires the target's
existing Python 3.13 and FFmpeg tools; it is not a self-contained release archive.

| Item | SHA-256 |
|---|---|
| Payload | `67b5b0fd2bd951cc7236e28d7e7f2a39ae073e4982308de865afa7a7d5fe2894` |
| Core binary | `b10ae9b528483a2dcbf6e7d58119f6cec2554fac111431222c9e4f81adca500f` |
| Camera inventory | `c0cb206c67a1dcb5fe76831d43666e5ddf697afe8c9f259258969a67020cfc4b` |
| GUI index | `ad4bd6630188d5f5e190b6f282cf05edacc3b801bbe8a3267c2c9a0b38d464ab` |

Project version remains 0.9.0; this is an Unreleased candidate, not a versioned
release. The production offline input lock remains incomplete as documented
before this work.

The final GUI fit correction is a separate verified overlay,
`02_Tools/cache/containers/outputs/camera-workspace/gui-fit.tar.gz`, SHA-256
`65157fa65a2d837bb6337827c8facae0de63c58bf652745d61e92b4089fa47e1`.
Final GUI index hash: `e88a9b727369b66bae521f6b03ad4583c110f97001ea849e82b3490c00e7540d`.
Served CSS `/assets/index-BFOtoFvn.css` matches its build hash
`9aaedd75153b41770d2e32819d712f93d100e02fb959917eabfb96091ef7f3fb`.
Final JavaScript `/assets/index-D0pmSQDT.js` retains content hash
`1eabfb9acdac4bc5e474850e1fb27db18e7e83f2355a6051d1457881b01a61f2`.
Core and Camera inventory hashes are unchanged from the table above.

### Final shutdown correction

Live testing found that immediate cancellation after a successful Stop command
could race the runner's final reply and trigger a false browser timeout. Core
now permits graceful completion and cancels only on failure/request cancellation;
the stop request allows bounded finalization time. Stale-preview warnings are
hidden when the stream is intentionally stopped. A focused shutdown regression
test, Bench/HTTP/MCP tests, all 542 frontend tests, lint/format and ARM64/GUI builds
passed after this correction.

The final complete payload supersedes the initial archive plus GUI overlay:
`02_Tools/cache/containers/outputs/camera-workspace/nilo-camera-workspace-arm64-20260926-final.tar.gz`.
It was transferred, checksum-verified and installed successfully, preserving the
same state and `auto` approval policy. Incremental rollback backup:
`/var/backups/nilo-camera-workspace-20260926-144900`; original pre-feature backup
remains `/var/backups/nilo-camera-workspace-20260926-143305`.

| Final item | SHA-256 |
|---|---|
| Payload | `b0db4d5ad10082f3fa0f880ac60270486a52d6788d6f95d7955cb5d60fc96765` |
| Core | `5f7a12b00da83a4c9b590e3b86f99f89a258729f6eb9b5a1c90b83bbeea622d6` |
| GUI index | `882de299ea2f44eaccbf146635270737d0452710f997b07b6f4d99ce16e4581d` |
| Served `index-D4zFrnb0.js` | `b62b7c39ee16bc4456dc892eafdfa43bed8865a1dec50be84f0b021f5c6bdada` |

Final CSS and Camera inventory retain the hashes recorded above. Final installer
log: `/home/dpasare/deploy-final.log`.

Post-deployment Start/Stop was repeated on the physical camera: Chrome returned
to Stopped with Start live view available and no error/stale warning; `fuser`
reported no process holding `/dev/video0`, and `nilo.service` remained active.
The browser was left on RealTime View with viewport overrides cleared.

## Deployment procedure and remaining acceptance

### Follow-up: unified library and compact RealTime View

The user replaced the initial protection of pre-workspace snapshots with one
library and retention policy for all captures. Media queries now cover both
storage layouts, with shared filtering, pagination, ownership, read leases and
deletion. Existing files remain in place; deployment itself performs no cleanup.
The separate Settings destination is removed. RealTime View includes initially
collapsed Camera settings, Overlays and Hardware controls. Its title and capture
buttons share a wrapping row; expanded display keeps the controls available.

Verification: Bench, HTTP API and MCP Go tests passed, including new mixed-layout
pagination, ownership, original/thumbnail reads, deletion and retention tests.
Frontend lint, formatting, TypeScript/build and 544 tests in 43 files passed.
Chrome on the Pi verified the single title/toolbar row, all three collapsed
sections, hardware expansion, Fill workspace/Normal view, and the unified library
and retention explanation. At 320px, all buttons wrapped and document width was
320px without horizontal overflow. Viewport overrides were cleared and live view
was restored. Browser fullscreen/export acceptance remains as noted above.

Deployed core and GUI; the Camera bundle and approval policy (`auto`) were
unchanged. The stopped-service backup is
`/var/backups/nilo-camera-unified-20260926-171647`. All eight original/thumbnail
files had identical hashes before and after deployment; service is active.
Payload: `camera-unified-20260926.tar.gz`, SHA-256
`6734df39b8b3fa3fa96194abf22f8a03596a4e28b514567aa732775e273e404e`.
Core: `e539151d59bbda3aeabb57fd942da50d4950920fa3d8b1f33f7bbc7a5d319262`.
Served JS (`index-C7NdXzfE.js`) was checksum-verified over appliance HTTPS:
`42875773b2474e6b677b903713272cad2b54bb3837e98dacddba2420c24d4133`.

### General acceptance procedure

1. Confirm restored target identity and no active user capture. Verify the payload
   hash after transfer. Preserve credentials, device settings and all existing media.
2. Back up `/usr/local/bin/nilo`, `/opt/nilo/gui`, catalog/installed Camera bundles
   and a consistent `/var/lib/nilo` database/state before replacing anything.
3. Deploy Core, GUI and the verified Camera bundle together. Installed bundle
   digest/state must match Core's verified package metadata; use the managed
   upgrade path or a documented stopped-service migration with rollback backup.
4. Verify service readiness, served GUI assets, installed bundle inventory and
   migration 021. Preserve the current approval policy.
5. Run the latest physical probe with normal and `--timelapse` modes. Check
   ffprobe duration, processed appearance, time/size stop, cancellation and camera
   release. Use explicitly disposable capture data for quota/deletion testing.
6. Test the actual GUI: hardware controls, effects, snapshot during video,
   library playback/export, view modes, reconnection, controller conflicts,
   unplug/restart and 1920/1024/800/320-pixel layouts. Record final acceptance and
   rollback location here. Do not mark deployment complete from build success.
