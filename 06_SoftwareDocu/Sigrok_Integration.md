# Sigrok integration

NILO manages sigrok as a curated, supervised plugin. The installed sigrok CLI
performs acquisition and decoding; NILO owns identity, approval, the command
queue, resource ownership, transaction history and capture artifacts. PulseView
can open downloaded `.sr` files. It is not embedded in the browser.

This guide describes the current usable integration. The planned replacement
waveform workspace is documented in
[Planning](../00_Planning/Sigrok_Waveform_Workspace/README.md); its proposed
capabilities must not be assumed available in the current build.

The first adapter exposes ATK-DL16 digital discovery, finite capture and saved
UART decoding. PWM is omitted from discovery and rejected at both the core and
runner boundaries. There is no arbitrary driver, executable, shell, path or
sigrok configuration parameter in the public interface.

## Operation and lifecycle

Open **Plugins**, install **Sigrok Instruments**, choose an approval policy and
enable it. **Ask** requires an administrator's browser approval for every
capture/decode transaction. **Auto** allows these validated operations without
that approval. Discover an analyzer, select digital channels and queue a finite
capture. Select its transaction to inspect replies, approve/cancel, preview the
first samples, download the capture or request UART decoding.

An installation starts disabled. Disable blocks new commands and cancels queued
and pending work; a capture already executing drains normally. Restart checks a
fresh isolated runner without restarting NILO. Restart/uninstall refuse while
work remains pending or active. Uninstall removes package files and inventory,
while preserving transactions and captures. A later install is explicit: a
restart does not silently reinstall or re-enable anything.

| Boundary | Behavior |
|---|---|
| Command admission | Durable UUID transaction, request and session IDs before acknowledgment |
| Idempotency | Same owner + same key + identical normalized input returns the original job; changed input returns `transaction_conflict` |
| Queue | At most 32 nonterminal commands, including pending approvals; overflow returns `queue_full` |
| Execution | At most two workers; commands for the same analyzer serialize |
| Deadline | Default 60 seconds, configurable 1–300; includes approval and queue time |
| Runner | CLI execution bounded to 30 seconds; timeout/cancel kills and awaits the child before releasing ownership |
| Replies | Up to 100 retained ordered replies of at most 4 KiB each; overflow fails the transaction |
| Results | At most 1 MiB; list endpoints return summaries and the transaction endpoint returns details |
| Reconnect | Query the transaction ID; HTTP/MCP disconnect does not cancel accepted work |
| Restart recovery | Unfinished transactions become `incomplete`; no automatic replay |
| Evidence | Verified digest and size, confined staging, 8 MiB artifact limit, owner/admin read access |

Deadline expiry stops execution; terminal publication follows bounded process
cleanup. Discovery and lifecycle are serialized control calls with bounded
runner waits (5-second health, 10-second discovery), not acquisition jobs.
Discovery refuses while NILO owns an analyzer. NILO's leases coordinate NILO
clients; another application such as PulseView can still hold the USB device,
in which case the backend can refuse acquisition.

The finite capture envelope is D0–D15, 1/2/4/5/10 MHz and 1–1,000,000 samples.
UART options are one captured digital channel and 1–4,000,000 baud. These limits
bound this integration slice; they do not certify signal or decoder accuracy.

## Shared interfaces

All three interfaces call the same managed service. Viewers have `bench.read`;
operators additionally have `bench.execute`; administrators also have
`plugins.manage` and `bench.approve`. API tokens remain intersected with their
owner's current permissions. A token cannot grant human approval.

| HTTP | CLI | MCP |
|---|---|---|
| `GET /api/v1/plugins` | `nilo plugin list` | `plugins_list` |
| `POST /api/v1/plugins/{id}/{install,enable,disable,restart,uninstall}` | `nilo plugin <action> --plugin sigrok` | `plugin_lifecycle` |
| `POST /api/v1/plugins/{id}/discover` | `nilo plugin discover` | `plugin_discover` |
| `POST /api/v1/bench/jobs` | `nilo bench capture` / `decode` | `bench_submit` |
| `GET /api/v1/bench/jobs` | `nilo bench jobs` | `bench_list_jobs` |
| `GET /api/v1/bench/jobs/{id}` | `nilo bench get --id ...` | `bench_get_job` |
| `POST /api/v1/bench/jobs/{id}/cancel` | `nilo bench cancel --id ...` | `bench_cancel` |
| `POST /api/v1/bench/jobs/{id}/approve` | Human browser session | Human browser session |
| `GET /api/v1/bench/jobs/{id}/artifact` | `nilo bench download --id ... --out capture.sr` | `bench_read_artifact` (bounded base64 chunks) |

