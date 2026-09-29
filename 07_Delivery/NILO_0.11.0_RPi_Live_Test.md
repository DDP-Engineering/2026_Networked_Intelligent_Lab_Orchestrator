# NILO 0.11.0 Raspberry Pi live-test deployment

Date: 2026-09-27. Target: `ddp-remote`, Linux ARM64, `10.205.148.11`.

## Installed candidate

- Core and GUI report project version **0.11.0**.
- USB to Serial bundle 0.11.0 is installed and enabled with **Ask** session approval.
- The bench bundle uses the existing Pi Python 3.13.5 interpreter and includes
  verified ARM64 SDK dependencies and pinned pySerial 3.5. It is a live-test
  bundle; it does not include a self-contained interpreter closure.
- Camera configuration, captures, existing authentication and network settings
  were retained. The privileged helper and service configuration were retained.
- Git staging, commits and pushes were not performed.

## Verification

- Accepted build image digest:
  `sha256:88ec9769aaba4398d64407cf326c84d5c49be8e0f72359ed7382d182f3aef2dc`.
- Before deployment, buildinfo, Bench, HTTP and MCP tests passed inside the
  pinned Linux build environment. ARM64 compilation and production GUI build passed.
  The serial implementation's full strict test/lint result is recorded in
  [TASK-5344100e](../.agentic/tasks/TASK-5344100e.md).
- All 289 files in the reused ARM64 dependency input matched its inventory;
  the new delivery contains 183 checksummed application/bundle files.
- Transferred archive SHA-256:
  `1f3a075a98a2240dd6248a2b28526b4611d9bd4aca690f672293183352e305fb`.
- Deployed Core SHA-256: `4f5deb99fdb92627b41d1fddc6d526c1e209f16ce941f32dc91a8b85609f581c`.
- Initially served JavaScript `assets/index-Bh_B0H8q.js` SHA-256: `8c60041e47be1d082afdae542fe05c0f8fd2a9ae526f1223a178e2107e9b6ad9`.
- External HTTPS verified the certificate obtained through authenticated SSH,
  using the permanent `nilo-n0hzmysj9v.local` certificate identity. The page
  references the new assets and served JavaScript/CSS hashes match the candidate.
- Unauthenticated status returns 401; authenticated status and `nilo version`
  both report 0.11.0. Core and network-helper services are active.
- The serial persistence tables are present. Discovery, device configuration,
  workspace, retained-log listing and retention APIs passed on the real Pi.
- Both adapters are enabled, configured, present and ready:

| NILO device | Display name | Linux endpoint | USB ID |
|---|---|---|---|
| serial-1 | QinHeng USB Serial | /dev/ttyACM0 | 1a86:55d3 |
| serial-2 | CP2102 USB Serial | /dev/ttyUSB0 | 10c4:ea60 |

The service account already belongs to `dialout`. No serial connection was
opened and no payload transmitted during deployment verification. Physical
RX/TX, baud/framing, line-control and unplug/replug testing remain for the live
hardware session. Temporary verification tokens were revoked. The two existing
capture files retained their original checksums.

## Rollback

Backup: `/var/backups/nilo-0.11.0-20260927-092635`.
It contains the prior Core binary, GUI, full stopped-service state archive,
capture inventory and deployment readiness receipt. Restore the prior binary,
GUI and state together if rollback is needed; the previous build predates the
serial database migration. Current Camera and network state remain preserved.

## Open for testing

Use `https://10.205.148.11:8443/`, refresh the browser, then select
**Instruments > USB to Serial > device > RealTime View**. Administration is at
**System > Plugins > USB to Serial**.

## GUI review updates — 2026-09-27

The live Chrome review corrected serial settings cards, expanded-view sizing,
fullscreen exit, policy text width, Sessions alignment and obsolete read errors
after disconnect. The final GUI remains project version 0.11.0.

- Final JavaScript: `assets/index-nS8Dr3mb.js`, SHA-256
  `43edb1cbb89d52035993e62f3634961782a65ef345632a41d4175e3e9561b1aa`.
- Final CSS: `assets/index-BpQ1curW.css`, SHA-256
  `d9cdf3d24d8e71b356c12921837afb5216bff2a31785b31d6fdf25fa4505d0c7`.
- Final GUI archive SHA-256:
  `1d712ab318f04b2bf34541a4e7d252e4b257d839e14affa3918434c15e73bbf9`.
- Final rollback copy: `/var/backups/nilo-serial-gui-feedback-20260927-102037`.
  It contains the preceding GUI and stopped-service state snapshot. This update
  changed only static GUI assets; the Core binary and persistent state were retained.
