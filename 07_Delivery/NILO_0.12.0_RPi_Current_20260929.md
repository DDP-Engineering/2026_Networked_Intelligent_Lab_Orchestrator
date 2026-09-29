# NILO 0.12.0 current-source Raspberry Pi deployment

On 2026-09-29, rebuilt and deployed NILO 0.12.0 to the DDP Raspberry Pi 4
at <https://10.205.148.11:8443/>. Production source is merge commit
`0d184978432173596994588ca7931f3ada40a91a` on `feature/38-add_ci`, containing the latest
master `59fb23716b8c636f6c8d0e0de5ba35d16a71bdda`. The local script simplification and
planning work remain uncommitted and were preserved through the merge.
This was a repository and delivery operation; no AEGIS task was opened.

## Build and validation

The isolated build used accepted image digest
`sha256:84578c0371424ee15cf43bb78083cadd45e9bda2ba2bdd6926f0c00b1d0f0568` with networking disabled and dropped capabilities.
The merged source's locked Go, Python and npm input coverage was verified;
Python installations and npm preparation ran offline.

- 218 tooling tests passed, with 35 explicit platform/acceptance skips.
- Go amd64 tests, ARM64 compilation checks and Go vet passed.
- 112 Python integration and contract tests passed.
- 596 frontend tests across 49 files passed. Formatting, lint and the
  production GUI build passed, retaining the existing bundle-size warning.
- ARM64 Core and helper ELF headers were checked before installation.
  Camera native dependency imports passed on the Pi without instrument I/O.

Master updates the frontend lockfile version and its recorded digest. The
merged environment lock therefore differs from the accepted image's embedded
lock. Validation invoked the existing `verify-offline.sh` suite directly in
the verified image; the public `test --in-container` exact-match guard remains
unchanged. A new accepted image/archive generation is still required for that
public/CI route. This run does not claim archive acceptance or a Gitea CI pass.

Build log: `02_Tools/cache/rpi-smoke-20260929/build-merged.log`, SHA-256
`b74e8ea0d2ef6188fcc9100ded288794a77896429530a1a506b3d18fbf72d4e8`.

## Installed payload and HTTPS checks

The transfer archive SHA-256 was verified on the Pi:
`225a3dcf2272f13bf902379a9a10125dd42f5c88b4109057e2832e4055aa8863`.

- Core: `5b0648a5cd3641f692d08e9b851d2d2d84592b56cc39b702dc615fc006568e7c`.
- Helper: `ae15bdd2c82887040c61643c1d68d4d2508a38e1b240adcaec7a07fd46654fef`.
- Camera inventory: `ccd9aa93ad167224d5980dba6bc6471e143aea196549ce1b3386ee9e8071a96c`. The refreshed first-party
  Camera package reproduces the existing inventory while retaining its verified
  ARM64 bench dependency closure. It uses the Pi's existing Python runtime and
  is not a self-contained release package. Serial and Sigrok were retained.

`nilo version` and authenticated `/api/v1/status` report version 0.12.0 and
profile `appliance`. The GUI returns HTTP 200, and served JS/CSS bytes match
this production build:

- `index-BkKRGd-d.js`:
  `a71703b3724e18ffe7a80fba9600fd43f07fce91fa951cbd5df9cd072a60ccae`.
- `index-CmAWkln6.css`:
  `e1a8e19d5b422741a6a8a1686d28fb7cf03de560d17c8671a81e850e8353b1bd`.

HTTPS checks verified the helper's public certificate obtained over SSH,
using `nilo-n0hzmysj9v.local` resolved to the WireGuard address. The existing
helper startup activates provisioning names before Core activates the configured
management names, renewing the certificate on restart. Its public key and all
configured names were retained; the final certificate SHA-256 is
`569129df801ef8f8797db24f6d6fc5d713d699c6a154e3e139528d95f8b0c2c0`. The private key remains root-owned, mode 0600,
and was not exported. Clients pinning the previous leaf certificate need the
new public certificate.

Protected status returns HTTP 401 without authentication. A temporary token
limited to `status.read,inventory.read,bench.read,network.read` received HTTP
200 from status, appliance, integrations, devices, plugins, network status and
bench jobs. It was revoked after verification; reuse returns HTTP 401.

## Service startup and state preservation

The first install attempt started helper, Core and recovery timer together.
The timer briefly held the helper's recovery lock, causing startup failures;
the four-second health check triggered automatic restoration of the complete
stopped-service backup. The retry starts the helper, waits for its socket,
starts Core, waits for its HTTPS listener, then starts the recovery timer.
This changes installation ordering only; existing service units are retained.

Final Core and helper are active with zero automatic restarts; both services
and the recovery timer are enabled. The timer is active and waiting. There
are zero unfinished jobs and zero active serial connections.

Accounts, plugin/device settings, management access, VPN/interface settings,
Camera media records and serial log records match the stopped-service backup.
Operating settings retain revision 4. Only device discovery
presence, last-seen times and their observation revisions changed on restart.
All 20 media/workspace/log files match their backup hashes. The 14 Camera
media records and five serial log records remain. NetworkManager profiles and
the serialized forwarding plan are byte-for-byte unchanged. WireGuard access
remained available and NILO nftables tables are present.

The Wi-Fi profile has autoconnect enabled; the active WireGuard profile has it
disabled. A full reboot was not attempted. Verification covers service restart,
HTTPS and read-only APIs, without serial transmission, Camera capture, GPIO,
PLC operations or cold-boot hardware acceptance.

## Backup and rollback

The root-only stopped-service backup includes runtime binaries, GUI, Camera
packages, all NILO state, helper state, service units and NetworkManager profiles:
`/var/backups/nilo-0.12.0-20260929-120845/pre-deploy.tar.gz`.
SHA-256 readback passed:
`dae97eb668cdad41e83ed10372cc23268639e84c8213a738a279878570718ce5`.

After a zero-work preflight, restore the previous runtime while retaining
current accounts, settings, network profiles and captures with:

```sh
sudo python3 /var/backups/nilo-0.12.0-20260929-120845/rollback.py
```

The first attempt's backup remains at
`/var/backups/nilo-0.12.0-20260929-120419/pre-deploy.tar.gz`.
The complete archives are retained on the Pi; private state was not downloaded.
Deployment JSON, public HTTPS evidence and preservation summaries are under
ignored `02_Tools/cache/rpi-smoke-20260929/`. No Git push or remote workflow
mutation was performed. The merge commit is local; deployment documentation
and prior tooling/planning changes remain uncommitted.

## Repository-local close-out — 2026-09-29

The user requested closure and a local commit of the prepared tooling,
planning and deployment documentation. Deployment is complete; this close-out
adds no new appliance changes. The software source deployed remains merge
commit `0d18497`. The accepted-image refresh remains a separate CI follow-up.
No Git push or remote pipeline run is authorized by the commit request.

The subsequent local CI compatibility change removes the image-refresh need
for project version metadata alone. Dependency coverage and integrity checks
remain; see section 10 of the tooling simplification plan. This changes no
deployed software or appliance state.
