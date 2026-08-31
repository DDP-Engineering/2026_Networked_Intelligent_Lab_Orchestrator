# REQ-FUNC-346ffa07 - List simulated devices

## Data / interface

Authenticated request:

```text
GET /api/v1/devices
```

Healthy response:

```json
{
  "api_version": "nilo.http/1",
  "scenario": "healthy",
  "items": [{
    "id": "simulation.fake-python.device.stable-001",
    "integration_id": "simulation.fake-python",
    "display_name": "SIMULATION - Electronics Bench 001",
    "kind": "simulated-electronics-bench",
    "status": "ready",
    "simulation": true,
    "capabilities": ["artifact", "crash", "echo", "fail", "sleep"]
  }]
}
```

The unavailable scenario uses the same identity and capabilities with status
`unavailable`. Array ordering and field values are deterministic.

## Acceptance

- The route uses the existing Bearer authentication boundary.
- The device always references the returned simulated integration.
- Only the readiness value changes between supported scenarios.
