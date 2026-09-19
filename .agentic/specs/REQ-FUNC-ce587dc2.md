# REQ-FUNC-ce587dc2 — Inventory and remove NILO-owned appliance state

## Behavior

The maintenance service derives cleanup only from durable ownership records
and package inventory. `remove` stops owned services and removes immutable
package state while preserving configured mutable state. `purge` additionally
backs up and removes selected mutable state and owned external network effects
after local confirmation. An unproven owner or sole remote management path is
retained and reported.

## Data / interface

```text
nilo maintenance inventory --data-dir <absolute>
nilo maintenance remove --data-dir <absolute>
nilo maintenance purge --data-dir <absolute> --confirm-local
```

The inventory classifies irreplaceable state, regenerable state, immutable
package state and operational artifacts. Network objects require exact stored
IDs/UUIDs plus NILO ownership metadata; name prefixes alone grant no authority.
Maintainer scripts call non-interactive preserve-state removal only.

## Examples

Removal leaves `nilo.db`, identities, integration state and configuration.
Confirmed purge first creates a verified snapshot, then removes those records
and exact owned NetworkManager/nftables/mDNS/runtime resources. A foreign
profile named `nilo-looking` survives.

## Acceptance

Inventory is deterministic and secret-free. Partial helper failures retain
unsettled records and return non-zero. Remote-only management cleanup refuses
without the local confirmation marker.

## Assumptions and open decisions

Logs and captured artifacts follow their configured retention/export policy;
purge reports retained operator-selected exports.
