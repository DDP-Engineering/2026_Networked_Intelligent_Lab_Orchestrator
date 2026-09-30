# REQ-REL-f258c750 - Durable Modbus write command

## Behavior

Core canonicalizes FC05/06/15/16, unit, zero-based address, count and values before policy or transport work. An owner and originating credential may submit a write only in a live pinned session whose plugin is enabled and whose admission policy permits successive writes. The idempotency key identifies one immutable operation for that session. Core stores the canonical JSON and SHA-256 digest with a `delivery_unknown` reservation before invoking the private runner. A committed reservation is never sent a second time. A matching retry returns its stored result; a different canonical request using that key conflicts. A failed or lost runner reply remains `delivery_unknown`; a verified runner outcome can replace it. A restart closes live sessions but retains command reservations and results.

## Data / interface

`modbus_commands` keys `(session_id,request_key)` with immutable `digest`, `request_json`, and mutable `result_json`; `session_id` references the durable `modbus_sessions` row. Canonical digest includes session identity, pinned revisions and the exact request; omitted values normalize to an empty array. Responses carry command ID, request key, digest, outcome, attempts and raw values. A result of `delivery_unknown` is never an instruction to retry with a new key.

## Examples

An FC06 request for unit 2, address 10, value 42 creates one reservation. A matching request with the same key yields the same command ID. Changing value to 43 conflicts. If the peer receives the frame but drops its reply, the reservation returns `delivery_unknown` and a later repeat still transmits zero new frames.

## Acceptance

An independent peer observes at most one write frame across concurrent duplicate submissions, lost replies and Core restart. Unsupported quantities and wrong revisions result in no frame and no reservation.
