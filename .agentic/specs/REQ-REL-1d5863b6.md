# REQ-REL-1d5863b6 — Reconcile device presence without losing records

## Behavior
After startup, records begin unknown. A bounded successful discovery atomically marks observed hardware present and formerly known but unseen hardware absent, while retaining name, configuration, id and history. A failed discovery leaves unknown rather than claiming absence. Newly present configured enabled cameras initialise their configured baseline through a system-actor reset under the device lease; user submissions are refused while initialising or after failed initialisation. New unconfigured cameras receive a unique suggested label but refuse operations until an administrator accepts configuration. Browser authentication events have no camera-reset effect.
## Data / interface
Public states are unknown, present-initializing, present-ready, present-unconfigured and absent, plus a structured last-reset status. Submit checks configured, present, ready and enabled. Reset points are startup/plugin enable, device enable, configured-baseline change, discovered reconnect and explicit reset. Discovery is explicit except the bounded startup/enable pass.
## Examples
If discovery fails while a prior record exists, its presence is unknown. If a configured camera reconnects, it is initialising until reset read-back succeeds.
## Acceptance
State-machine tests cover success, failure, reconnect, new device, busy reset and authentication events.
## Assumptions and open decisions
The implementation may store presence and readiness as separate columns; public states and refusal codes are fixed.
