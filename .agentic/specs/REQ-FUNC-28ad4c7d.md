<!--
Copyright (c) 2026 DDP Engineering
This software is licensed under the PolyForm Noncommercial License 1.0.0.
Commercial use requires a separate commercial license.
See the LICENSE.txt in the .agentic folder for full license details.
SPDX-License-Identifier: LicenseRef-PolyForm-Noncommercial-1.0.0
-->

# REQ-FUNC-28ad4c7d — Bound explicit Modbus read scans

## Behavior

A scan starts only from an explicit owner action inside an authorized live session. The caller selects FC01/02/03/04, inclusive unit and wire-address ranges, a probe rate and a deadline. Each probe requests one bit/register at one address; the plan never infers subnet peers, units outside the selected range, address bases, engineering types or write capability. Reject the whole plan before I/O if any field or endpoint-unit range is invalid. At most one scan runs on an endpoint session.

## Data / interface

The initial contract caps units at 1–247 on RTU/RTU-over-TCP or the pinned TCP endpoint range, addresses at 0–65535, total probes at 512, rate at 1–10 probes/second and deadline at 1–60 seconds. The first probe is immediately eligible; later probes are paced with a monotonic clock. `POST /api/v1/modbus/endpoints/{id}/scans` accepts session ID, function, inclusive `unit_start/end`, inclusive `address_start/end`, `rate_per_second`, `deadline_seconds` and an idempotency key; it returns a scan ID and status. Invalid/oversized plans return invalid input; a concurrent scan returns busy. CLI and MCP use the same service.

## Examples

Units 1–2 and addresses 10–11 with FC03 produce four one-register probes in unit-major, address-minor order. Units 1–247 and all addresses are refused before any dispatch.

## Acceptance

Focused service/API tests prove finite validation, selected unit/address coordinates, session and endpoint bounds, duplicate submission behavior and absence of automatic scans.

## Assumptions and open decisions

The user's answer will settle whether the existing policy-approved live session plus explicit Scan action is sufficient, or each scan needs its own policy job. No code path may silently substitute one for the other.
