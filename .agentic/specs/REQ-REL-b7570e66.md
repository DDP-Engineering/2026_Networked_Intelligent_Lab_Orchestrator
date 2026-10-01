<!--
Copyright (c) 2026 DDP Engineering
This software is licensed under the PolyForm Noncommercial License 1.0.0.
Commercial use requires a separate commercial license.
See the LICENSE.txt in the .agentic folder for full license details.
SPDX-License-Identifier: LicenseRef-PolyForm-Noncommercial-1.0.0
-->

# REQ-REL-b7570e66 — Keep Modbus scans fair and cancellable

## Behavior

The scan worker submits one probe at a time to the existing per-endpoint runner scheduler, leaving poll requests eligible between probes. Progress counts only settled probes. Cancellation stops future submissions, cancels queued work, and treats an in-flight unknown read as cancelled or timed out without inventing an absent unit. Session stop, credential revocation, plugin disable and deadline finish the scan with a terminal reason.

## Data / interface

States are `running`, `completed`, `cancelled`, `deadline`, `session_ended` and `failed`. A monotonic `completed_probes` count never exceeds `total_probes`. The worker owns its context and bounded goroutine lifetime; HTTP client disconnect does not itself cancel a server-owned scan. `DELETE /api/v1/modbus/endpoints/{id}/scans/{scan_id}` requests cancellation. Polling and scans use the runner's single-transport-request scheduler; no scan creates a second transport owner.

## Examples

After 3/10 settled probes, Cancel yields 3 completed results and `cancelled`; a queued fourth probe is never reported as absent. A poll group due between scan probes is serviced before a later scan probe.

## Acceptance

Deterministic scheduler/service tests show no overlapping requests, poll progress, bounded cancellation and truthful terminal status for deadline, revocation and disconnect.

## Assumptions and open decisions

Transport deadline and session cleanup retain their existing authority; exact per-scan policy admission awaits the user answer in the scan-plan spec.
