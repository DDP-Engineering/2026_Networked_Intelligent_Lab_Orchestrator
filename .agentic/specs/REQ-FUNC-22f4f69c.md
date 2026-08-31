# REQ-FUNC-22f4f69c - List simulated integrations

## Data / interface

Authenticated request:

```text
GET /api/v1/integrations
```

Healthy response:

```json
{
  "api_version": "nilo.http/1",
  "scenario": "healthy",
  "items": [{
    "id": "simulation.fake-python",
    "display_name": "SIMULATION - Fake Python Integration",
    "status": "healthy",
    "simulation": true,
    "operations": ["artifact", "crash", "echo", "fail", "sleep"]
  }]
}
```

The unavailable scenario uses the same identity and operations with status
`unavailable`. Array ordering and field values are deterministic.

## Acceptance

- The route uses the existing Bearer authentication boundary.
- Repeated requests for one scenario return semantically identical data.
- Only the readiness value changes between supported scenarios.
