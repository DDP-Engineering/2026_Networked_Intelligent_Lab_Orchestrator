# Structured Sigrok worker feasibility

This is the SW-01 feasibility result for the NILO 0.2.0 development line. It proves a structured native boundary and records its limits; it does not replace the current public Sigrok operations, qualify hardware, or claim PulseView feature parity.

## Decision

Use a small C11 **native child process** below the existing isolated Python integration runner. The child calls libsigrok and libsigrokdecode directly and emits bounded `nilo.sigrok.worker/1` JSONL. The Python parent owns timeout, process termination, byte/record limits, schema validation, and rejection of PWM or output capabilities. Bench Core continues to supervise the Python package through `nilo.integration/1`, so a native driver or decoder crash cannot occur in the Go process.

The checked-in helper is a feasibility executable with exactly two operations:

```text
nilo-sigrok-worker demo-capture --samples <1..4096>
nilo-sigrok-worker uart-fixture
```

There is no driver/configuration/path passthrough. The Demo sample limit is internal and finite. The worker does not call output-module APIs, does not expose configuration setters to its caller, and never reports PWM channels or settings. Production capability/configuration commands belong to SW-02 after their public types and policy limits are designed.

## Evidence

The worker compiled with C11 warnings-as-errors and ran on the authorized ARM64 development appliance. The deployed stack reported sigrok-cli 0.8.0, libsigrok 0.6.0 ABI `4:0:0`, libsigrokdecode 0.6.0, GLib 2.72.4, and libsigrokdecode's embedded Python 3.10.12. The installation contained 220 hardware drivers and 138 decoder directories; those counts describe installed runtime content, not NILO support claims.

[`demo-capture.jsonl`](../05_Software/integrations/sigrok/evidence/demo-capture.jsonl) is a direct native Demo run. It records the Demo driver/device, eight logic and five analog channels, seven native channel groups, five waveform-related typed settings, 64 logic samples, and 320 analog values. The direct datafeed callback received header/end, logic, and analog packets. It did not receive trigger, frame-begin, or frame-end packets in this configuration; the summary records those fields as `false`. The native ABI does expose those callback types, so later device/configuration tests can distinguish available-and-observed from available-but-not-observed.

[`uart-fixture.jsonl`](../05_Software/integrations/sigrok/evidence/uart-fixture.jsonl) feeds a deterministic 1 MHz waveform containing byte `0x55` at 100 kbaud to the installed UART decoder. Native callbacks returned sample-indexed annotations and two binary classes. No console prose is parsed. This is decoder integration evidence, not a general UART correctness qualification.

[`coverage.json`](../05_Software/integrations/sigrok/evidence/coverage.json) is the machine-readable feature, evidence-level, device-class, experimental-option, dependency, and limitation register. Tests validate all three retained artifacts.

## Comparison with the sigrok-cli bridge

| Concern | Current sigrok-cli bridge | Structured worker result |
|---|---|---|
| Driver/device metadata | Parses an ATK-specific scan line | Uses driver/device/channel/group native accessors |
| Settings | Hard-coded channel/rate schema | Preserves native keys, datatypes, GVariant types, scopes, access flags, current/list values |
| Acquisition | Produces a finite `.sr` file and reads a preview | Receives logic, analog, metadata, trigger and frame packet types directly |
| Decoder output | Parses sample-numbered UART text | Receives annotation and binary callback structures |
| Fault boundary | sigrok-cli is already a child of Python | Native helper remains another short-lived child; Python validates JSONL and owns kill/wait |
| Deployment coupling | Requires compatible sigrok-cli | Requires compatible native ABI, decoder data, embedded-Python runtime and transitive libraries |
| Present usefulness | Retains the working ATK capture/download route | Proves the boundary needed by SW-02/SW-04/SW-10; not yet a public operation |

The sigrok-cli bridge remains useful and unchanged for the current finite ATK workflow. It cannot provide the complete typed packet/callback contract needed by the waveform workspace. Loading the native libraries with Python `ctypes` was rejected because it couples opaque C layouts and libsigrokdecode's embedded Python 3.10 into the long-lived Python runner. Linking them into Go was rejected because it would bring device/decoder crashes and packaging dependencies into Bench Core. A short-lived C child preserves the existing process-oriented architecture and can be fenced/restarted independently.

## Capability and device-class boundary

The waveform workspace can build on logic-analyzer, oscilloscope, and mixed Demo evidence. Power supplies, electronic loads, signal generators, multiplexers, delay generators, and scales are excluded from this worker surface. Measurement-oriented classes—including multimeters, sound/temperature/humidity/energy/power/frequency meters, demodulators, and LCR meters—need an explicit later classification; installation in libsigrok alone does not make them waveform instruments.

The local PulseView source includes an experimental `ENABLE_FLOW` option whose default is false. Its presence in source does not establish deployed availability and it is excluded from this worker slice. No experimental source feature is promoted into NILO capability metadata without build and runtime evidence.

## Packaging constraints

The feasibility worker uses `dlopen` so package build requires only a C11 compiler and the system dynamic-loader interface; runtime requires matching `libsigrok.so.4`, `libsigrokdecode.so.4`, and all transitive libraries. The decoder process also requires the installed decoder tree and the Python standard library/extensions matching libsigrokdecode's Python 3.10 build. On the observed appliance, `PYTHONHOME=/opt/pulseview-ddp/usr`, `LD_LIBRARY_PATH=/opt/pulseview-ddp/usr/lib`, and the decoder directory under `/opt/pulseview-ddp/usr/share/libsigrokdecode/decoders` were required.

Real drivers additionally need their firmware, udev/device permissions, connection libraries, and hardware-specific acceptance. Those are packaging inventory items, not proof of electrical correctness. The SW-15 package must pin and report compatible versions together.

Build the helper on a Linux target without modifying the system toolchain:

```text
python3 02_Tools/scripts/build-sigrok-worker.py \
  --source 05_Software/integrations/sigrok/worker/nilo_sigrok_worker.c \
  --out <new-output-path>
```

The destination must be new. Binaries and target runtime files are not committed.

## Explicit limitations

- Demo and deterministic fixtures are the normal SW-01 validation inputs. No physical analyzer was opened.
- The observations do not establish trigger timing, voltage scaling, protocol correctness, signal integrity, or any other electrical correctness property.
- Trigger/frame event types are structurally handled but were not emitted by the bounded Demo configuration.
- Native GVariant type/value retention is evidence for typed settings; SW-02 still must define stable public types, dependency refresh, policy limits, and errors.
- The helper's fixed feasibility protocol is intentionally not installed as a new public manifest operation. Adoption and packaging are subsequent work packages.
