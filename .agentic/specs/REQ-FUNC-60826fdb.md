# REQ-FUNC-60826fdb - Identify an appliance from the interface

## Behavior

`POST /api/v1/appliance/identify` is session-only, requires the existing
administrator-only `network.manage` permission, requires CSRF and accepts no
body. Raspberry Pi starts a
ten-second Identify pattern; a repeat moves expiry to ten seconds from the last
request without starting another loop. Expiry restores the recovery-derived
LED state. Generic hardware returns `supported=false` and touches no GPIO.

The two-second pattern is three repetitions of 200 ms on/200 ms off followed
by 800 ms off, for five cycles. Reset-pending and recovery-failure patterns take
precedence; Identify resumes only inside its original deadline. Software
Identify does not reannounce discovery; short-button Identify does.

## Data / interface

```json
{"supported":true,"active_until":"2026-09-15T12:00:10Z"}
{"supported":false,"reason":"indicator_unavailable"}
```

Unsupported is HTTP 200. Helper failure is bounded
`503 identify_unavailable`. Concurrent requests serialize through one
coordinator.

## Acceptance

- Permission, CSRF and empty-body rules match session mutations.
- Fake-clock/GPIO tests settle cadence, coalescing, precedence and expiry.
- Generic/simulation paths open no GPIO.
- Previous indicator state returns after expiry.
- The dedicated audit contract covers every request.

## Resolved design decisions

Identify is a physical side effect and uses `network.manage`; read-only identity
and supported-state metadata use `network.read`.
