# NILO 0.12.0 Raspberry Pi live-test deployment

On 2026-09-28, deployed the uncommitted issue #25 candidate from
`task/25-platform-operating-limits` to the DDP Raspberry Pi 4 at
`https://10.205.148.11:8443/`. The project version was raised from 0.11.0 to
0.12.0 before building and deploying. The root `VERSION`, Core buildinfo and
frontend package/lockfile mirrors agree.

## Validation and installed files

- The final AEGIS strict gate for the Nilo Settings View activity change passed:
  full tests (436.6 seconds), lint (9.7 seconds), 0 errors. Following the
  separate version assignment, Core buildinfo and CLI tests, 73 frontend
  tests, and the production GUI build passed. The Linux ARM64 Core was built
  with the pinned repository toolchain.
- Transfer archive SHA-256:
  `2fee271b0049c48e4dafe7b8abed9ca09d7c9061dcb8d602ab3395f9dd227e6d`.
  Installed Core SHA-256:
  `6ef611c7ce0fc2980fdd7923768a5f7e433c9075877ce499b4553d5b42ae56c1`.
  `/usr/local/bin/nilo version` reports 0.12.0.
- The certificate-verified HTTPS index references
  `assets/index-BkKRGd-d.js` and `assets/index-CmAWkln6.css`. Their served
  SHA-256 hashes equal the production build:
  `a71703b3724e18ffe7a80fba9600fd43f07fce91fa951cbd5df9cd072a60ccae`
  and `e1a8e19d5b422741a6a8a1686d28fb7cf03de560d17c8671a81e850e8353b1bd`.
  Core inserts the `appliance` runtime-profile meta tag into the served index,
  so its hash differs from the on-disk build index as expected.
- `nilo` and `nilo-nethelper` are active. The Camera catalog was not changed;
  its installed `files.sha256.json` remains at SHA-256
  `ccd9aa93ad167224d5980dba6bc6471e143aea196549ce1b3386ee9e8071a96c`.

## Persistent state and recovery

The preflight found zero unfinished jobs and zero active serial connections;
no `ffmpeg` process was active. The version update retained the existing
settings row at revision 4, including its serial connection limit of 2. The
predeployment backup contains the identical row. Audit sequences 903, 904 and
910 are `audit` / `platform` / `operating_settings.changed`; sequence 910 and
revision 4 predate this deployment. Verification did not edit settings or
create an audit event. No physical instrument command, serial payload or camera
capture was issued during deployment.

The stopped-service backup of the previous Core, GUI and runtime state is
`/var/backups/nilo-0.12.0-20260928-174744/pre-deploy.tar.gz`, SHA-256
`fb8e6dafe86f31527e27e22c81381ce512ae54e312a7872770b7fc8fda6c6ff3`;
checksum readback passed. The previous Core and GUI also have direct copies in
that backup directory. After a zero-work preflight, the reversible Core/GUI
rollback command is
`sudo sh /var/tmp/nilo-0120-mFFgDpsw/rollback-0120.sh`. It retains current
runtime state; the archive preserves the complete predeployment state if a
full restore is needed.

No Git files were staged, committed or pushed. Physical I/O acceptance remains
for a separate live test.

Implementation: [platform operating limits](../.agentic/tasks/TASK-95cf116e.md).
