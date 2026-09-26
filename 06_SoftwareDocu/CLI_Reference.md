# NILO CLI reference

This reference describes the implemented `nilo` commands, checked against the
repository on 2026-09-27. Camera commands use the same Core services as the GUI
and [MCP tools](MCP_Reference.md). For identity administration, token creation and
first-run setup, see [CLI and authentication](../07_Delivery/NILO_CLI_and_Authentication.md).

## Remote connection and results

Remote commands require `--server URL --token-file PATH`. The server can also
be an exact discovered permanent name or unique friendly name. Add
`--ca-file PATH` for the appliance CA and `--json` for machine-readable results.
The token file contains the API token; browser login cookies are not used.

PowerShell examples below use this argument array; substitute your addresses
and existing credential files:

```powershell
$remote = @('--server', 'https://10.205.148.11:8443',
            '--token-file', 'C:\NILO\token.txt',
            '--ca-file', 'C:\NILO\nilo-ca.pem', '--json')
nilo plugin list @remote
```

Token scopes intersect the owner's current role. Reading Bench state requires
`bench.read`; submitting operations requires `bench.execute`. Plugin management
and retention configuration require `plugins.manage`. Discovery requires
`bench.execute`. A token never grants human approval: an operation under Ask
policy must be approved in the GUI.

Remote command exit codes are `0` for a successful request, `1` for failure and
`2` for invalid usage. Submission success means a transaction was accepted;
inspect its terminal state to determine whether the operation succeeded.
`--timeout-seconds` includes queue and approval time, defaults to 60, and is
bounded to 300 seconds. It is the command deadline, not a video's duration.
Accepted jobs continue after the CLI disconnects.

## Command families

