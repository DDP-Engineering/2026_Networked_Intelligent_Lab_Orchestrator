<!--
Copyright (c) 2026 DDP Engineering
This software is licensed under the PolyForm Noncommercial License 1.0.0.
Commercial use requires a separate commercial license.
See the LICENSE.txt in the .agentic folder for full license details.
SPDX-License-Identifier: LicenseRef-PolyForm-Noncommercial-1.0.0
-->

# REQ-FUNC-f533ef04 — Restart or power off the appliance

## Behavior

The power surface controls the Linux appliance, not only the Bench Core process.
It accepts exactly two actions: `restart` requests an orderly operating-system
reboot and `power_off` requests an orderly operating-system halt. An accepted
HTTP request dispatches one corresponding privileged operation and is never
retried automatically. Sign out remains the existing independent session action.

Only a live `appliance` profile receives a controller backed by the installed
privileged helper. Simulation uses an unavailable controller. Controller errors
are returned before acceptance when the process remains alive; if connectivity
disappears before the browser receives the response, the browser reports the
outcome as unconfirmed rather than claiming success or retrying.

## Data / interface

HTTP request:

```text
POST /api/v1/system/power
Content-Type: application/json
X-NILO-CSRF: <matching browser-session token>

{"action":"restart"}
{"action":"power_off"}
```

The request accepts no other fields and is limited to a small JSON body. It is
session-only and requires `system.power.manage`. Success is `202 Accepted`:

```json
{"api_version":"nilo.http/1","action":"restart","status":"accepted"}
```

`action` echoes the accepted action. `status` is always `accepted`. Invalid JSON,
an unknown action or an extra field returns `400 invalid_power_action`; a missing
or unusable credential returns `401`; a Viewer, a bearer token or invalid CSRF
returns `403`; unavailable simulation/controller returns
`503 power_control_unavailable`; a durable audit failure returns
`500 power_audit_failed`; and a helper rejection returns
`500 power_action_failed` while the process remains able to answer.

The helper request contains only its existing envelope and one of these exact
operation names, with every other request field empty:

```text
system.restart
system.power_off
```

The helper maps them to the system D-Bus login manager methods
`org.freedesktop.login1.Manager.Reboot(false)` and
`org.freedesktop.login1.Manager.PowerOff(false)`. The Boolean disables
interactive authorization; the root helper already is the installed authority.
No executable, shell, target, delay, environment or arbitrary argument crosses
the socket.

Before dispatch, Bench Core appends `system.power.requested` with component
`system`, subject type `appliance`, target label `restart` or `power_off`, the
authenticated user actor, session credential and requested outcome. Failure to
write this event prevents dispatch. Access denials continue through the shared
permission-denial audit path.

## Examples

- An Operator browser session with matching CSRF submits `restart`: the audit
  write completes, the helper receives `system.restart` once, and the endpoint
  answers 202 if the connection remains available.
- An Administrator cancels the Power off dialog: the client sends no request.
- A Viewer posts `power_off`: Bench Core returns 403 and the helper is untouched.
- An Administrator bearer token posts `restart`: Bench Core returns 403 even
  though the owner role contains the permission.
- Simulation receives a direct valid request: Bench Core returns 503 and no
  helper socket is opened.
- The HTTP connection closes after confirmation but before a response: the GUI
  reports that acceptance could not be confirmed and does not retry.

## Acceptance

- Operator and Administrator sessions can dispatch both actions; Viewer and
  bearer credentials cannot.
- Session CSRF, strict JSON decoding, body bounds, permission checks and audit
  completion precede privileged dispatch.
- The helper accepts only the two empty, exact power operation envelopes and
  calls the corresponding login-manager method once.
- Simulation and the browser mock produce no host power effect.
- The account menu and confirmation flow satisfy the linked UX requirements in
  English, German and Romanian without changing Sign out.
- Automated tests are green; the bench Pi restart changes the boot ID and
  returns healthy; the user separately records the power-off/power-on result.

## Assumptions and open decisions

The existing `nilo-nethelper` name is retained for this bounded change even
though its privileged responsibility becomes broader than networking. A future
helper rename or split is a separate migration. Power-off recovery is intentionally
human-operated and therefore is not invoked by repository automation or the agent.
