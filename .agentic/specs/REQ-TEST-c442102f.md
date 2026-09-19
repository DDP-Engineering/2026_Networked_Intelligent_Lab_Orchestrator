# REQ-TEST-c442102f — Report unavailable hardware acceptance honestly

## Behavior

Every preparation receipt contains a `hardware_acceptance` object sourced from
the selected target definition:

```json
{
  "status": "not_run",
  "required_checks": ["boot", "network", "integrations"]
}
```

The preparation interface exposes no argument that can set this status.
Cross-compilation, emulation, static validation, and an offline cache check do
not change it. A later hardware-acceptance workflow may produce separate
evidence; it does not rewrite the deterministic preparation receipt.

## Data / interface

`status` is exactly `not_run`. `required_checks` is the target registry's
validated, sorted checklist. The receipt contains no `passed`, `successful`,
or inferred observation field.

The initial Ubuntu checklist covers physical boot/reboot, NetworkManager and
management access, helper/core/GUI operation, USB device access, and production
integration health. The Raspberry Pi checklist covers image flashing and card
boot, boot/reboot and power cycling, filesystem expansion, unique identity,
administrator creation and one-time credential consumption, NetworkManager,
helper/core/GUI operation, integration health, explicitly enabled and wired
GPIO recovery, and normal operation while GPIO recovery is disabled or
unavailable. Each checklist is part of the exact target definition; it cannot
be reduced or replaced by registry input.

## Examples

Preparing `rpi-debian13-arm64` on an amd64 build host still records `not_run`
and includes the physical GPIO check. Preparing Ubuntu without the mini-PC
records the Ubuntu checklist with the same status.

## Acceptance

Tests reject reduced target checklists, prepare both target fixtures under
injected offline observations, and compare the exact target-specific receipt
checklists. CLI help and parser tests confirm no hardware status override
exists.

## Assumptions and open decisions

The physical machines are unavailable for this task. Hardware acceptance
remains a release gate outside the implementation increment.
