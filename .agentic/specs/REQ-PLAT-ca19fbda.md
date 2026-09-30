# REQ-PLAT-ca19fbda — Store bounded administrator Modbus operating settings

## Behavior

Core owns one appliance-wide revisioned Modbus settings record. Only plugin administrators can change it. Revision conflict and invalid values leave it unchanged and are reported. The active-session ceiling gates new connection and test admissions; reducing it does not terminate a live session. New sessions use the current lease and inactivity lifetime. Recording retention values are stored for the recording service added with retained sessions; the administration page identifies them as future recording limits until that service exists.

## Data / interface

`GET /api/v1/modbus/settings` requires Bench read; `PUT /api/v1/modbus/settings` requires `plugins.manage` and an `expected_revision`. Values: `max_sessions` integer 1–16 (default 4); `session_lease_seconds` integer 60–3600 (default 3600); `idle_seconds` integer 30–3600 (default 300); `retention_count` integer 1–1000 (default 100); `retention_bytes` integer 1 MiB–16 GiB (default 2 GiB); `part_bytes` integer 1 MiB–256 MiB (default 100 MiB) and no larger than `retention_bytes`. The JSON parser requires all fields and rejects unknown fields. New settings take effect on new sessions only. The existing platform settings for cross-plugin jobs and stream capacity stay separate.

## Examples

- Lowering `max_sessions` from 4 to 1 with two active sessions preserves both and refuses a third.
- Raising it to 3 then admits a third when endpoint ownership permits.
- A stale expected revision returns conflict and does not change the stored record.

## Acceptance

SQLite round-trip and Manager tests cover default, restart, CAS, audit, exact bounds, unauthorized update, and the active/new-session distinction. Frontend test displays units, allowed ranges and future-retention status.

## Assumptions and open decisions

The values are bounded software defaults from the implementation plan, not physical hardware capacity guarantees. Raspberry Pi and amd64 target measurements remain required before release. The MB-08 recording service must consume the stored retention policy when it is added; this ticket does not claim recording enforcement without that service.
