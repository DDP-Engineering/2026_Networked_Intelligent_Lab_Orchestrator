<!--
Copyright (c) 2026 DDP Engineering
This software is licensed under the PolyForm Noncommercial License 1.0.0.
Commercial use requires a separate commercial license.
See the LICENSE.txt in the .agentic folder for full license details.
SPDX-License-Identifier: LicenseRef-PolyForm-Noncommercial-1.0.0
-->

# REQ-DATA-797a97da — Stream faithful Modbus recording exports

## Behavior

An authorized download holds a deletion lease until its reader closes. JSONL preserves exact stored evidence lines with a recording header containing metadata and pinned definitions. CSV and readable text are human-facing projections over a selected sequence range; they include status, incomplete reason and quality and never invent missing raw frames. The ZIP bundle contains metadata, pinned definitions, numbered JSONL parts and a manifest of SHA-256 digests and sizes. Export verifies stored part digests before streaming and fails on mismatch. Requests and responses are bounded by context deadline and configured byte/range limits.

## Data / interface

`GET /api/v1/modbus/recordings/{id}/export?format=jsonl|csv|text|bundle&after=&through=` streams a body through `io.Pipe` with bounded buffers. JSONL is the canonical lossless interchange. Bundle paths are fixed server-generated names, not client paths. `manifest.json` names schema version, recording status and exact `sha256`, byte count and sequence range for each part. CSV has explicit UTF-8 columns for sequence, UTC time, relative nanoseconds, kind, unit, function, address, point ID, raw JSON, typed JSON, quality, outcome and hex. Text fields that would begin a spreadsheet formula (`=`, `+`, `-`, `@`, tab or carriage return) receive a leading apostrophe only in the CSV projection; JSONL and raw register data stay unchanged.

## Examples

A point label `=2+2` appears as `'=2+2` in CSV and exactly `=2+2` in JSONL. A changed part byte makes bundle export fail integrity verification rather than produce a successful manifest.

## Acceptance

Round-trip and stream-cancellation tests verify JSONL, CSV/text projection, bundle digests, lease release and bounded memory. A downloaded incomplete recording carries its incomplete reason and last durable sequence.

## Assumptions and open decisions

ZIP is the bundle container and JSONL is the lossless content.