| Command | Purpose / reference |
|---|---|
| `nilo version` | Report the project build version. |
| `nilo discover [--timeout 5s] [--json]` | Discover appliances on the local network; timeout is 1–60 seconds. |
| `nilo discover --label NAME` | Write the selected appliance's printable SVG label; cannot combine with `--json`. |
| `nilo open NAME` | Open a uniquely discovered appliance in the browser. |
| `nilo serve ...` | Run Core; see [development flow](Development_and_Simulation_Flow.md) and [appliance networking](Network_Operations.md). |
| `nilo admin ...` | Local or remote identity, token and password-policy administration; see the [administrator command reference](../07_Delivery/NILO_CLI_and_Authentication.md#3-the-nilo-admin-command-line). |
| `nilo seed-admin ...` | Provision an initial administrator from a password file during installation. |
| `nilo plugin ...` | Manage installed plugins and physical device records. |
| `nilo bench ...` | Submit, inspect, cancel and download durable transactions. |
| `nilo camera ...` | Camera snapshots, hardware controls, workspace state and retained media. |
| `nilo network ...` | Network status, interfaces, Wi-Fi, VPN and device networks. |
| `nilo activity ...` | Read/export activity and configure its retention. |
| `nilo maintenance ...` | Local data inventory, migrations, snapshots, restore, removal and purge. |
| `nilo serve-foundation-check --data-dir D` | Local foundation verification. |
| `nilo development issue-token --data-dir D` | Issue a development Viewer token. |

There is no top-level `nilo diagnostics` command. Network checks are described
under `network` below. Development and maintenance commands are local appliance
or test-environment operations; they do not take the shared remote arguments.

## Plugins and managed cameras

Append the shared remote arguments to each command. Plugin selection defaults
to `sigrok`; always specify `--plugin camera` for Camera administration.

```text
nilo plugin list
nilo plugin install|upgrade|enable|disable|restart|uninstall --plugin ID
nilo plugin enable --plugin ID --mode ask|auto
nilo plugin discover --plugin ID
nilo plugin devices --plugin ID
nilo plugin configure --plugin ID --revision REV --file settings.json
nilo plugin device configure --plugin camera --device ID --revision REV --file device.json
nilo plugin device enable|disable|retry --plugin camera --device ID --revision REV
nilo plugin device rebind --plugin camera --device ID --revision REV --provisional NEW_ID --provisional-revision NEW_REV --confirm ID
nilo plugin device forget --plugin camera --device ID --revision REV --confirm ID
```

Install/upgrade uses the appliance's prepared curated bundle. It does not accept
an arbitrary package path or download a release from the Internet. Upgrade
requires an available validated candidate. Disabling a plugin hides its
instrument access; per-device enablement is a separate setting.

`settings.json` is the plugin settings object. `device.json` contains `name`,
optional `purpose`, and `settings` conforming to the installed device schema.
Use the observed device revision from `plugin devices`; device writes require
revision ≥ 1. Plugin configuration uses its own revision (≥ 0), available from
`GET /api/v1/plugins/{id}/configuration` or MCP `plugin_configuration`; the CLI
has no dedicated configuration-read verb. On conflict, read current state
again before changing it. Rebind and forget require the exact record ID in
`--confirm`; forgetting a device retains its transaction history.

## Bench transactions

```text
nilo bench jobs
nilo bench get --id TRANSACTION
nilo bench cancel --id TRANSACTION
nilo bench download --id TRANSACTION --out FILE
nilo bench submit --plugin ID --operation OP --device ID --parameters-file parameters.json [--key KEY] [--timeout-seconds 60]
nilo bench capture --device ID [--channels D0,D1] [--samplerate 1000000] [--samples 1024]
nilo bench decode --capture TRANSACTION [--channel D0] [--baudrate 115200]
```

`jobs` returns up to 100 recent authorized jobs. Non-administrators see their
own transactions; `plugins.manage` permits administrator access across owners.
Capture/decode are existing Sigrok shortcuts; this reference does not describe
the planned replacement waveform implementation.

Submission returns a job containing `transaction_id`, `state` and its deadline.
Poll `get` for ordered replies, the terminal result/error and any `artifact_id`.
States include `pending_approval`, `queued`, `running`, `completed`, `failed`,
`cancelled` and `incomplete`. Cancellation can require asynchronous hardware
cleanup; check state before reusing the device.

The CLI generates an idempotency key when omitted. Explicit keys use 8–128
ASCII letters, digits, underscores or hyphens. Reuse an explicit key only
to retry identical inputs. Use a new key for each intentional new snapshot or
recording command. `parameters.json` must contain a JSON object, not a string
containing JSON. Device IDs come from discovery; do not submit `/dev/video*`
paths, output paths or runner socket paths as public operation parameters.

## Camera snapshots and hardware controls

These are standalone transactions. Stop a live workspace before using them;
for a camera already in live view use the workspace actions in the next section.

```text
nilo camera snapshot --device ID [--width 640] [--height 480] [--format MJPG|YUYV] [--zoom 1] [--rotation 0|90|180|270] [--grayscale] [--invert] [--maximum-dimension N] [--wait-seconds 0] [--out FILE]
nilo camera controls get --device ID
nilo camera controls set --device ID --controls-file controls.json
nilo camera controls reset --device ID [--factory]
```

All accept `--key` and `--timeout-seconds`. A snapshot's `--wait-seconds` is a
bounded wait (1–30 seconds when enabled; 0 normally submits immediately), not
an instruction to bypass approval. `--out FILE` waits up to 20 seconds by
default and downloads if completed. A wait timeout prints `snapshot pending`
text even with `--json`; for JSON automation submit without waiting, then poll
`bench get`. Download a completed snapshot using
`nilo bench download --id TRANSACTION --out image.jpg`.
Download commands require a new output file and refuse to overwrite one.

`controls get` returns a transaction whose result describes actual hardware
controls and bounds. Use the camera's returned IDs and supported values, for
example this `controls.json` only if that device exposes these controls:

```json
{"brightness": 0, "contrast": 15, "saturation": 32}
```

Reset without `--factory` uses the saved baseline; `--factory` selects hardware
defaults. The generic `bench submit --plugin camera --operation snapshot` path
also accepts advanced snapshot parameters such as `encoding` (`jpeg` or `png`),
`jpeg_quality` (1–95), `frame_interval`, `controls` and processing options.
Standalone `jpeg_quality` differs from workspace `quality`.

## Camera live workspace and recording

```text
nilo camera workspace-status --device ID
nilo camera stop-live --device ID
nilo camera stop-recording --device ID
nilo camera media --device ID
nilo camera media-download --id MEDIA_ID --out FILE
nilo camera media-delete --id MEDIA_ID
nilo camera retention --device ID --revision REV --file limits.json
```

Starting live view, changing live settings, taking a processed live snapshot,
and starting video use the existing generic submission command:

```powershell
nilo camera workspace-status --device cam-1 @remote
nilo bench submit --plugin camera --device cam-1 --operation workspace --parameters-file start.json @remote
nilo bench get --id TRANSACTION_FROM_SUBMISSION @remote
```

Create `start.json` with the **current** workspace revision; `1` below is only
an example. Wait for that transaction to complete before sending a dependent
command. Re-read workspace state after settings or retention changes.

```json
{"action": "start", "revision": 1}
```

Use the same `bench submit` command with a different JSON file for each action:

| `action` | Additional fields / effect |
|---|---|
| `start` | `revision`; start live view using saved settings. |
| `settings` | `revision`, `settings` object; apply and persist live capture/processing settings. |
| `controls` | `revision`, `controls` object of hardware control IDs and values. |
| `read_controls` | Read hardware controls through the active workspace. |
| `reset_controls` | `revision`; reset workspace hardware controls. |
| `snapshot` | `revision`; save the processed live frame with current encoding and overlays. |
| `record_start` | `revision`; start video or time-lapse using saved workspace settings. |
| `record_stop` | Stop recording; prefer the dedicated `camera stop-recording` command. |
| `stop` | Stop live view; prefer `camera stop-live`. |

The dedicated stop commands do not create a new approval request and require
the session owner or an administrator. Do not invoke the internal `stream`
operation directly. The live frame endpoint is authenticated
`GET /api/v1/cameras/{id}/frame` (JPEG); the CLI does not render a live viewer.
An idle preview without frame requests expires after 30 seconds. Recording
continues without a browser until stopped or limited; sessions also have a
55-minute bound. Settings cannot change during recording.

### Workspace settings and overlays

Read `workspace-status`, copy its settings, modify the desired fields and send
the resulting full `settings` object. Omitted fields receive defaults; this is
not a partial patch. Choose a resolution/format/frame rate the device advertises.

Example `settings.json` for a workspace action (replace its revision):

```json
{
  "action": "settings",
  "revision": 1,
  "settings": {
    "width": 640, "height": 480, "format": "MJPG", "fps": 15,
    "processing": {
      "zoom": 1, "rotation": 0,
      "flip_horizontal": false, "flip_vertical": false,
      "grayscale": false, "invert": false,
      "overlays": [{"kind": "crosshair"}]
    },
    "stamps": [
      {"kind": "time", "position": "top-left", "background": "black", "size": 16, "timezone": "UTC"},
      {"kind": "name", "position": "bottom-left", "background": "transparent", "size": 16}
    ],
    "encoding": "jpeg", "quality": 85,
    "mode": "video", "interval": 5, "playback_fps": 15, "video_quality": 23
  }
}
```

| Field | Values / constraints |
|---|---|
| `width`, `height` | Even integers 16–4096, at most 2,000,000 pixels in combination; must match device support. |
| `format`, `fps` | `MJPG` or `YUYV`; integer 1–30 fps, advertised by the device. |
| `processing.zoom` | 1–8; optional `center_x`, `center_y` are normalized 0–1 coordinates. |
| `processing.rotation` | 0, 90, 180 or 270 degrees. |
| Processing switches | `flip_horizontal`, `flip_vertical`, `grayscale`, `invert`. |
| `processing.overlays` | Drawing overlays including crosshair and grid; see [processing schema](../05_Software/integrations/camera/src/nilo_camera/processing.py) for shape-specific fields. |
| `stamps` | At most 3: `kind` = `time`, `name` or `text`; custom `text` up to 120 characters. |
| Stamp placement | `top-left`, `top-center`, `top-right`, `bottom-left`, `bottom-center`, `bottom-right`. |
| Stamp appearance | `background` = `transparent`, `black` or `white`; `size` = 8–48; time `timezone` = installed IANA zone or `UTC`. |
| `encoding`, `quality` | Snapshot `jpeg` or `png`; JPEG quality 1–95, default 85. |
| `mode`, `interval` | `video` or `timelapse`; acquisition interval 1–3600 seconds for time-lapse. |
| `playback_fps` | Time-lapse playback frame rate 1–30, default 15. |
| `video_quality` | H.264 CRF 18–35, default 23; lower values give higher quality. |

Video is currently MP4/H.264. Processing, overlays and stamps are applied to
both preview and saved files. Hardware brightness, contrast, saturation, gamma
and other controls depend on the physical camera; use the returned capabilities.

### Media, export and retention

`camera media` returns an `items` array containing the first 24 authorized
captures. It currently has no pagination flag; use MCP `camera_media_list`
with `offset`, or the HTTP media route, for subsequent pages. Use the returned
media ID for media commands; workspace captures have their own IDs, while a
standalone snapshot uses its transaction ID in the unified library.
Export with `media-download`; use `bench download`
for a transaction artifact. `media-delete` permanently deletes the selected
authorized capture without an interactive confirmation. Active media cannot
be deleted.

Default `limits.json`:

```json
{"snapshots": 100, "total_bytes": 2147483648, "video_bytes": 524288000, "video_seconds": 600}
```

Stop live view before saving limits and use the latest workspace revision.
Supported limits are 1–10,000 snapshots, 64 MiB–100 GiB total, at least 8 MiB per
video with 2 MiB of headroom below the total limit, and 1–3500 seconds per video.
All captures share one library and retention policy. On future capture
admission, the oldest eligible completed files may be removed to satisfy limits;
saving limits does not immediately purge files. Active/incomplete captures and
downloads are protected from automatic removal. All files count toward usage.
Core also reserves at least 2 GiB or 10% of appliance disk space, whichever is
larger. A video stops at its byte or acquisition-time limit; time-lapse duration
here measures acquisition time, not playback length.

## Network and Activity

The full network workflows, JSON configuration and connectivity recovery rules
are in [Network operations](Network_Operations.md). Supported command forms:

```text
nilo network status
nilo network statistics --interface ID --period 15m|1h|24h
nilo network interface enable|disable ID
nilo network interface settings ID --method static|automatic [--address A/P] [--gateway G] [--dns A,B] [--dns-automatic] [--protect] [--never-default]
nilo network wifi scan ID
nilo network wifi join ID --ssid NAME --security open|wpa2-personal|wpa3-personal [--password-file PATH] [--startup]
nilo network vpn check|install
nilo network vpn profiles list
nilo network vpn profiles import --file PATH --name NAME
nilo network vpn profiles remove ID
nilo network vpn profiles rename ID --name NAME
nilo network vpn enable ID
nilo network vpn disable
nilo network vpn autostart ID --enabled true|false
nilo network device-networks list|show ID|instructions ID
nilo network device-networks create --file PATH
nilo network device-networks update ID --file PATH
nilo network device-networks enable|disable|delete ID
nilo network device-networks check ID [--tcp-port N]
nilo network operation list|status ID|confirm ID|rollback ID
nilo network operation acknowledge ID --revision REV
nilo activity list [FILTERS]
nilo activity export --format jsonl|csv [FILTERS] [--out PATH]
nilo activity retention
nilo activity retention --retention audit=90,event=14 --quota audit=268435456,event=134217728
```

Network mutation previews may require `--acknowledge` in noninteractive use;
connectivity confirmation is a separate step. Network reads use `network.read`,
ordinary network changes use `network.manage`, and VPN changes/installation
use `vpn.manage`/`vpn.install` respectively.

Activity filters are `--category`, `--component`, `--target`, `--actor`,
`--severity`, `--outcome`, `--operation`, `--action`, `--from`, `--to`, `--cursor`
and `--limit`; time bounds use RFC 3339. Activity read/export/retention require
`activity.read`, `activity.export`, and `activity.retention.manage` respectively.

## Local provisioning and maintenance

These commands use an absolute appliance data-directory path, without remote
credentials. Use the installation/upgrade procedure for the target appliance
before changing its state:

```text
nilo seed-admin --data-dir D --username NAME --password-file PATH
nilo maintenance inventory --data-dir D
nilo maintenance migrate --data-dir D
nilo maintenance snapshot --data-dir D --transaction ID
nilo maintenance restore --data-dir D --snapshot PATH --failed-target-version VERSION
nilo maintenance remove --data-dir D [--nethelper-socket PATH]
nilo maintenance purge --data-dir D --confirm-local [--nethelper-socket PATH]
```

Inventory reads existing state; migration, restore and cleanup change it.
Purge removes state and requires explicit local confirmation. These are
appliance lifecycle operations, not camera-media retention commands.

## Keeping this reference current

Command dispatch is in [main.go](../05_Software/core/cmd/nilo/main.go); camera,
plugin and Bench flags are in [bench.go](../05_Software/core/cmd/nilo/bench.go).
Workspace validation is in
[camera_workspace.go](../05_Software/core/internal/bench/camera_workspace.go) and
[workspace.py](../05_Software/integrations/camera/src/nilo_camera/workspace.py).
Update this reference and the MCP reference together when public operations,
flags, permissions or parameter schemas change.
