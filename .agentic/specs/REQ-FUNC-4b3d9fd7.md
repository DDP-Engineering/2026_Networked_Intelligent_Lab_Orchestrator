# REQ-FUNC-4b3d9fd7 - Serve the simulation status API

## Behavior

The `nilo serve` command starts a simulation server when `--profile simulation`
is explicitly selected. It also supports the appliance profile. Simulation
requires an absolute data directory and an explicit loopback listener address.
The server runs until its context is cancelled or the process receives `SIGINT`
or `SIGTERM`.

`GET /api/v1/status` requires an authenticated browser session or Bearer token;
there is no unauthenticated alternative route.

## Data / interface

Command:

```text
nilo serve --profile simulation --data-dir <absolute-path> --bind <loopback-host:port>
```

Successful status response:

```json
{
  "api_version": "nilo.http/1",
  "status": "healthy",
  "profile": "simulation",
  "simulation": true,
  "build": {
    "name": "nilo",
    "version": "<project VERSION>",
    "integration_protocol": "nilo.integration/1"
  }
}
```

The example's version placeholder is the current root `VERSION`, mirrored by
`internal/buildinfo`. The response uses `application/json`. Unsupported methods
return `405`; unknown paths return `404`. Protected requests use a session
cookie or `Authorization: Bearer <token>`.
Authentication failures return `401` with `WWW-Authenticate: Bearer` and a
small JSON error object that contains no status representation.

Development token command:

```text
nilo development issue-token --data-dir <absolute-path>
```

It creates a unique non-login development identity, persists only the existing
one-way token verifier, and emits the token ID, user ID, plaintext token, and
creation time once as JSON. This command is development tooling, separate from
production administrator bootstrap.

## Acceptance

- The status schema is exact and identifies simulation unambiguously.
- Valid persisted tokens authenticate after the database is reopened.
- Missing, malformed, unknown, and revoked tokens cannot access the response.
- The simulation listener never opens on wildcard or non-loopback addresses.
- Cancellation closes the listener within the configured shutdown timeout.