- External pinned HTTPS verified the served assets; Chrome loaded the same final
  JavaScript. Direct DOM inspection confirmed five retained session rows, centered
  checkbox/date alignment, 8 px gaps and 44 px targets. Both adapter destinations
  were inspected; 1024x600 Fill workspace and fullscreen exit were checked live.
- Focused component/API tests: 14 passed. Responsive tests: four sizes passed.
  Final full strict gate: tests passed in 493.7s, lint in 10.9s, zero errors.
  Catalog warnings include earlier repository warnings and paths covered by the
  original serial task rather than this narrowly scoped follow-up.
- The user approved interruption of the active connection for deployment.
  The agent did not transmit serial payloads; physical RX/TX acceptance remains
  separate. The disconnect race is covered by deterministic regression tests.

Implementation and verification records:
[layout review](../.agentic/tasks/TASK-5344100e.md) and
[disconnect feedback](../.agentic/tasks/TASK-1ef49b01.md).

## Shared notification GUI deployment - 2026-09-27

The shared plugin/system notification UI and compact Camera/Serial headers are
deployed to `https://10.205.148.11:8443/`, retaining project version 0.11.0.

- The 23 candidate file hashes matched the final validated evolution record.
  Full tests passed in 397.8s and lint in 9.7s; the production GUI rebuilt successfully.
- Served JavaScript: `assets/index-Pw5O43tx.js`, SHA-256
  `818ed3e3d628a37dd0bcc44a73dcc6ab87b825fc45ad92b693b3e40cccd5537e`.
- Served CSS: `assets/index-BQ4cp2BL.css`, SHA-256
  `9f5d2ffb11bcd02bb223c92a54a00eb5bef030788a4e5528ecd061408d6f61f7`.
- Archive SHA-256:
  `f839666ec83e3aa3e72822ecbbadadbddf5eb3fa9cff9604daf142e77ae05aba`.
- Rollback backup: `/var/backups/nilo-notification-gui-20260927-171655`, including
  the preceding GUI and stopped-service state archive.
- Deployment preflight found no active serial connections, active bench jobs,
  or open camera/serial device endpoints. NILO was restarted for the GUI update.
- External certificate-verified HTTPS checks matched all served static assets
  and index references. `nilo` and `nilo-nethelper` are active; Core reports 0.11.0.
  The deployment verified the Core binary was unchanged and retained persistent state.
- Refresh the browser to load this build. No instrument payloads were sent and
  no physical I/O acceptance is claimed by this deployment.

Implementation: [shared notifications](../.agentic/tasks/TASK-e2a95ed4.md).

## Ready instruments deployment - 2026-09-27

Deployed the validated Core, GUI and refreshed Camera bench package to
`https://10.205.148.11:8443/`, retaining version 0.11.0. Instruments now filters
active plugins and configured, enabled, present, ready devices. Core polls
Camera/Serial presence automatically and propagates detected hardware failures.

- All 17 task candidate file hashes matched the final validated evolution.
  Final full tests passed in 414.0s, lint in 9.5s, strict exit 0.
- Built linux/arm64 Core and production GUI in the pinned, offline OCI environment.
  Camera dependencies were reused only after full base-bundle checksum validation;
  this remains a bench package using the Pi interpreter, not a self-contained release.
- Transfer archive SHA-256:
  `cb5b29c4b4637b111f2f1372cf4dbcfc1988060f2e3b7651f66dfc23f249458d`.
- Core SHA-256:
  `a962614d9440d7c6c84b08146273b7c383dc2d8440c192a2094d744d6487a159`.
- Served JavaScript: `assets/index-8kHlQ5Zp.js`, SHA-256
  `3c55f0ddd48ad6b16f0b2d99020d0bbbd93cf49f27dc7c42a2421cf4f1def84e`.
- Rollback backup: `/var/backups/nilo-ready-instruments-20260927-195922`.
  It includes the stopped-service state, prior Core, GUI and Camera catalog.
- Deployment found no active instrument sessions or open hardware endpoints.
  Core, GUI, Camera catalog and installed Camera package were replaced; saved
  captures and plugin approval settings were verified intact.
- Certificate-verified external HTTPS matched served assets. Authenticated
  inventory reports cam-2 and serial-1 through serial-6 ready; cam-1 remains absent.
  The temporary read-only verification token was revoked.
- All 298 deployed application/package files matched candidate hashes. Over a
  12-second observation, present devices advanced last-seen timestamps without
  changing their configuration revisions. Both NILO services are active.
- Physical unplug/replug acceptance remains for the live user test; no serial
  payload or new camera capture was requested. No Git changes were staged or committed.

Implementation: [ready instrument navigation](../.agentic/tasks/TASK-4a92f703.md).

## Backup retention cleanup - 2026-09-27

