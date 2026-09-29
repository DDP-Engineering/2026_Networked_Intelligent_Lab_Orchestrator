# Modbus supervised client runner

This package supplies a supervised Modbus client runner, the transport/dependency
baseline, and the versioned development contract. The SDK `serve` operation opens
one pinned endpoint in an owner-only workspace and accepts bounded private
transactions over `control.sock`. Core's credential-bound live sessions, retained
recordings, API/CLI/MCP, GUI and target bundle work follow the implementation backlog.

The scope is master/client, serial RTU, native TCP and RTU over TCP. Named typed
maps include scaling, units and CSV/JSON interchange. Shared definitions use
creator/admin edits; recordings use owner/admin visibility. Plugin policy controls
successive writes with no individual write approval. See the
[contract specification](../../../.agentic/specs/REQ-DATA-6fcda58c.md) and
[implementation plan](../../../00_Planning/Modbus_Plugin_Plan.md).

## Run

Use the pinned repository tools on a prepared Windows/WSL host:

```powershell
.\02_Tools\scripts\doctor.ps1
.\02_Tools\scripts\doctor-wsl.ps1
wsl -d Ubuntu -- sh ./02_Tools/env/with-tools.sh uv run --locked --project ./05_Software/integrations/modbus pytest -q ./05_Software/integrations/modbus/tests
```

Windows runs request/schema/TCP checks and explicitly skips Linux PTY cases.
The complete transport proof uses real Linux pseudo-terminals and literal loopback
TCP peers. SimulationClient requires simulation=True and refuses real target
addresses and serial adapters. Runner tests supply pinned loopback/PTY snapshots;
no discovery or lab-device operation occurs.

The handwritten peers use independent CRC and response encoding. Tests cover all
eight initial functions, golden bytes and limits, stream fragmentation/coalescing,
wrong identity/function/transaction/CRC/length/count/echo, exceptions, cancellation,
late replies, dropped writes, trace bounds and exclusive serial descriptor release.

## Candidate boundary

PyModbus 3.15.0 and pySerial 3.5 are exact pins. Independent request and response
validation supplements the candidate: protocol consistency is checked before
publishing a result, and outgoing bytes are checked before transmission.
Both library retries and reconnect are zero. Transactions are serialized; timeout,
cancellation and protocol faults close and permanently fence that client instance.
A post-TX write without a valid acknowledgement is delivery_unknown and is never
replayed. A pre-TX encoding refusal is rejected.

Trace entries are bounded to 256 observations / 64 KiB raw bytes and correlate
with a request ID. PyModbus passes its cumulative receive buffer to its callback;
kind=library_buffer describes that observation precisely. It is not passive
bus capture or a count of wire packets.

The supervised runner serializes one client call per endpoint, rotates queued units,
coalesces pending polls, and bounds total and per-unit queues. It retries only TCP
reads, at most three times within the request deadline. RTU recovery discards
serial bytes until a frame-gap silence; RTU-over-TCP reconnects after a quiet gap.
Connection startup is capped at 60 seconds even for a longer saved profile timeout.
Because these modes lack a transaction ID, the timed-out unit/function remains
quarantined for this runner lifetime. Other units may continue after recovery;
writes are never retried.

## Dependency evidence and offline proposal

[evidence/dependencies.json](evidence/dependencies.json) binds exact wheel/source
URLs, SHA-256 checksums, and retained license member hashes to uv.lock.
The Modbus-specific third-party transport closure consists of two universal wheels.
The supervised runner also uses the repository SDK and its pinned Pydantic chain
from `uv.lock`; the candidate build environment carries those shared inputs.
pySerial's wheel omits license text; its verified source archive supplies it.
The reviewed build recipe consumes all four archives in local candidate generation
`noble-20260929-01`; the existing accepted archive generation is recorded separately
in `02_Tools/archives/build-environment/current.json`. Candidate verification uses
`02_Tools/scripts/verify-build-environment.sh --engine docker --image nilo/build-environment:noble-20260929-01`.

```powershell
.\02_Tools\env\with-tools.ps1 python .\02_Tools\scripts\prepare-modbus-dependencies.py
.\02_Tools\env\with-tools.ps1 python .\02_Tools\scripts\prepare-modbus-dependencies.py --offline
```

Inputs and extracted notices stay under the ignored 02_Tools/cache/modbus-feasibility.
Production bundle work must carry these complete notices, the SDK and an
architecture-matched verified interpreter. The development run uses pinned
Python 3.12.10 and uv 0.11.25; a universal wheel does not prove a native ARM64
runtime import or appliance acceptance.

Physical parity/baud read-back, RS-485 direction and silence timing, USB identity
and unplug behavior, a named gateway/device, Raspberry Pi throughput, and
native ARM64 import remain distinct target gates. PTYs coerce framing and cannot
settle those properties. Proposed administrator defaults remain in the plan.

## Contracts

schemas/contracts.schema.json uses JSON Schema draft 2020-12. Named definitions
cover endpoint, profile, map/point, workspace/poll group, session, command,
recording, error, private request/reply and trace messages. Semantic cross-field
constraints and ownership/state rules are defined in the contract specification;
later services enforce them as well as the structural grammar. Schema version
is independent of the project's single release version.