CLI calls take `--server URL --token-file PATH [--ca-file PATH] [--json]`.
Capture uses `--device ID --channels D0,D1 --samples 1024 --samplerate 1000000`.
Supply `--key` when a script needs a repeatable retry; otherwise the CLI prints
its generated key to stderr before submission. Decode uses `--capture
TRANSACTION --channel D0 --baudrate 115200`. The CLI never overwrites a download.

Example submission:

```json
{"plugin_id":"sigrok","operation":"capture","device_id":"atk-dl16-1.3","parameters":{"channels":["D0","D1"],"samplerate":1000000,"samples":1024},"idempotency_key":"capture-example-001","timeout_seconds":60}
```

HTTP returns 202 with the transaction, its state and a `Location` header. Poll
that location. A client timeout does not prove submission failed; repeat the
same key and inputs to resolve that uncertainty without executing twice.

## Package preparation and deployment

Packages are trusted local catalog content, not arbitrary API uploads. Every
file, including the runner, schemas and Python dependencies, is covered by
`files.sha256.json`; installation and execution verify it. The package copy
lives below NILO's data directory. The catalog defaults to
`/opt/nilo/integration-bundles`, overridable with `serve --integration-bundles`.
No bundle is automatically enabled. `--sigrok-cli` selects an administrator-
configured absolute backend executable; it is never a client parameter.

Prepare a new ARM64 bundle on the development host:

```powershell
.\02_Tools\env\with-tools.ps1 python 02_Tools/scripts/build-sigrok-bundle.py --out 02_Tools/runtime/my-edition/integration-bundles/sigrok --architecture arm64 --python-version 3.13
```

The destination must be new. The builder exports the locked dependency graph
and verifies wheel hashes for the target architecture/Python minor. It copies
the SDK and adapter into the bundle. The target needs that Python minor on its
PATH and an installed, usable sigrok CLI with the ATK driver. Host software is
not installed by a plugin API request. Match the target architecture, installed
Python minor and sigrok build before preparing the bundle.

For an edition shipped with NILO, use `NILO_BUNDLE_SIGROK=1` and optionally
`NILO_TARGET_PYTHON=3.13` with `02_Tools/scripts/build-appliance.sh`; each target
directory receives its integration catalog alongside the binaries. For a later
installation, deliver the same catalog directory separately. Place it under
`/opt/nilo/integration-bundles` (readable by the NILO service account), deploy the
normal NILO binary/GUI, then install and enable through NILO. Removing a plugin
does not remove the catalog copy, so it remains available for explicit reinstall.

Windows development keeps the runner in WSL2. Build an `amd64` bundle matching
the WSL runner's Python minor, pass that Linux catalog path to `serve`, and use
the simulation profile. Runtime state stays on the WSL filesystem. The browser
mock does not emulate plugin execution; use the real simulated runner for this
workflow.

## Extension boundaries

The lifecycle, admission queue, identity, policy, transactions, leases, artifacts
and HTTP transport are reusable. `internal/bench/parameters.go` and the sigrok
parts of `execution.go` are the first curated operation adapter: they resolve
connections and core-owned artifact paths before invoking the package. Future
CAN, J-Link, STM32 and ESP integrations add their own typed operation adapter
and manifest/runner, with operation-specific safety and resource claims, instead
of adding device commands to HTTP handlers. A generic third-party adapter
registry, package updater/rollback, marketplace, continuous capture and full
waveform editing remain later work.

## Verification

The portable suites cover queue bounds, independent/conflicting concurrency,
delayed replies, expiry, cancel cleanup, restart without replay, retained
uninstall choices, tampering, ownership, PWM refusal, HTTP/CLI/MCP permissions
and GUI retry/approval behavior. `TestAssembledBundleDescribe` is an opt-in target
test using `NILO_TEST_BUNDLE`. `02_Tools/scripts/check-sigrok-integration.py`
starts an isolated HTTPS appliance instance and exercises the installed ATK
through the integration. It neither stops the installed NILO service nor
qualifies a known electrical signal. Physical accuracy, advanced sampling and
decoder correctness remain separate acceptance work.
