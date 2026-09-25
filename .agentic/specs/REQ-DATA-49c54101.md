# REQ-DATA-49c54101 — Preserve managed device identity

## Behavior
A managed device has separate immutable record id, current physical-hardware binding and validated capture endpoint. Record ids are monotonic per plugin (`cam-1`, `la-1`) and never reused. Stable serial matching uses vendor, product and serial only when the serial is nonempty and not persistently classified weak. A serial becomes weak when it matches a reviewed junk value or two simultaneously or historically observed devices share it. Otherwise matching uses vendor, product and physical port path. Replugging a serial-matched device retains the record; moving a port-matched device creates a new record. The former record remains absent.
## Data / interface
The runner reports vendor/product, serial, port path, by-id names, candidate endpoints, interface number, capabilities and selected capture endpoint. The core validates endpoint paths against sysfs and V4L2 capability evidence and retains paths internally. The public record exposes id, kind, identity attributes safe to display, match rule and endpoint capability summary, without a writable node path. A rebind may move one new provisional hardware binding to exactly one absent configured record only if the provisional record has no accepted job, artifact or administrator configuration; it tombstones the provisional id.
## Examples
Two cameras with the same serial at two ports retain two port-bound records after one is unplugged. A serial camera moved to another port retains its original record.
## Acceptance
Reconciliation tests cover strong, blank, junk and duplicate serials, multi-node grouping, restart and constrained rebind.
## Assumptions and open decisions
The exact sysfs evidence fields and reviewed junk serial list will be finalised after target probe records. No device-node path becomes public input.
