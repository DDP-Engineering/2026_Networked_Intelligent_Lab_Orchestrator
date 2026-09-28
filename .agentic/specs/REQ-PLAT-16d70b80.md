# REQ-PLAT-16d70b80 - Persist bounded operating settings

## Behavior

Core keeps one revisioned appliance-wide operating-settings row. The database migration seeds revision 1 with the current defaults. Replacement is atomic, complete, and compare-and-swap by revision. Values are integers. Lowering a capacity leaves admitted work running. Capacity reads include current usage; usage is live and is not stored in the settings row. Settings are applied immediately to new admissions; timeout and lifetime values are copied into work at its start.

## Data / interface

`GET /api/v1/platform/operating-settings` returns `{ "revision": number, "settings": { ... }, "usage": { "executing_jobs": number, "outstanding_jobs": number, "serial_connections": number, "camera_streams": number } }`. `PUT` accepts `{ "revision": number, "settings": { ... } }` and returns the same response shape. Both routes require `platform.settings.manage`, granted to administrators. Bearer tokens additionally require that scope. A stale revision returns 409 `operating_settings_conflict`; an invalid body or range returns 400 `operating_settings_invalid`; unavailable storage returns 503.

Fields and inclusive bounds:

| Field | Unit | Default | Range |
|---|---:|---:|---:|
| `executing_jobs` | jobs | 2 | 1-8 |
| `outstanding_jobs` | jobs | 32 | 1-256 |
| `serial_connections` | connections | 4 | 1-16 |
| `camera_streams` | streams | 2 | 1-8 |
| `default_job_timeout_seconds` | seconds | 60 | 1-3600 |
| `max_job_timeout_seconds` | seconds | 300 | 1-3600 |
| `serial_lifetime_minutes` | minutes | 60 | 1-1440 |
| `serial_idle_minutes` | minutes | 1 | 1-60 |
| `camera_lifetime_minutes` | minutes | 55 | 1-240 |

The default job timeout is at most the maximum. The serial idle timeout is at most its lifetime. The camera runner deadline is one minute longer than the session deadline to allow orderly cleanup. The job timeout counts from admission, including approval and queue time. An explicit client timeout is accepted only in 1 through the configured maximum. Capacity and timeout settings are independent: outstanding jobs need not be greater than executing slots, because a lower outstanding limit can intentionally block new work while current jobs finish.

## Examples

At defaults, two jobs can execute, 32 nonterminal jobs can exist, four distinct serial ports can connect, and two cameras can stream. Changing `serial_connections` to 2 while three ports are connected leaves all three in place and rejects new connections until fewer than two remain. A job admitted with a 60-second deadline retains that deadline after an administrator changes the default to 120 seconds.

## Acceptance

Fresh and reopened SQLite databases return the complete defaults or last accepted replacement. Invalid and stale writes preserve the previous revision and values. GET usage reflects live Manager state. GUI, HTTP and service tests agree on ranges, effective timing and error codes.

## Assumptions and open decisions

The ticket proposes new-work-only timeout/lifetime changes; that rule is adopted here. The bounds are conservative resource ceilings for the initial platform implementation and can be revisited through a later documented setting-contract change. Protocol message limits, hardware-specific capabilities and authorization remain fixed boundaries.
