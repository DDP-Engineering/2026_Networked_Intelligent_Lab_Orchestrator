<!--
Copyright (c) 2026 DDP Engineering
This software is licensed under the PolyForm Noncommercial License 1.0.0.
Commercial use requires a separate commercial license.
See the LICENSE.txt in the .agentic folder for full license details.
SPDX-License-Identifier: LicenseRef-PolyForm-Noncommercial-1.0.0
-->

# REQ-REL-e1048fd2 — Report interrupted Modbus recordings honestly

## Behavior

A successful Stop syncs and closes the current part and marks the logical recording complete. Failure to append, sync, index, rotate, reserve space or close marks it incomplete with a stable reason and last durable sequence. Polling continues after a logging failure, while logging stops and live status shows an explicit warning. On startup, active records become incomplete; Core verifies complete lines and part digests, truncates any torn tail, rebuilds the index for the durable prefix, and never reopens a hardware session or resumes logging automatically.

## Data / interface

Metadata has `status=active|completed|incomplete`, `reason`, `last_sequence`, `started_at`, `ended_at`, `size_bytes` and `part_count`. `last_sequence` advances only after file sync and index commit. A failed index insert can leave an extra durable line: recovery includes it after validation and still marks the recording incomplete. A missing or mismatched closed part is an integrity error, never silently treated as complete.

## Examples

If sequence 15 is synced and sequence 16 is torn, recovery retains through 15, truncates the tail and reports `incomplete` with `last_sequence=15`. If sequence 16 was synced but indexing failed, recovery indexes it and still reports incomplete.

## Acceptance

Fault-injection and restart tests cover every failure boundary, preserved durable prefixes and explicit status. No test accepts a complete flag over missing bytes.

## Assumptions and open decisions

The recorder stops accepting further evidence once failed. Polling continues and the live status shows the incomplete recording reason.
