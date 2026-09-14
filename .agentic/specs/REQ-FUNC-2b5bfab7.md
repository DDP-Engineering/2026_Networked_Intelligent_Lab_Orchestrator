# REQ-FUNC-2b5bfab7 — Arm recovery for a subsequent Ethernet link

## Behavior

The recovery coordinator has the durable states `idle`, `armed`, `active`,
`restoring`, and `failed`. It runs only for a commissioned appliance with
Raspberry Pi hardware recovery enabled. A recovery button event received while
provisioning is recorded as refused and does not change networking.

On a recovery-duration release in `idle`, NILO records `armed_at`,
`expires_at = armed_at + 10 minutes`, a strictly increasing hardware-event
generation, and the stable adapter IDs of every Ethernet link currently
connected. Merely lacking DHCP or an address is not a transition. A candidate
must subsequently be observed disconnected then connected, absent then
connected, or disconnected at arm then connected.

Each reconciliation compares the current link sample with the durable pre-arm
and previous-link sample. When several newly connected candidates are eligible
in the same sample, canonical stable adapter ID order selects the winner. Once
one candidate starts activation, all others are ignored for that window. The
contract deliberately does not infer sub-sample ordering from Linux interface
names or polling order; only one observed sample is available to core.

A recovery-duration release in `armed` or `active` replaces `expires_at` with
ten minutes after the new release. It does not change the selected interface,
rerun activation, or reopen candidate selection. Expiry in `armed` returns to
`idle`; expiry in `active` enters `restoring`.

## Data / interface

The privileged helper exposes a typed hardware event status carrying only an
opaque event ID, `generation`, `action` (`arm_recovery` or
`reset_commissioning`), and UTC release time. A recovery-duration event also
carries a sorted set of stable adapter IDs that had carrier at release and a
snapshot-valid flag; reset events carry no link snapshot because they never
select a recovery link. Core acknowledges the handled event identity, so
polling and restart do not execute one release twice. The helper retains one
coalesced unacknowledged event in its root-owned
recovery record: a reset supersedes recovery, a pending reset cannot be
weakened, and a later recovery release replaces an earlier recovery release so
it extends from the real release time. No shell command, GPIO path, raw
NetworkManager settings, or responder text crosses this interface.

The core recovery record contains state, event generation, UTC arm/expiry time,
pre-arm connected stable IDs, selected stable ID/name, helper recovery
record identity, bounded last outcome code, and an ordered queue of at most the
eight distinct lifecycle audit markers that still need durable append. A marker
is persisted in the same first durable state transition as the corresponding
post-effect failure or cancellation, so a crash cannot create an unaudited
advanced state. The record contains no profile text, credentials, key material,
or arbitrary error strings.

After restart, an expired `armed` record closes immediately. `active`,
`restoring`, or `failed` records reconcile restoration before accepting another
event or candidate; startup does not silently re-adopt and extend an existing
recovery exposure. Remaining duration is never extended by restart or wall
clock rollback beyond the original ten-minute bound: each process also tracks
an independent steady deadline, and a persisted last wall-clock observation
makes a backward discontinuity after restart expire fail closed. A release timestamp that
appears to be in the future is audited and durably consumed as refused, so it
cannot become actionable when wall time later catches up.

## Examples

If `eth0` is connected and `eth1` is disconnected when generation 7 arms
recovery, reconnecting `eth0` changes nothing. A later link-up on safe `eth1`
may activate it. A press at minute 8 moves expiry to minute 18 and retains
`eth1`.

If `eth1` and `eth2` become eligible in the same sample, the lower canonical
stable adapter ID wins and only that adapter reaches helper activation.

## Acceptance

- An event executes at most once across polling and restart.
- No pre-arm connected adapter becomes a candidate.
- DHCP absence and passage of time alone never activate recovery.
- The first deterministic eligible transition wins and only one activation is
  in flight.
- The ten-minute deadline and repeat-press extension use release time.
- Nonterminal state reconciles before another recovery action.

## Resolved design boundary

SQLite stores the core recovery interlock and highest handled generation,
including an idle tombstone after reset; the helper atomically stores one
coalesced unacknowledged event in its private state. The helper captures the
release-time link snapshot from NetworkManager only for recovery-duration
events, while core uses later provider samples under the existing role
coordinator and does not claim sub-sample event ordering it cannot observe.
