# REQ-SEC-035a8f94 - Resolve operation permissions

## Behavior

Permission policy belongs to an integration app and its operations, not to an MCP token or client. Each request resolves to `allow`, `prompt`, or `deny` from an effective operation policy and app mode.

## Data / interface

App modes are `ask`, `auto`, and `full`. Operation policies are `auto_eligible`, `full_only`, `always_prompt`, and `denied`.

| Operation policy | Ask | Auto | Full |
|---|---|---|---|
| Auto-eligible | Prompt | Allow | Allow |
| Full-only | Prompt | Prompt | Allow |
| Always prompt | Prompt | Prompt | Prompt |
| Denied | Deny | Deny | Deny |

Resolution precedence is:

1. A `denied` effective operation policy always denies.
2. A persisted operation override replaces the manifest default.
3. An unknown integration app or device forces Ask mode.
4. A known device policy may lower effective authority but cannot silently raise the app mode.
5. The matrix produces the decision.

The decision snapshot records app mode, manifest default, operation override, device input, resolved policy, decision, reason, and time. Full Access may bypass equipment-protection limits but never bypasses authentication, schema validation, compatibility, resource leases, audit, cancellation, safe shutdown, `denied`, or `always_prompt`.

## Examples

- `auto_eligible` under Auto resolves to Allow.
- `full_only` under Auto resolves to Prompt.
- `full_only` under Full resolves to Allow.
- `always_prompt` under Full remains Prompt.
- `denied` under Full remains Deny.
- A newly discovered device under a Full app is treated as Ask until configured.

## Acceptance

A table-driven test asserts all twelve cells and separate tests cover unknown identity, persistent settings, operation override precedence, device authority lowering, Full Access boundaries, and decision snapshot content.
