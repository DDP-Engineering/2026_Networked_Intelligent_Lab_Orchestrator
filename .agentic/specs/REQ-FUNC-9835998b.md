# REQ-FUNC-9835998b - Select a deterministic simulation scenario

## Data / interface

The simulation server accepts:

```text
--scenario healthy
--scenario unavailable
```

Omitting `--scenario` selects `healthy` for backward-compatible development
startup. Scenario identifiers are lowercase, stable, and exact.

## Acceptance

- `healthy` produces healthy integration and ready device inventory.
- `unavailable` produces unavailable integration and device inventory.
