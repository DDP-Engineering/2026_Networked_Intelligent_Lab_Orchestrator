<!--
Copyright (c) 2026 DDP Engineering
This software is licensed under the PolyForm Noncommercial License 1.0.0.
Commercial use requires a separate commercial license.
See the LICENSE.txt in the .agentic folder for full license details.
SPDX-License-Identifier: LicenseRef-PolyForm-Noncommercial-1.0.0
-->


# TASK-bd56e123 - Complete commissioning and fix the management allowlist

stage: done
type: feature
path: full
interaction: autopilot
requirements: REQ-FUNC-b49d7866, REQ-REL-fa990acb
parent:
branch: ddp/nilo-first-boot-provisioning
base: 5ef872e1bbab6df6264b5f1cb1ec007f96c4eb77
opened: 2026-09-12

## Done checklist (current stage)

- [x] validated candidate confirmed unchanged; otherwise returned to validate
- [x] validation evidence, diff, exclusions, and outcome presented
- [x] delivery confirmed by the human
- [x] journal and report included in the delivery commit
- [x] agreed outcome delivered, normally a PR
- [x] close-out committed after the outcome succeeded, or carried by the
      delivery commit where nothing could fail after it

## Frame

Problem: a provisioning appliance follows the hardware forever. There is no way
for an administrator to say which interfaces should carry management access from
now on, and no way for the appliance to stop widening itself. The lifecycle state
delivered earlier has no writer.

Desired outcome: an administrator fixes the allowlist to the interfaces they
chose, the appliance records itself commissioned and stops deriving, and a
transition that does not finish leaves the appliance exactly where it was.

Actors: an administrator with the network-manage permission; the management
service (applies and records); the durable lifecycle record.

Primary scenario: the administrator posts the final interface list -> the
service checks it can still be reached, applies it to the firewall and the
socket gate, persists it, records the appliance commissioned, and stops
deriving -> a later adapter never joins.

Examples: a Mini PC provisioning on Ethernet, Wi-Fi and a USB adapter is
commissioned onto built-in Ethernet alone, freeing the other two for device
networks.

Counterexamples: an empty selection, an absent interface, a container interface
and the loopback device are each refused; dropping the interface the request
arrived on is refused until confirmed; a second completion is refused.

In scope: the transition on the management service, the durable writer and its
adapter, the administrator-only HTTP route, and the failure path that restores
the previous allowlist.

Out of scope: assigning the freed adapters to device networks, which is the
existing device-network workflow and is already prevented from taking a
management adapter by the role coordinator delivered with issue #5; every
browser interface change.

Constraints: the appliance must never be commissioned into unreachability, and a
failed transition must not leave it narrowed but still provisioning-shaped. The
three enforcement layers and the role coordinator are unchanged.

Success signals: after commissioning the allowlist is exactly what was chosen
and stops following the hardware; every refusal leaves the derived allowlist and
the firewall untouched; a failed recording restores what was serving.

Evidence:
- OBSERVED `internal/network/management_access.go` - the provisioning mode
  delivered by TASK-20c64f67 derives the allowlist and refuses `Update`; there
  was no way out of it.
- OBSERVED `internal/appliance/commissioning.go` - TASK-ac1e9003 deliberately
  shipped a reader and no writer, because nothing completed commissioning yet.
- OBSERVED `internal/httpapi/server.go:307-311` - management routes declare
  `auth.PermissionNetworkManage` in the one permission table; the new route
  joins them.
- OBSERVED `internal/network/adapter_roles.go` - the role coordinator already
  refuses to give a management adapter to a device network, so this task does
  not re-implement that guard.
