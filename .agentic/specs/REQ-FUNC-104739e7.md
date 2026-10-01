<!--
Copyright (c) 2026 DDP Engineering
This software is licensed under the PolyForm Noncommercial License 1.0.0.
Commercial use requires a separate commercial license.
See the LICENSE.txt in the .agentic folder for full license details.
SPDX-License-Identifier: LicenseRef-PolyForm-Noncommercial-1.0.0
-->

# REQ-FUNC-104739e7 — Read typed Modbus identity and serial diagnostics

## Behavior

FC43/14 reads device identification on a supported live transport with a typed request and bounded object response. The selected read-only serial diagnostics are FC11 Get Communication Event Counter and FC17 Report Server ID on RTU and RTU-over-TCP only; native TCP returns unsupported before I/O for those serial-specific functions. Device exceptions and unsupported functions are explicit outcomes. FC08, broadcasts and arbitrary raw frames remain unavailable.

## Data / interface

Identity request PDU is `2B 0E read_code object_id`, initially basic read code 1 with object ID 0. The reply must echo MEI 14/read code and contain valid conformity, more-follows, next-object and bounded `(object_id,length,bytes)` entries within the 253-byte Modbus PDU ceiling. Return typed vendor/product/revision objects where provided, plus raw UTF-8-safe text or hex for unknown object IDs and the continuation marker; do not assume a missing optional object. FC11 PDU is `0B`, normal response contains status and event count (two uint16 words). FC17 PDU is `11`, normal response has bounded byte count, server ID, run indicator and optional additional bytes. All requests stay unicast within the pinned endpoint's unit range.

## Examples

Basic identity with vendor `ACME` yields object ID 0 as `ACME` and no claim about absent optional objects. Native TCP FC11 is refused before contacting the peer. An FC43 reply with a mismatched MEI is a protocol error.

## Acceptance

Independent request/reply fixtures, malformed byte counts, exceptions and transport capability tests prove typed output and refusal of FC08/raw/broadcast.

## Assumptions and open decisions

The plan names selected serial diagnostics but does not require FC08; FC11 and FC17 are the non-disruptive selection. Physical serial support remains MB-10 hardware acceptance.
