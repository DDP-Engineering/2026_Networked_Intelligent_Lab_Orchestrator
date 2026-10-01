<!--
Copyright (c) 2026 DDP Engineering
This software is licensed under the PolyForm Noncommercial License 1.0.0.
Commercial use requires a separate commercial license.
See the LICENSE.txt in the .agentic folder for full license details.
SPDX-License-Identifier: LicenseRef-PolyForm-Noncommercial-1.0.0
-->

# REQ-FUNC-b0d59f49 — Control Modbus logging modes and sampling

## Behavior

The credential-bound owner starts and stops logging independently of polling. Start accepts `mode` in `values|traffic|both`, `sampling` in `every|interval|change`, and `interval_ms` from 100 to 60000 only for interval. Mode controls which evidence classes are retained. Every stores each poll result; interval stores the first result at or after each elapsed interval per point; change stores raw/typed/quality transitions per point. Error and quality transitions are always retained in values modes even when value sampling would otherwise suppress them. Traffic mode retains each request/result pair, including failures. Writes and recording failures remain explicit control evidence regardless of mode.

## Data / interface

`POST /api/v1/modbus/endpoints/{id}/recording` with session ID and start options returns status and logical recording ID. `DELETE` on that live control stops and finalizes; status exposes `recording_id`, `logging`, mode, sampling and warning. CLI `nilo modbus recording-start|recording-stop|recording-status` and shared MCP `modbus_recording_*` call the same Core methods. Repeated start returns conflict; stop when none is active returns a stable not-found response. No operation creates an individual write approval job.

## Examples

At interval 1000 ms, point A at 0/400/1100 ms yields retained samples at 0 and 1100 ms; a timeout at 500 ms is retained as a quality transition. Display pause does not affect either sample.

## Acceptance

Core/API/CLI/MCP and browser tests assert one shared behavior, effective status and controls, owner denial, mode filtering, quality transitions and independence from display/poll pause.

## Assumptions and open decisions

The reliability spec defines the logging-failure action; polling and browser display controls remain independent of logging.
