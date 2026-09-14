# REQ-FUNC-fb666359 — Return network access to commissioning on a long press

## Behavior

A commissioning-reset hardware event is accepted only while the appliance is
commissioned. NILO first restores any non-idle temporary recovery record. It
then derives the safe provisioning selection from current eligible physical
interfaces minus all preserved protected roles and prepares that management
admission. No eligible interface is a valid result; provisioning then admits
nothing until a safe interface appears.

The durable reset transaction changes lifecycle from `commissioned` to
`provisioning`, replaces the explicit management selection with the safe
derived selection (or none), replaces temporary network recovery with an idle
tombstone carrying the highest handled hardware-event generation, and appends
the bounded audit event. The tombstone carries no adapter, helper record, alias,
deadline, or other temporary effect. It does not delete or rewrite stable appliance
identity/hostname, TLS material, users, password verifiers, sessions, API
tokens, audit history, integrations, PLC/device-network assignments, saved
interface settings, or unrelated NetworkManager profiles.

The helper firewall is widened to the derived selection before the durable
transaction so a successful reset remains reachable. If preparation or the
transaction fails, NILO restores the commissioned firewall/admission and leaves
the lifecycle and explicit management selection unchanged. After commit,
provisioning reconciliation owns recovery from a process interruption.

## Data / interface

The commissioning repository exposes a typed compare-and-transition operation
whose expected source is `commissioned`; it atomically writes lifecycle,
management selection, the idle replay tombstone, and audit record in one SQLite write
transaction. A source already `provisioning` returns a typed no-op/refusal and
does not repeat mutation.

The reset carries no user-supplied erase mask. Password recovery and factory
reset remain different workflows and cannot be selected by this hardware event.

## Examples

A commissioned unit using `wlan0`, with `eth0` assigned to a PLC and safe
unassigned `eth1`, preserves the PLC record and derives provisioning on `wlan0`
and `eth1` only. Identity, certificate and administrator login continue to
work.

If firewall preparation for `eth1` fails, the appliance remains commissioned
on its original explicit selection and the failed reset is audited.

## Acceptance

- Recovery restoration precedes reset mutation.
- Protected roles remain persisted and excluded from derived provisioning.
- The named preserved records compare unchanged after success and every
  injected failure.
- Lifecycle, derived selection, recovery-effect clearance, replay tombstone and
  audit commit atomically.
- Failure leaves commissioned access intact; restart reconciles a committed
  provisioning state.
- No password or factory-reset behavior is reachable from this event.

## Resolved design boundary

TLS material is external to the SQLite data transaction and is untouched by
this operation.
