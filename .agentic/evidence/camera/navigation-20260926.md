# Camera navigation update — 2026-09-26

Removed the Instruments Camera overview destination. Camera is now an expandable
navigation group; clicking it does not open a workspace tab. Configured cameras
retain direct RealTime View and Recordings destinations. System > Plugins >
Camera still opens administration. Navigation containers are excluded from tab
lookup and available page IDs.

## Verification

- All 544 frontend tests in 43 files passed. After correcting a TypeScript-only
  test option, the seven navigation tests passed again in the pinned container.
- Final frontend lint, formatting, TypeScript and production build passed.
- `git diff --check` passed.
- Chrome on the Pi confirmed the Camera group expands/collapses without opening
  a tab, has no Overview entry, and opens RealTime View and Recordings directly.
- Restored the physical live preview after deployment; the browser shows Live
  with the timestamped camera frame and capture controls available.

## Deployment

- Appliance: `ddp-remote`, `https://10.205.148.11:8443/`.
- GUI-only update; Core binary checksum and camera approval policy (`auto`)
  remained unchanged. NILO service is active.
- Both retained media/thumbnail files matched their pre-deployment checksums.
- Backup: `/var/backups/nilo-camera-no-overview-20260926-231052`.
- Payload SHA-256:
  `49382efe1c3da43262778b1aeb913a725db43db03d0b29246f1956aeb61b274c`.
- Served JavaScript: `index-Bf00DwyQ.js`, verified over appliance HTTPS with
  SHA-256 `578caa8f6c440a1c43ec2059895e135cefe93ad2750c31d149ace5e0cd72b3be`.
