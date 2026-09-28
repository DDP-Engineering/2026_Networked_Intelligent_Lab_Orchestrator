# Nilo Settings

Administrators manage the four appliance-wide job settings in **System > Nilo Settings**. Camera stream capacity and session lifetime are under **System > Plugins > Camera**; serial connection capacity, lifetime, and idle disconnect are under **System > Plugins > Serial**. Each screen presents an editable table with the setting label, allowed range, value, and live usage where available. Save settings sits above the table. A `?` beside each label explains its effect and timing.

Changes take effect for new admissions immediately. Lowering a capacity lets existing work finish and prevents additional admissions until occupancy falls below the new limit. Each physical serial port and camera still has one controller at a time.

**View activity** beside the Nilo Settings title opens Activity filtered to audit
events for platform settings changes. The action appears when the session may
read Activity. Accepted settings replacements are already recorded as
`operating_settings.changed`; the filter shows their actor and changed values.

| Setting | Default | Allowed range | Unit |
|---|---:|---:|---|
| Executing finite jobs | 2 | 1–8 | jobs |
| Outstanding jobs, including approval and queue time | 32 | 1–256 | jobs |
| Active serial connections | 4 | 1–16 | connections |
| Active camera streams | 2 | 1–8 | streams |
| Default finite-job timeout | 60 | 1–3600 | seconds |
| Maximum finite-job timeout | 300 | 1–3600 | seconds |
| Serial connection lifetime | 60 | 1–1440 | minutes |
| Serial idle disconnect when logging is inactive | 1 | 1–60 | minutes |
| Camera session lifetime | 55 | 1–240 | minutes |

The default job timeout cannot exceed the maximum. Serial idle timeout cannot exceed serial lifetime. A finite job's deadline starts at admission, including approval and queue time. Jobs and connections keep their assigned deadlines when settings change. A new camera runner receives the configured session lifetime plus one minute for cleanup. Active serial and camera runners do not occupy finite-job worker slots after their setup jobs complete.

The finite-job timeout is an outer deadline. An integration may enforce a shorter operation-specific safety bound; the Sigrok capture worker, for example, bounds its hardware call to 30 seconds. Increasing the job timeout does not relax that worker bound.

## API and authorization

`GET /api/v1/platform/operating-settings` returns `revision`, the complete `settings` object, and live `usage` for the four capacities. `PUT` replaces the whole settings object and carries the revision from the last read. An administrator browser session or API token with `platform.settings.manage` may use both routes. Core returns 400 `operating_settings_invalid` for malformed or out-of-range values, 409 `operating_settings_conflict` for a stale revision, and 403 for a credential without the permission. Accepted changes and their numeric values are recorded in Activity as `operating_settings.changed`.

The browser uses the shared HTTP API. CLI and MCP instrument operations use the same Core Manager and therefore obey these limits; their current command/tool sets do not include a separate settings editor. An administrator automation client can manage settings through the HTTP API with a scoped token.

Each browser screen reads and submits the complete revisioned record, preserving fields shown on the other screens. A stale revision leaves the user's draft visible and asks for a reload. Contextual help uses the same accessible `?` button pattern as password policy; controls that need explanation should use this pattern beside their label.

## Other parameter inventory

| Category | Parameters | Decision |
|---|---|---|
| Existing administrator settings | Password policy, Activity retention, serial log retention, camera media retention | Keep their existing controls and ownership. |
| Candidate for separate follow-up | Browser session lifetime and login failure protection; Activity export *defaults*; network statistics sampling and history | Assess security, storage, CPU impact and interaction with existing policies before adding controls. |
| Fixed resource or safety boundary | Activity export maximum rows/bytes and query-page maximum; saved WireGuard profile cap; network confirmation and temporary recovery windows; serial buffer/transmit bounds; camera preview quality/dimensions; runner call timeouts; protocol message bounds, hardware capabilities and authorization rules | Keep as implementation or contract limits pending individual evidence and cross-component review. |

The proposed follow-up settings are separate from this operating-settings record. Their current values and source owners remain in the corresponding component documentation.
