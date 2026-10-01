<!--
Copyright (c) 2026 DDP Engineering
This software is licensed under the PolyForm Noncommercial License 1.0.0.
Commercial use requires a separate commercial license.
See the LICENSE.txt in the .agentic folder for full license details.
SPDX-License-Identifier: LicenseRef-PolyForm-Noncommercial-1.0.0
-->

# REQ-FUNC-90e44759 — Execute typed FC22 and FC23 writes

## Behavior

FC22 modifies one holding register with an AND mask and OR mask. It is a single device operation, never emulated as a host read/modify/write. FC23 writes 1–121 holding-register words then reads 1–125 holding registers in the same transaction; its returned values are read-side data, not an echo of the submitted write values. Both functions follow the existing preview, plugin-policy, credential-bound reservation, idempotency and no-retry write path. Lost acknowledgement remains `delivery_unknown`.

## Data / interface

Extend the canonical request with optional `and_mask` and `or_mask` (FC22 only), and `read_address` and `read_count` (FC23 only). Existing `address`, `count`, `values` name the write span for FC23; FC22 has count 1 and no values. All mask/word values are uint16, all spans fit 0–65535, and irrelevant fields are rejected. FC22 PDU is `16 address:u16 and_mask:u16 or_mask:u16` with an exact seven-byte normal echo. FC23 PDU is `17 read_address:u16 read_count:u16 write_address:u16 write_count:u16 byte_count:u8 words:u16[]`; the reply is `17 byte_count:u8 read_words:u16[]` with byte count exactly twice `read_count`. Both functions are classified as writes in the runner scheduler and never retried.

## Examples

FC22 at address 1 with masks `0xF2F2` and `0x2525` previews `160001f2f22525`. FC23 with one read at 2 and one word `0x1234` written at 4 previews `170002000100040001021234`. FC23 read count 126 or write count 122 is rejected before dispatch.

## Acceptance

Independent PDU/reply vectors, policy-denial and lost-reply tests prove payloads, echo/byte-count validation, distinct read values, idempotency and one transport attempt.

## Assumptions and open decisions

The existing plugin policy governs successive writes per the user's prior answer; no individual approval job is added.