- STATED (user, issue #6) - "Provide an administrator-only workflow for assigning
  permanent interface roles", "Do not allow commissioning to complete without a
  usable management interface", "If the transition would lose management access
  or verification fails, automatically restore the last known-good
  configuration", "After commissioning, freeze the explicit management
  allowlist"; acceptance tests 9, 10, 11 and 14.

Assumptions: an administrator exists to make the request, per RULING-002 on
TASK-2873801a.

Unknowns: none blocking.

Decision-blocking questions: none.

Proposed behaviors:
1. Completing commissioning fixes the allowlist to the chosen interfaces,
   records the appliance commissioned, and refuses a selection that is empty,
   absent, ineligible, or gives up the caller's own path unconfirmed; a second
   completion is refused.
2. A transition that does not finish restores the allowlist that was serving and
   leaves the appliance provisioning.

Path and reason: full. A new administrator-facing route, a new durable writer,
and a security boundary that changes permanently.

## Requirement coverage

| Frame outcome | Atomic behavior | Requirement | Positive case | Boundary or failure | Verification | Design decision or no impact |
|---|---|---|---|---|---|---|
| An administrator fixes the permanent allowlist | Commissioning applies the chosen interfaces, records the state and stops deriving | REQ-FUNC-b49d7866 | One of two derived interfaces is chosen; the allowlist, firewall and admitted destinations carry only it; a later adapter never joins | Empty, absent, ineligible and loopback selections are refused intact; a second completion is refused; the caller's own path needs confirmation to give up | `commissioning_test.go`, `httpapi/commissioning_test.go` | The lifecycle state is recorded last, because it is the only step whose failure can be undone completely |
| A failed transition costs nothing | The previous allowlist is restored and the appliance stays provisioning | REQ-REL-fa990acb | A refused recorder leaves the derived allowlist, the firewall and the admitted destinations as they were | The appliance still follows the hardware afterwards, which is only true while provisioning | `commissioning_test.go` | `restoreProvisioning` reapplies the previous record through the same firewall and store |


## Design

**Requirements coverage.** REQ-FUNC-b49d7866 is carried by
`ManagementAccessService.Commission`, the durable writer and the HTTP route;
REQ-REL-fa990acb by `restoreProvisioning` and the ordering of the transition.

**Component responsibilities.** *Bench Core foundation*. The management service
decides whether the appliance can safely be commissioned and owns the allowlist
change; the `appliance` package owns the durable lifecycle record; the HTTP
layer is a thin adapter that maps refusals onto the existing refusal shapes.

**Interfaces and contracts.** `Commission` and `CommissionConfirmed`, the second
being the first with the caller's own arrival interface deliberately given up -
the same shape `Update` already uses for that decision.
`network.Commissioner` is the narrow interface the service calls;
`appliance.DurableCommissioner` adapts the SQLite writer to it and returns nil
when there is nothing to record through, so a service can never commission in
memory only. `POST /api/v1/commissioning/complete`, administrator-only through
the one permission table.

**Execution flow and ordering.** Refuse if already commissioned or if there is
nowhere to record. Refresh, then check the selection: it must validate, every
interface must be one that may carry management access, at least one must be
usable now, and the caller's own arrival interface may not be dropped
unconfirmed. Then, under the role coordinator, apply the firewall, persist the
record, and stop deriving. Record the lifecycle state **last**, because it is
the only step whose failure can be undone completely: the applied allowlist is
still a valid provisioning allowlist, so restoring the previous one returns the
appliance exactly to where it was.

**Failure recovery.** Any failure calls `restoreProvisioning`, which reapplies
the previous configuration to the firewall, persists it and rebuilds the socket
allowlist, and the appliance remains provisioning - proven by the test asserting
it still picks up an adapter attached afterwards.

**Security and trust boundaries.** The route declares
`auth.PermissionNetworkManage` beside the other management routes and inherits
session, CSRF and same-origin handling. A selection the configuration rejects is
reported as a conflict rather than a service failure, so an administrator's
mistake is not dressed up as an outage. Device-facing reassignment is not
implemented here: the role coordinator already refuses to give a management
adapter to a device network, and freeing an adapter is exactly what
commissioning onto a smaller allowlist does.

**Alternatives rejected.** Recording the lifecycle state first - a failure after
it would leave an appliance recorded as commissioned but serving a derived
allowlist, with nothing to narrow it. Reusing `Update` plus a separate confirm -
`Update` is refused while provisioning by design, and a two-step pending window
would leave the appliance half-transitioned if the administrator never returned.

**Expected files.** `internal/network/commissioning.go` (new),
`commissioning_test.go` (new), `management_access.go`,
`internal/appliance/commissioner.go` (new), `commissioning.go`,
`internal/store/sqlite/commissioning_repository.go`,
`internal/httpapi/commissioning.go` (new), `commissioning_test.go` (new),
`server.go`, `internal/app/app.go`, `cmd/nilo/main.go`.

**Open decisions.** None.

## Log

- 2026-09-12 frame: scope drafted for completing commissioning (issue #6, work
  package 6 of the agreed plan), autopilot under the user's standing instruction.
  Full path: a new administrator route, a new durable writer and a permanent
  security boundary change.
- 2026-09-12 specify: 2 requirements created, 0 revised, 0 reused, 0 retired.
  Split into the transition and the failure path, which fail independently.
- 2026-09-12 design: note recorded in `## Design`; no spec is owed. The lifecycle
  state is recorded last so that the only step that can fail after the allowlist
  changes is one whose failure is completely undoable.
- 2026-09-12 build red: REQ-FUNC-b49d7866, REQ-REL-fa990acb;
  command=`go test ./internal/network/ -run Commissioning -count=1`; exit=1;
  expected missing behavior=`ManagementAccessOptions.Commissioner` and
  `ManagementAccessService.Commission` do not exist, so the package does not
  compile.
- 2026-09-12 build green: REQ-FUNC-b49d7866, REQ-REL-fa990acb;
  command=`go test ./internal/httpapi/ ./internal/network/ -run 'CommissioningAPI|Commissioning' -count=1`;
  exit=0. Whole Go suite `go test ./... -count=1` exit=0, `go vet ./...` exit=0
  and `GOOS=linux go vet ./...` exit=0.
- 2026-09-12 build: the API test found a real defect rather than confirming the
  code. Commissioning onto the loopback device answered 503, because the
  configuration validation failure fell through to the handler's service-failure
  branch, reporting an administrator's mistake as an outage. `commission` now
  wraps a validation failure in `ErrManagementAccessUnsafe`, so every refused
  selection answers as a conflict. The test was not relaxed to match the code.

- 2026-09-12 validate: review round 1, both verdicts approved on the current
  candidate. Specification compliance: the eleven files are those the design
  named, the three enforcement layers and the role coordinator are untouched,
  and device-facing reassignment was deliberately not re-implemented because the
  coordinator delivered with issue #5 already refuses an adapter still in the
  management allowlist. Implementation quality: the transition records the
  lifecycle state last so the only step that can fail after the allowlist
  changes is one whose failure is completely undoable, and the restore path is
  asserted by behaviour - the appliance still picks up an adapter attached after
  a failed transition, which is only true while it really is still provisioning.
- 2026-09-12 validate: knowledge reconciled. `.agentic/knowledge/bench-core-foundation.md`
  gains the commissioning invariant, the three new file seams, the two
  requirement IDs and a current-state section.
- 2026-09-12 validate: final gate run.
  command=`.\02_Tools\env\with-tools.ps1 python .agentic/aegis.py check --task TASK-bd56e123 --strict --run`;
  exit=1; errors=2 (`run: test` exit 1 after 79.3s, `run: lint` exit 1 after
  4.3s). Both are the base-branch failures accepted under TASK-2873801a
  RULING-003, unchanged and with nothing new joining them: the build-environment
  reconstruction manifest test (92 script tests ran, the same 1 failed) and
  `prettier --check` on the same four GUI files.
- 2026-09-12 validate: verification completed on the final candidate.
  `go test ./... -count=1` exit=0 over every package in
  `05_Software/nilo_assistant`, `go vet ./...` exit=0 and
  `GOOS=linux go vet ./...` exit=0. This task changes no GUI, Python or
  documentation file, so the suites the script-test failure short-circuits carry
  no evidence it could affect; they were last proven green on this branch under
  TASK-20c64f67.
- 2026-09-12 validate: DEVIATION validate: the configured gate exits non-zero for
  the base-branch failures already accepted for this branch, so this candidate
  advances on per-suite evidence rather than one green gate command. No new
  failure was introduced.

- 2026-09-13 deliver: delivered. The human committed the whole issue #6
  candidate as `47f4ad6` "Feature: First-boot management provisioning on
  eligible interfaces" on `ddp/nilo-first-boot-provisioning`, together with the
  other work packages and the 0.3.0 version bump. This task contributed completing commissioning and freezing the allowlist.
  A per-package split was no longer clean by then - five files had been edited by
  more than one package, so separate commits would not have built individually -
  and the human chose one commit over hunk-level staging. Per-package
  traceability remains in these journals and their requirements. Repository Git
  authority reserves committing to the human; the agent neither staged,
  committed nor pushed. No push, PR or release was part of this outcome.
- 2026-09-13 deliver: hardware evidence recorded for the feature as a whole on
  the Raspberry Pi `ddp-remote` before the commit. A fresh data directory started
  with no listener option, derived `eth0`/`eth1`/`wlan0`, excluded the WireGuard
  device, recorded `provisioning`, bound to the documented stable adapter
  identities, published `ddp-remote.local` through avahi, and resumed the same
  allowlist after a restart. Upgrading the real installation recorded
  `commissioned` and left its identities and `{nilo0, wlan0}` allowlist
  untouched. The board now runs 0.3.0. Detail is in
  `06_SoftwareDocu/Local_Hardware_Access.md` section 2.8; this is early bench
  evidence from a personal board, not a reproducible acceptance gate.
- 2026-09-13 done: task closed. This close-out line is journal-only and rides
  with the next commit the human makes.

## Rulings


## Handoff


current: done
status: ready
next: none - issue #6 is delivered in commit 47f4ad6 and verified on ddp-remote
must_read: .agentic/tasks/TASK-bd56e123.md
blockers: none

## Stage report

stage: deliver
result: completed
current: done
changed: delivered in commit 47f4ad6 together with the rest of issue #6 and the 0.3.0 version bump; this task contributed completing commissioning and freezing the allowlist
evidence: automated suites green before delivery, recorded in this journal's validate entries; hardware observations on ddp-remote recorded in the deliver entry above and in 06_SoftwareDocu/Local_Hardware_Access.md section 2.8
issues: the base-branch failures accepted under TASK-2873801a RULING-003 remain open on the branch and still fail CI, as they do on master
next: none
needs_user: no
spec_compliance: approved
implementation_quality: approved
review_round: 1

