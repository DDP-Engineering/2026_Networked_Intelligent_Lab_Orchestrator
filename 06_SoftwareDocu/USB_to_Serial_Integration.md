# USB to Serial integration

## Responsibilities and access

The `serial` package discovers physical USB serial endpoints on the NILO Linux
appliance. Bench Core owns managed device names, enablement, approval, session
ownership, credentials and retained session logs. The supervised Python runner
owns one exclusive port descriptor and serves a private Unix control socket.
System plugin configuration is administrator-only; instrument access follows
the shared Bench permissions.

Discovery reads USB ancestry below `/sys/class/tty` without opening a port.
Endpoints are `ttyUSBn` or `ttyACMn`. USB vendor/product, serial number, interface
and channel distinguish multiport adapters. Missing or known placeholder serial
numbers use physical USB location fallback. The runner checks the full identity
again immediately before and after opening a descriptor.

## Session controls

Port settings include baud rate, 5–8 data bits, parity, stop bits, software or
hardware flow control, and initial RTS/DTR. Settings are immutable within the
approved connection; reconnect to change them. Linux represents 1.5 stop bits
with five data bits; other 1.5-stop combinations are refused. Driver rejection
or a mismatch in read-back framing, flow control or baud rate fails the connection.

The runner uses pySerial 3.5, an advisory exclusive lock and Linux `TIOCEXCL`.
Modem indicators come from a hardware ioctl. Unsupported signals are `null`,
and a requested unsupported line change returns `unsupported_signals`.
Manual RTS changes are refused while RTS/CTS flow control owns that line.
Some USB adapters briefly assert output lines while opening; electrical behavior
must be verified with the actual adapter and target.

Each transmission is at most 4096 bytes. The queue is bounded to 64 KiB and a
write has a two-second deadline, including time waiting for another write.
Results distinguish complete, partial and failed OS writes. Accepted bytes
describe the host driver, not acknowledgement by the connected instrument.
The runner emits each accepted TX byte once into the same sequence as RX data.
Core owns persistent idempotency and never replays uncertain transmissions.

The live ring retains at most 1 MiB of raw data and 4096 events. Read pages are
below 64 KiB of encoded JSON and report explicit cursor gaps. Receive waits are
finite; unplug and shutdown terminate acquisition and release the descriptor.

## Private control boundary

Core supplies a mode-0700 workspace owned by its service account. The runner
binds `control.sock` with mode 0600. The socket exposes `/status`, `/read`,
`/send`, `/signals` and `/stop`; it has bounded request sizes, request workers
and deadlines. Status includes `last_sequence` so Core can drain accepted bytes
before finalizing a log. This socket is private infrastructure; GUI, CLI and
MCP use authenticated Core services.

The SDK operation `discover_devices` accepts `{}`. The public `connect`
operation is resolved by Core into an approved private `stream`. A direct
runner `connect` request is refused. Package configuration validates the
ordinary revisioned plugin/device envelope and opens no descriptors.

## Offline dependency preparation

The universal pySerial wheel is pinned by SHA-256 in
`02_Tools/scripts/prepare-serial-dependency.py` and the project lockfile.
Prepare it once on the development host:

```powershell
python 02_Tools/scripts/prepare-serial-dependency.py --output 02_Tools/cache/containers/outputs/python-packages
```

`--offline` verifies an existing wheel and refuses a missing or modified input.
The accepted container image predates this package. Its Python environment
preparation installs the verified wheel from `/outputs/python-packages`, or
the explicit `NILO_SERIAL_WHEELHOUSE` directory. Downloads and extracted
dependencies remain in ignored tool caches.

## Target bundle and permissions

`02_Tools/scripts/build-serial-bundle.py` assembles the source, SDK/Pydantic
dependencies, verified pySerial wheel and target Python runtime. It checks the
native architecture, runtime inventory and dependency notices, then creates
the package manifest and complete `files.sha256.json` inventory. Production
bundles use `--runtime-root`; the runtime is the same verified target input
used by the appliance integrations. Packages and dependencies are read-only.

```sh
python 02_Tools/scripts/build-serial-bundle.py \
  --project 05_Software/integrations/serial \
  --dependency-root /verified/target/dependencies \
  --wheelhouse /verified/serial-wheels \
  --runtime-root /verified/target/runtime \
  --architecture arm64 --out /output/serial
```

The candidate-set builder includes serial on both target architectures. The
development appliance builder supports `NILO_BUNDLE_SERIAL=1` with
`NILO_SERIAL_RUNTIME_ROOT`, `NILO_SERIAL_DEPENDENCY_ROOT` (each containing
`arm64` and `amd64` subdirectories), and `NILO_SERIAL_WHEELHOUSE`.

The packaged service runs as `nilo` with supplementary `dialout` membership.
The udev rule applies mode 0660 and group `dialout` only to tty endpoints with
USB ancestry. Administrators control installation and plugin enablement.

## Verification

`05_Software/integrations/serial/tests/` covers USB metadata, multiport and weak
identity, SDK handshake, no-open configuration, real Linux pseudo-terminal
RX/TX, exclusivity, timeout/partial writes, disconnect, modem support, cursor
gaps, socket permissions and shutdown. Tool tests under
`02_Tools/tests/software/test_serial_bundle.py` verify pinned-input refusal and
non-root target access policy. Physical USB baud accuracy, signals and reconnect
identity require an adapter on the appliance; pseudo-terminals do not establish
those hardware properties.
