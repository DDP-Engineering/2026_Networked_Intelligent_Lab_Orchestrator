<!--
Copyright (c) 2026 DDP Engineering
This software is licensed under the PolyForm Noncommercial License 1.0.0.
Commercial use requires a separate commercial license.
See the LICENSE.txt in the .agentic folder for full license details.
SPDX-License-Identifier: LicenseRef-PolyForm-Noncommercial-1.0.0
-->

# REQ-DATA-5084f2d5 — Retain and export truthful Modbus scan outcomes

## Behavior

Each settled probe records selected unit/function/address, UTC time, raw returned Boolean or 16-bit word, outcome and exception code where present. A timeout, exception, cancellation, disconnected or protocol error is an attempted probe with its own outcome; none means that a unit is absent. A successful read carries no writable or engineering-type claim. The bounded result page and export preserve partial/cancelled status and exact scan selection.

## Data / interface

Scan results are session-owned, in-memory and capped by the plan's 512 probes; they are available for inspection/export until the live session ends or Core restarts. `GET /api/v1/modbus/endpoints/{id}/scans/{scan_id}` returns plan, state, total/completed counts and terminal reason. `/results?after=` returns at most 100 results with a sequence cursor, count and truncation. `/export?format=csv|jsonl` streams a header containing the plan and status plus the exact bounded result set. CSV text is escaped for spreadsheet formulas; raw numeric words are unchanged. Export after cancellation is allowed while the session still exists.

## Examples

Unit 2/address 11 with timeout yields `outcome=timeout`, empty values and no `absent` field. A successful FC03 read of `42` yields raw `42` without `writable=true` or a guessed temperature type.

## Acceptance

Mixed-result fixtures verify page/cursor order, finite memory, CSV/JSONL content, partial status and owner-only export.

## Assumptions and open decisions

The ticket requires export but not durable scan history; saved session recordings remain the durable evidence mechanism when logging is separately enabled. This transient scan lifetime is a proposed contract until design confirms it against Core APIs.