At the user's request, removed 40 older NILO backup snapshots under
`/var/backups`, reclaiming approximately 1.6 GiB. The retained rollback backup is
`/var/backups/nilo-ready-instruments-20260927-195922`; older backup locations
recorded above are historical and no longer available. Both services remained
active after cleanup. Current application state and saved captures were retained.

## Platform operating settings deployment - 2026-09-28

Deployed issue #25 from the uncommitted `task/25-platform-operating-limits`
working tree to the DDP Raspberry Pi 4 at `https://10.205.148.11:8443/`.
The project version remains 0.11.0.

- The candidate passed the AEGIS strict gate before deployment: full tests
  (406.0s), lint (10.0s), and 0 errors. Built the Linux ARM64 Core and production
  GUI with pinned repository tools. The Core SHA-256 is
  `18f91240b856b7e93713dba4bfeade76130898011530e808ea0f4ca72769a4df`.
- The 298-file transfer archive SHA-256 is
  `33f00e99dc7c095aef354c33cd6ed32f6d69f30cf982cca91cb1fa3bcf1bcbdd`.
  The Camera catalog was rebuilt from a verified base bundle; its installed
  292-file index SHA-256 is
  `ccd9aa93ad167224d5980dba6bc6471e143aea196549ce1b3386ee9e8071a96c`.
  The installed Camera package upgrade reported ready.
- The certificate-verified served JavaScript `assets/index-8KHtMGQf.js` matched
  SHA-256 `8fab64f59d0d21d98075833afd46137f5303bd494eddeff46a43d2c99b6aa94c`.
  The served CSS `assets/index-BuOjCF_x.css` matched SHA-256
  `951f90bc78fbfaeaa4cb6bcfb84d2bd88d6f8f077db77100568995e922b5d0ab`;
  the HTML referenced both.
- The new settings API returned the nine defaults with zero current usage.
  An authenticated live update raised `serial_connections` from 4 to 5 at
  revision 2; a second update restored 4 at revision 3. Both changes have
  consecutive `operating_settings.changed` audit events. Revision 3 and the
  restored defaults survived a NILO restart. Both `nilo` and `nilo-nethelper`
  are active. The temporary verification token was revoked, confirmed by HTTP
  401, and its local and remote secret files were removed.
- The predeployment Core, GUI, Camera catalog, and runtime state are backed up
  at `/var/backups/nilo-issue25-20260928-183607/pre-deploy.tar.gz`, SHA-256
  `2d5abf69f7845a1a791016db4f984c3072228bbbaa364df0118ac95672d410a6`.
  The matching rollback script is staged at
  `/var/tmp/nilo-issue25-nuNA6gLe/rollback.sh`.
- No instrument command, serial payload, or camera capture was issued during
  this deployment. Physical I/O acceptance remains for the live test. No Git
  changes were staged, committed, or pushed.

Implementation: [platform operating limits](../.agentic/tasks/TASK-95cf116e.md).

## Nilo Settings GUI refresh - 2026-09-28

Deployed the user-directed issue #25 browser revision to the same DDP Raspberry
Pi. Nilo Settings now shows the four platform job controls in an editable table;
Camera and Serial controls appear under their respective plugin administration
pages with contextual help. The Core settings API, database and Camera package
were retained.

- Final AEGIS strict gate passed: full tests (441.3s), lint (9.4s), 0 errors.
  The GUI transfer archive SHA-256 is
  `25ab070e2a20292a8d218c2317259ada509df8e4ea32f7c87ccd5098a2e3b566`.
- The certificate-verified served JavaScript `assets/index-ChpNCT5y.js` matched
  SHA-256 `bbb3ce838c415fa11d23e2645a2ff7abfb67a8a2ef279ddae872c03d681c8b6b`.
  The served CSS `assets/index-CmAWkln6.css` matched SHA-256
  `e1a8e19d5b422741a6a8a1686d28fb7cf03de560d17c8671a81e850e8353b1bd`.
  After restart, the served HTML referenced both assets.
- The GUI directory swap retained a rollback copy under
  `/var/backups/nilo-issue25-ui-20260928-195834`. The rollback script is
  `/var/tmp/nilo-issue25-ui-CqzPO9oV/gui-rollback.sh`. Core cached the old HTML
  index, so NILO was restarted to activate the new index. Preflight found zero
  unfinished jobs, zero active serial connections and no active Camera/Serial
  control sockets. Both `nilo` and `nilo-nethelper` are active after restart.
- Core and installed Camera package hashes were unchanged. The settings row
  remains at revision 3 with the default serial limit of four. No physical
  instrument command or capture was issued, and no Git change was staged,
  committed, or pushed.
