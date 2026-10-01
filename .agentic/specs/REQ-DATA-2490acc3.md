<!--
Copyright (c) 2026 DDP Engineering
This software is licensed under the PolyForm Noncommercial License 1.0.0.
Commercial use requires a separate commercial license.
See the LICENSE.txt in the .agentic folder for full license details.
SPDX-License-Identifier: LicenseRef-PolyForm-Noncommercial-1.0.0
-->

# REQ-DATA-2490acc3 — Retain immutable Modbus recording parts

## Behavior

One recording ID spans numbered JSONL parts. Start writes a durable metadata row; every part header contains schema version, recording/session/endpoint/owner identity, mode, UTC start and pinned endpoint/profile/workspace/map definitions. JSONL export embeds the same pinned snapshot in its recording header; ZIP includes it as `definitions.json`. Each evidence line has a strictly increasing recording sequence, UTC timestamp, monotonic elapsed nanoseconds, kind, group and canonical request/result. Sample lines include point ID, exact raw values, typed tagged value, unit and quality; the pinned group-to-map revision is in each part header. Traffic lines include actual captured request/reply hex when the runner supplies it, with absent frames explicitly absent. Write lines retain the durable command ID, request digest and acknowledged or delivery-unknown outcome. Neither browser display pause nor live-ring eviction alters the recording.

## Data / interface

Files live under Core's data directory at `modbus-recordings/<UUID>/<six-digit-part>.jsonl`, mode 0700 for directories and 0600 for files. SQLite owns metadata, a part manifest and an event index `(recording_id, sequence, part_no, offset, length, kind, unit, function, address, outcome, point_id)`. A part closes at the configured `part_bytes` ceiling; a single bounded event larger than the remaining space begins the next part, and an event larger than the ceiling is rejected rather than split. The file line is written and synced before its index transaction commits; recovery rebuilds only the indexed durable prefix and truncates a torn tail. Closed parts carry SHA-256, size and first/last sequence. JSONL is the lossless exchange format, including decimal strings for 64-bit values and tagged non-finite values.

## Examples

At a 1 MiB part ceiling, a 400 KiB first part followed by a 700 KiB event closes part 1 and puts the event wholly in part 2. A 2 MiB event is refused and the logical recording becomes incomplete with its previous durable sequence intact.

## Acceptance

Fake-runner tests verify ordered lossless evidence, pinned snapshots after definition edits, part rotation and digest reproducibility. Raw frame fields are never inferred from canonical request metadata.

## Assumptions and open decisions

The runner trace boundary must expose exact sent/received bytes without changing request validation. Network transports are the first browser workflow; physical RTU acceptance remains separate.
