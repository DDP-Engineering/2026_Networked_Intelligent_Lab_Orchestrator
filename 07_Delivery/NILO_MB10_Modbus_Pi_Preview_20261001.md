# MB-10 Modbus Raspberry Pi preview — 2026-10-01

The DDP Raspberry Pi 4 at `10.205.148.11` runs the current
`ddp/modbus-client` Core and GUI with a Modbus client preview package. The
appliance's TLS name is `nilo-n0hzmysj9v.local`; NILO listens on port 8443.
This is a bench deployment of the project's 0.12.0 development version, not a
versioned release artifact.

## Installed candidate

- Core SHA-256: `76585a335c4b05661809e4382c3faf7dce970ffc4ab0000b23729e76e7b083c1`.
- Transfer archive SHA-256: `2a1f5a862886083dd642fafa24c1ff4e462e28afc37508fb0357d8a9dd315364`;
  the Pi independently matched the transfer and verified each payload file.
- Modbus bundle inventory SHA-256:
  `010d6326fb51d6b9bd0f9c1d2c84feb202a8717681617a3e96f3234c7483e22b`.
  The bundle and installed copy passed Core's package verification. The runner
  and Python dependencies imported under the unprivileged `dpasare` account.
- Modbus is installed, enabled and `ready`, with plugin policy mode `ask`.
  No endpoint or real-device I/O was created by this deployment.

The bundle contains exact PyModbus 3.15.0 and pySerial 3.5 wheels and retained
licenses. Its native Pydantic closure came from the prior 292-file verified
Camera bench payload and imports on the Pi with Python 3.13.5. Its
`runtime.json` marks `self_contained: false`; production self-contained runtime
closure for both architectures remains an unperformed release gate.

## Observed checks

- The Pi returned the supervised runner's `hello` and `describe` responses for
  the Modbus integration; the replies advertised `connect` and `serve`.
- Focused Modbus runner, scheduler and supervised tests: 47 passed under WSL2.
  The Core's Modbus and Serial/Modbus physical-port contention tests passed.
  The amd64 bench bundle also answered runner `hello` and `describe` under WSL2.
- Modbus frontend component tests: 21 passed. Integration contract tests: 11
  passed. The disposable appliance-profile stage passed its real
  NetworkManager/helper/Core checks and 46 browser end-to-end checks. Those
  browser scenarios exercise the appliance shell and services, but do not
  install the Modbus package inside the browser fixture; Modbus-specific UI
  behavior is supported by the focused component tests and Pi API/catalog
  observations rather than claimed as a combined browser-to-runner pass.
- The canonical isolated full suite returned exit 1 after 224 tooling tests
  passed: the unchanged supervisor `one_MiB_overflow` test observed a closed
  pipe instead of the expected line-limit error. That focused test passed in
  five consecutive reruns. The command stopped before its later Python and
  browser routing; the separate focused and appliance checks above do not
  turn that full-suite result into a pass.
- The target's `nilo.service` is active. TLS verification against the retained
  public certificate succeeded. The GUI and its built JS/CSS matched the local
  payload byte for byte. An unauthenticated status call returned 401; a scoped
  temporary token received healthy appliance status and listed Modbus as ready.
  The token was revoked, and reuse returned 401.
- The first installation attempt restored the prior stopped-service snapshot
  because its health probe used loopback, which appliance firewall policy does
  not admit. The corrected probe checked the listening management socket; the
  retry passed and the management HTTPS origin returned 200 from the host.

The local preview establishes ARM64 import, non-root startup and package
registration, not physical Modbus behavior. No named USB-RS485 adapter,
gateway, unit or safe read address was supplied, so baud/parity read-back,
direction control, unplug handling, gateway behavior and real read results are
**pending**. The simulator and PTY checks do not stand in for those checks.
The project release-input configuration remains empty, so `.deb`, `.run` and
image release artifacts have not been produced for MB-10.

## Backup and rollback

The stopped-service predeployment archive on the Pi is
`/var/backups/nilo-mb10-20261001-090342/pre-deploy.tar.gz`, SHA-256
`e1caa2f5abd63a3fe37f9a0e37d80efd9d3ae68362eccc8754d99dcd7f6b29a4`.
It contains the previous Core, GUI and `/var/lib/nilo` state. After ensuring
there are no active jobs and accepting that postdeployment state would be
replaced, restore the immediate predeployment snapshot with:

```sh
sudo python3 /var/backups/nilo-mb10-20261001-090342/rollback.py
```

The first attempt's automatic rollback backup is retained separately on the
Pi. No Git push or release publication was performed.
