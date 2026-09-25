# REQ-DATA-3e86a281 — Validate revisioned plugin and device settings

## Behavior
The installed manifest supplies plugin and device JSON schemas. Administrator updates validate against those schemas and a last-observed revision, and commit atomically with the next revision. A stale revision returns a conflict without changing stored settings. Device names are Unicode plain text of 1..64 characters, unique per plugin by NFKC Casefold; purpose is at most 200 characters. A camera's baseline controls, mounting correction, capture/processing defaults, calibration and enable state belong to device settings, while hard image limits and preview limits belong to plugin settings.
## Data / interface
Plugin configuration and each managed device expose `revision` with settings. The device configuration schema reference is confined within the installed package like the existing app configuration schema. An update carrying a structurally valid but out-of-schema field fails validation. Only administrators with plugins.manage mutate these records.
## Examples
Two concurrent writes against revision 3 yield one revision 4 and one conflict. Canonically equivalent names cannot be assigned to two devices in one plugin.
## Acceptance
Repository and route tests cover revision races, Unicode names, schema refusal, permission denial and surviving restart.
## Assumptions and open decisions
Exact stored JSON columns and API resource paths are design choices; canonicalisation semantics are fixed.
