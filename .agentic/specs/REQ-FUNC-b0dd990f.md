# REQ-FUNC-b0dd990f - Exchange integration lifecycle messages

## Behavior

Bench Core and one supervised integration runner communicate using protocol `nilo.integration/1`. The runner must complete `hello` successfully before any other method. Each request receives zero or more correlated events and exactly one terminal response.

## Data / interface

The transport is newline-delimited UTF-8 JSON over supervised stdin/stdout. Each complete line is at most 1 MiB. Stdout is reserved for protocol messages; stderr carries bounded diagnostics and is never parsed as protocol.

```json
{
  "protocol": "nilo.integration/1",
  "kind": "request",
  "message_id": "019...",
  "request_id": "",
  "method": "hello",
  "timestamp": "2026-08-30T12:00:00Z",
  "payload": {}
}
```

Every message has a unique `message_id`. A response or event copies the initiating request's `message_id` into `request_id`. Unknown envelope fields, unknown kinds, duplicate request message IDs, unsupported versions, malformed JSON, and a non-newline-terminated partial message are protocol errors.

The lifecycle methods are `hello`, `describe`, `discover`, `health`, `configure`, `execute`, `cancel`, and `safe_shutdown`. Typed event payloads cover progress in the closed interval `[0,1]`, timestamped measurements, domain events, warnings, bounded log references, and artifact declarations. Typed terminal payloads are either a validated result or a structured error.

Encoder writes are serialized so one JSON object and its newline are atomic with respect to other writers. All timestamps are UTC. EOF during an active request is a failure, not a successful terminal response.

## Examples

- An `execute` request can emit progress events at `0.25` and `0.75`, followed by one result response correlated to the same request.
- A second terminal response for the same request fails the connection contract.
- A diagnostic `print()` in a Python integration is redirected to stderr and cannot corrupt protocol stdout.

## Acceptance

Shared golden fixtures round-trip through Go and Python codecs. Boundary tests cover all methods, correlation, concurrency, strict fields, maximum size, oversized input, malformed input, EOF, diagnostics separation, and exactly-one-terminal enforcement.
