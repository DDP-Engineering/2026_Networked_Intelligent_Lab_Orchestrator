<!--
Copyright (c) 2026 DDP Engineering
This software is licensed under the PolyForm Noncommercial License 1.0.0.
Commercial use requires a separate commercial license.
See the LICENSE.txt in the .agentic folder for full license details.
SPDX-License-Identifier: LicenseRef-PolyForm-Noncommercial-1.0.0
-->


# TASK-99ea6c3b - Sign in to a new NILO without a console step

stage: build
type: feature
path: full
interaction: checkpoint
requirements: REQ-SEC-c9e034de, REQ-PLAT-3c35bacd, REQ-SEC-d0d3d4c4
parent:
branch: ddp/nilo-first-boot-signin
base: a9573fc18f3b83fdc68c8126208e0434cb3a226d
opened: 2026-09-13

## Done checklist (current stage)

- [ ] each automated behavior increment has a pre-implementation red record
      with requirement IDs, exact selector or command, non-zero exit, and
      expected failure reason, plus one final focused green result; each manual
      increment has a consolidated post-implementation observation
- [ ] repository anchor recorded after red and evolution reviewed after
      implementation
- [ ] implementation makes those tests pass
- [ ] focused project tests green, with their actual scope recorded
- [ ] `affected_paths` and `linked_tests` updated on every requirement
- [ ] `status = "active"` on every requirement this task delivers
- [ ] `check` clean, or each remaining warning explained in the log
- [ ] `stage: validate`

## Frame

Problem: A freshly imaged appliance holds no identity, so the
administrator-only commissioning workflow cannot be started at all until
somebody creates one at a console, and it holds no certificate identity for the
address a user is told to visit, so the browser objects before anything else
happens. The appliance is reachable and still cannot be set up.

Desired outcome: First contact is plug in, open the address, sign in, finish
setup. A published first credential makes that possible, and cannot survive
commissioning, so the widest exposure the appliance ever has ends at the same
moment the weakest credential does.

Actors: the person commissioning a newly imaged appliance, who becomes its
Administrator; the appliance image build, which places the seeded identity and
the shipped certificate (its mechanics are out of scope); Bench Core, which
authenticates, holds the password rule and decides whether commissioning may
complete; the browser, which judges the shipped certificate.

Primary scenario: a user opens the first-boot address of a newly imaged
appliance -> the browser shows a certificate warning naming an untrusted signer
rather than a wrong name, the user proceeds, signs in as the published
administrator, is refused commissioning until the published password is
replaced, replaces it under the ordinary rule, and commissions -> the appliance
is commissioned and the published password no longer opens it.

Examples:
- Sign-in with the published user name and password on a freshly imaged
  appliance succeeds with no console step.
- Completing commissioning while the seeded identity still holds the published
  password is refused, and the refusal names the password change as what must
  happen first.
- The shipped certificate carries both `<system-hostname>.local` and
  `nilo.local`, so the only browser objection is to the signer.

Counterexamples:
- Setting a password shorter than the minimum after imaging is refused,
  including a new password for the seeded identity itself.
- An appliance whose administrator was created at a console, never seeded, is
  not blocked from commissioning by this rule.
- A management name absent from the certificate is still warned about and the
  appliance still starts, unchanged (OBSERVED internal/httpapi/serving.go:61,
  cmd/nilo/main.go:480-482).
- A required discovery name absent from the certificate is refused at start with
  exit 2, not warned about. That is the behavior a9573fc introduced, and this
  task keeps it (OBSERVED cmd/nilo/main.go:411-414, 476-479).

In scope:
- One seeded administrator identity holding the published password, created at
  image build.
- Accepting that password at sign-in.
- Holding every password set after imaging to the ordinary rule.
- Refusing to complete commissioning while the seeded identity still holds the
  published password, with a message naming what must change.
- Confirming a shipped certificate that carries the two first-contact names
  a9573fc requires, `<system-hostname>.local` and `nilo.local`, and keeping both
  of today's reporting behaviors unchanged: exit 2 for a missing discovery name,
  a warning for a missing management name.
- Stating the published credential, the shared certificate key, the expected
  certificate warning, the bounded exposure of each and when each stops
  mattering, in the deployment and security documentation.

Out of scope (STATED):
- Any exception to the password rule. The published password is
  `admin_password` (RULING-001), which already satisfies the compiled-in
  minimum, so no password-rule change is needed anywhere.
- Building, burning or distributing the appliance image, including how the
  identity and the certificate are placed into it.
- Discovery: the names themselves, their derivation and their advertisement
  belong to issue #12, whose work landed as a9573fc. This task consumes
  `requiredDiscoveryTLSNames` and changes nothing about it.
- Changing authentication, authorization, session or CSRF handling.
- Per-appliance credentials.
- A configurable password policy: an administrator-settable minimum length and
  character-class requirement. Requested mid-frame and moved to its own issue
  #16 (RULING-002). This task keeps the compiled-in rule exactly as it is.

Constraints:
- The password rule is not weakened anywhere. Issue #13 asks for an exception
  confined to the seeded identity because weakening the rule generally would
  weaken every password set later; RULING-001 removes the need for any
  exception, so the constraint is met by leaving the rule alone (STATED).
- No publicly trusted certificate can exist for a reserved `.local` name or a
  private address, so first contact shows a certificate warning, which is
  documented as the expected first step rather than engineered away (STATED).
- The shipped certificate's private key is in every image and is therefore not
  secret; its exposure is bounded to appliances that have not been
  commissioned, on the user's network segment (STATED).
- The shipped certificate stays for the appliance's working life; commissioning
  does not replace it, because a replacement would raise a second warning just
  as setup appears to finish (STATED).
- NILO reads the certificate it is given, checks that the names it advertises
  are present, warns when they are not, and never generates or replaces one
  (STATED, and OBSERVED internal/httpapi/serving.go:59-60).
- No response, log, audit event, export or error message carries a password or
  verifier (OBSERVED .agentic/knowledge/bench-core-foundation.md invariants).
- Commissioning is one way and completes only onto an allowlist the appliance
  can still be reached through (OBSERVED internal/network/commissioning.go).
- `identity.Service` decides whether an actor may request an action;
  `policy.Resolver` decides whether an operation may run. The new refusal is
  neither, and must not blur them (OBSERVED .agentic/context/project.md).

Success signals: the nine acceptance items of issue #13, in order: sign-in with
no console step; the seeded password accepted; every later password held to the
rule; commissioning refused while the published password stands, with a naming
message; commissioning succeeds after the change; a console-created
administrator is not blocked; first contact over HTTPS with both first-contact
names in the certificate; an advertised identity absent from the certificate
still reported, by the route a9573fc gives it; and the documented exposure.

Acceptance item 7 is read as the two names a9573fc requires, with no address:
issue #12's landed direction provides none, and item 7's "fixed address" has no
referent. Acceptance item 8's "as it does today" is read as today after a9573fc,
which means exit 2 for a discovery name and a warning for a management name.

Evidence:
- STATED: the whole of issue #13 - problem, required behavior, the nine
  acceptance items, the four exclusions.
- STATED mid-frame by the user: the published password is `admin_password`,
  not `admin` (RULING-001), and a request for a configurable minimum password
  length and character-class setting.
- OBSERVED `internal/auth/password.go:19` - `MinPasswordBytes = 12`,
  `MaxPasswordBytes = 1024`, both compiled-in constants with no configuration.
- OBSERVED `internal/auth/password.go:53,79,89` - `validatePassword` is called
  by both `HashPassword` and `VerifyPassword`, so a password below the minimum
  is refused when it is checked, not only when it is set.
- OBSERVED `internal/httpapi/administration.go:394,444` - the HTTP layer
  repeats the same bounds before the slow hash runs.
- OBSERVED `internal/identity/service.go:225` - `Bootstrap` creates the first
  administrator through `prepareUser`, which hashes under the ordinary rule;
  the repository holds no seeding concept today.
- OBSERVED `internal/network/commissioning.go:49-108` - `commission` refuses on
  lifecycle state, a missing commissioner, an unsafe or ineligible selection
  and an unconfirmed current interface. It has no identity dependency.
- OBSERVED `internal/httpapi/commissioning.go` - the route maps each network
  sentinel to a refusal code; `already_commissioned`,
  `management_access_required` and `current_interface_confirmation_required`
  are conflicts, `commissioning_unavailable` is a 503.
- OBSERVED `cmd/nilo/discovery_runtime.go:13-19` - `requiredDiscoveryTLSNames`
  returns `<system-hostname>.local`, plus `nilo.local` when the recovery alias
  is included. These are DNS names; no address appears anywhere in them.
- OBSERVED `cmd/nilo/main.go:405-414` - the provisioning listener calls
  `requiredDiscoveryTLSNames(systemHostname, true)`, so first boot requires both
  names, and exits 2 naming the first missing one rather than warning.
- OBSERVED `cmd/nilo/main.go:470-482` - the commissioned listener exits 2 for a
  missing discovery name and warns for a missing management name, so the two
  reporting behaviors now coexist.
- OBSERVED `internal/network/management_access.go:536` - `ProvisioningPort` is
  8443.
- OBSERVED issue #12 - discovery uses standards-based IPv4 link-local
  addressing in 169.254/16 plus mDNS/DNS-SD, explicitly adds no fixed recovery
  subnet, and its first acceptance item names `https://nilo.local:<port>/`
  with no address at all. Its work landed as a9573fc, which this branch merged.
- OBSERVED `git diff bdcd757 a9573fc` - the merge left `internal/auth/`,
  `internal/identity/`, `internal/network/commissioning.go` and
  `internal/httpapi/commissioning.go` untouched, so every seam this task works
  on is exactly as framed.
- OBSERVED `internal/store/sqlite/commissioning_repository.go:116-126` - a
  commissioned appliance can be reset back to `provisioning`, and that reset
  moves the lifecycle row and the management access record only. Identities are
  untouched.
- INFERRED: `admin_password` is 14 bytes and so already satisfies the
  compiled-in 12-byte minimum. Nothing in `internal/auth/password.go` or
  `internal/httpapi/administration.go` needs to change, acceptance items 2 and
  3 are met by the existing rule, and the shared `validatePassword` on the
  verification path stops being an obstacle.
- INFERRED: the commissioning check can either compare the published password
  against the seeded identity's stored verifier or read a persisted seeded
  marker cleared on password change. Both satisfy acceptance item 6, because
  either is scoped to the published user name and an appliance whose
  administrator was created at a console has no identity by that name. The
  marker is the more precise of the two; the comparison needs no schema change.
- INFERRED: an appliance reset to provisioning after commissioning carries its
  existing administrator, whose password was necessarily changed to commission
  in the first place, so P4's gate finds nothing to refuse and the reset
  appliance recommissions normally. No extra work, but it is the reason P4 must
  test the credential rather than the lifecycle state.
- UNKNOWN: how an appliance image is expected to invoke the seeding surface,
  given that placing the identity into the image is out of scope.
- UNKNOWN: whether issue #12's commissioning reset will later clear identities.
  Issue #12 states the data it clears "must be defined before implementation".
  If a future reset reseeds the published identity, P4's gate has to fire again
  for it, which the credential test gives for free and a one-shot marker would
  not.

Assumptions:
- The published pair is the same for every image; per-appliance credentials are
  excluded (STATED).
- The seeded identity is an ordinary Administrator in every other respect, so
  role, audit, session, throttle and token behavior are unchanged.
- The existing missing-identity warning and the refusal to generate or replace
  a certificate stay exactly as they are; this task adds no certificate
  generation.
- Documentation lands in `06_SoftwareDocu/Network_Operations.md` and
  `07_Delivery/`, where first boot is already described
  (OBSERVED `07_Delivery/NILO_First_Boot_Provisioning_Verification.md` exists).

Unknowns:
- Whether the seeded password check at commissioning verifies the published
  password against the stored verifier, or reads a persisted marker cleared on
  password change. The verifier comparison needs no schema change and keeps
  working if a future reset reseeds the identity; the marker is more precise
  about intent. Design decides.
- Which surface carries the image-build seeding - a flag on an existing local
  `nilo admin user create`, a dedicated command, or a build-time entry point.
  Design decides.
- Whether the commissioning refusal is a new sentinel in `internal/network`
  supplied by the composition root, or a check in
  `internal/httpapi/commissioning.go` before the network call. Design decides.

Decision-blocking questions: none outstanding. All four are answered.

1. RESOLVED by merged commit a9573fc, as option (a). The certificate carries DNS
   names only and no address: `requiredDiscoveryTLSNames` returns names alone
   (OBSERVED cmd/nilo/discovery_runtime.go:13-19), matching issue #12's
   link-local direction, which provides no fixed address to carry. Acceptance
   item 7's "fixed address" has no referent and is read as the name pair.
2. RESOLVED by merged commit a9573fc. First boot requires **both**
   `<system-hostname>.local` and `nilo.local`
   (OBSERVED cmd/nilo/main.go:405). The name question was #12's to settle and it
   settled it; this task changes no name and only confirms the shipped
   certificate carries the pair.
3. RESOLVED by RULING-001. The published pair is user name `admin` and password
   `admin_password`, superseding issue #13's `admin`/`admin`. Issue #13's
   "A known first credential" paragraph, which describes a password shorter
   than the minimum and an explicit confined exception, no longer applies and
   needs amending on the ticket.
4. RESOLVED by RULING-002. The configurable password policy is issue #16,
   "Configure the appliance password policy", and is out of scope here.

Proposed behaviors:
- P1 Offer one explicit, confined way for an image build to create a single
  seeded administrator holding the published password, refused once any
  identity exists. Serves acceptance 1.
- P2 Accept the published password at sign-in through the existing login path
  unchanged, because `admin_password` already satisfies the password rule.
  Serves acceptance 1, 2; no code change expected beyond a test that holds it.
- P3 Hold every password set after imaging to the ordinary rule, including a
  replacement for the seeded identity, on both the command line and the HTTP
  administration routes. Serves acceptance 3.
- P4 Refuse to complete commissioning while the seeded identity still holds the
  published password, as a conflict whose message names the password change;
  let a changed password, or an administrator that was never seeded, commission
  normally. Serves acceptance 4, 5, 6.
- P5 Confirm the shipped certificate carries both `<system-hostname>.local` and
  `nilo.local`, so first contact over HTTPS leaves the signer as the browser's
  only objection, and leave a9573fc's two reporting behaviors and the refusal to
  generate or replace a certificate untouched. Serves acceptance 7, 8. Expected
  to be verification and documentation rather than new logic: a9573fc already
  requires both names and already reports a missing one.
- P6 State the published credential, the shared certificate key, the expected
  certificate warning, the bounded exposure of each and when each stops
  mattering, in the deployment and security documentation. Serves acceptance 9.

Path and reason: full.

| criterion | yes | no | evidence |
|---|---|---|---|
| no new externally observable behavior | | no | a published sign-in, a new commissioning refusal and a new refusal message are all observable |
| no new interface or data format | | no | a new refusal code on the commissioning route, and a new image-build surface for the seeded identity |
| no new dependency | yes | | everything named sits inside the existing Go module |
| an existing requirement already covers the behavior | | no | no active requirement covers a seeded credential or a credential-bound commissioning refusal |
| no security- or data-handling behavior changes | | no | a published credential ships in the image and a new credential-bound gate decides commissioning |

Four criteria fail, so the work takes the full path. It is not large enough for
child tasks: the seeded credential, the commissioning refusal and the
certificate identities are one tightly coupled first-boot story sharing the
same acceptance.

## Behavior inventory

| Frame outcome | Actor/system | Trigger or state | Response | Positive case | Boundary or failure | Observable result |
|---|---|---|---|---|---|---|
| P1 | image build, running `nilo admin bootstrap` | a staged data directory holding no identity | creates one administrator under the published user name with the published password read from standard input | the staged image yields the published administrator | a second run is refused because an enabled administrator exists | the imaged appliance signs in with no console step |
| P2 | Bench Core login | a login request presenting the published pair on a freshly imaged appliance | establishes an expiring browser session | the published password signs in | a password above the maximum is refused | a session cookie the browser can use |
| P3 | Bench Core identity administration | any password set after imaging, the published identity included | applies the ordinary password rule | a compliant replacement is accepted | a replacement below the minimum is refused with the bounds named | the stored verifier changes only for a compliant password |
| P4 | the appliance completing commissioning | provisioning, and the published identity still accepts the published password | refuses, as a conflict naming the password change | a changed password commissions on the identical request | an appliance with no identity under the published user name is unaffected | allowlist and lifecycle state unchanged; the appliance is still provisioning |
| P5 | the appliance starting first boot | first-boot provisioning start with the given certificate | serves over HTTPS when both first-contact names are present, otherwise refuses to start naming a missing one | both names present, the signer is the browser's only objection | either name missing refuses the start | a reachable HTTPS appliance, or a non-zero exit naming the gap |
| P6 | the deployment and security documentation | a reader preparing or receiving an appliance | states the published credential, the shared certificate key, the expected warning, the bounded exposure of each, and when each ends | the reader meets the warning as a step in the procedure | not applicable | documentation a reviewer can check against the five facts |

## Requirement coverage

| Frame outcome | Atomic behavior | Requirement | Positive case | Boundary or failure | Verification | Design decision or no impact |
|---|---|---|---|---|---|---|
| P1 | one administrator is created under the published user name into an empty data directory | REQ-FUNC-00305f82 (reused) | the staged image yields the published administrator | a second bootstrap is refused because an administrator exists | existing command and service tests | no impact: `nilo admin bootstrap --data-dir <path> --username admin` with the password on standard input already does exactly this, and placing it into the image is out of scope |
| P2 | the published password establishes a browser session | REQ-FUNC-db6fb2fa (reused) | the published pair signs in | a password above the maximum is refused | existing login tests, plus one build-stage test holding the published pair | no impact: `admin_password` is 14 bytes and already satisfies the rule |
| P3 | every password set after imaging meets the ordinary rule | REQ-SEC-0ecda0e7 (reused) | a compliant replacement is accepted | a replacement below the minimum is refused | existing bounds tests, plus one build-stage test for the published identity's own replacement | no impact: the published identity is not special, so the existing rule already covers it |
| P4 | commissioning is refused while the published identity still accepts the published password | REQ-SEC-c9e034de (new) | a changed password commissions | published password refused as a conflict; an appliance with no identity under the published user name is unaffected | failing test at internal/network/commissioning_test.go with a fake PublishedCredential; API test at internal/httpapi/commissioning_test.go; adapter test at internal/appliance; HoldsPassword test at internal/identity/service_test.go | gate is a precondition inside `commission`, behind a `PublishedCredential` interface declared in internal/network and satisfied by an internal/appliance adapter over identity.Service.HoldsPassword; fails closed when absent |
| P5 | first boot serves only with both first-contact names in its certificate | REQ-PLAT-3c35bacd (new) | both names present, signer is the only objection | either name missing refuses the start | cmd/nilo/discovery_runtime_test.go, plus one inspection of a running appliance | extract the inline decision at cmd/nilo/main.go:405-414 into `checkDiscoveryCertificate`, mirroring `checkProvisioningPermitted`; behavior unchanged |
| P6 | the published credential and shipped key exposure are documented | REQ-SEC-d0d3d4c4 (new) | the five facts are present and the warning sits beside the address | not applicable | inspection | no design impact beyond naming the two documents and the compiled-in published pair they quote |

## Design

**Requirements coverage.** REQ-SEC-c9e034de drives the commissioning gate and
everything under it. REQ-PLAT-3c35bacd drives one extraction so the existing
certificate-name decision becomes testable; it adds no behavior.
REQ-SEC-d0d3d4c4 is documentation and has no design impact beyond naming the
two files and the compiled-in pair they must quote.

**Component responsibilities.** All of this is Bench Core foundation.
`internal/network` continues to own whether commissioning may complete;
`internal/identity` continues to own what a stored verifier accepts; neither
learns about the other. They are joined by a narrow interface declared in
`internal/network` and satisfied by an adapter in `internal/appliance`, exactly
as `Commissioner` already is (OBSERVED internal/network/commissioning.go:24).
`cmd/nilo` remains the composition root that wires them.

**Interfaces and contracts.** `internal/network` gains, beside `Commissioner`:

```go
// PublishedCredential reports whether the credential published with the
// appliance image still opens this appliance.
type PublishedCredential interface {
    StillOpensAppliance(context.Context) (bool, error)
}
```

It answers a boolean, never the credential, so no implementation of it can
become a way to read the published password out of a running appliance
(REQ-SEC-c9e034de, and the Bench Core invariant that no response, log, audit
event, export or error message carries a password or verifier).

`internal/identity` gains one narrow query:

```go
// HoldsPassword reports whether the named identity's stored verifier accepts
// password. It establishes no session, records no audit event and takes no
// part in login throttling, because it answers an administrator's own
// commissioning request rather than an authentication attempt.
func (service *Service) HoldsPassword(ctx context.Context, username string, password []byte) (bool, error)
```

`internal/appliance` gains the published pair as compiled-in constants and the
adapter that joins the two. The password is a published fact, not a secret, so
compiling it in leaks nothing that the documentation does not already state
(REQ-SEC-d0d3d4c4).

**Execution flow.** In `commission`, the gate is a precondition, checked
immediately after the existing provisioning and commissioner checks and before
`refresh`, because it is a property of the appliance rather than of the
selection, and because refusing there mutates nothing. It costs one Argon2id
derivation per completion attempt, on an administrator-initiated action that
happens once in an appliance's life.

**Failure recovery.** A `PublishedCredential` that is absent or that fails
refuses commissioning rather than allowing it, mirroring
`ErrCommissioningUnavailable`: a gate that cannot answer must not silently pass
(REQ-SEC-c9e034de). Two sentinels, so the interface can tell a refusal from an
outage:

- `ErrPublishedCredentialUnchanged` - the published password still opens the
  appliance. `internal/httpapi/commissioning.go` answers `409` with reason
  `published_password_unchanged`, beside the existing conflict reasons.
- `ErrPublishedCredentialUnknown` - no checker, or the check failed.
  Answered `503` with reason `commissioning_unavailable`, as today.

Nothing is applied before the gate, so a refusal leaves the allowlist, the
firewall and the lifecycle state untouched with no restore path needed - unlike
the later failures that `restoreProvisioning` exists for.

**Data ownership and state transitions.** None. No schema change, no migration,
no new persisted state. The gate reads the identity table through
`identity.Service` and decides from what is already there.

**Security and trust boundaries.** `HoldsPassword` is a password oracle by
construction, so it stays unexported from HTTP: no route calls it, only the
commissioning path does, and that path is already administrator-only and
CSRF-protected (OBSERVED internal/httpapi/server.go:312). It is reachable only
by a caller who already holds the rights the published credential would grant,
so it discloses nothing that caller could not obtain by signing in.

**Test seams.** REQ-SEC-c9e034de attaches its failing test at
`internal/network/commissioning_test.go` with a fake `PublishedCredential`, and
its API test at `internal/httpapi/commissioning_test.go`; the adapter's own
test sits at `internal/appliance`, and `HoldsPassword` at
`internal/identity/service_test.go`. REQ-PLAT-3c35bacd attaches at
`cmd/nilo/discovery_runtime_test.go`.

**REQ-PLAT-3c35bacd shape.** The certificate-name decision is inline in
`runServe` today (OBSERVED cmd/nilo/main.go:405-414), where no test can reach
it. Extract it beside the names it checks:

```go
func checkDiscoveryCertificate(certificatePath, systemHostname string, includeRecoveryAlias bool) error
```

This mirrors `checkProvisioningPermitted` in `listener_mode.go`, which exists
for exactly this reason and carries REQ-PLAT-b1abf074's linked test. Behavior
is unchanged: the same names, the same refusal, the same exit code.

**Compatibility and migration.** One deliberate break, inside the repository
only: `network.NewManagementAccessService` gains a required option for
provisioning services, so existing constructions in tests must supply a
checker. Nothing persisted, published or externally observable changes shape.

**Observability and operations.** The refusal message names the identity and
the action needed, never the password. No new audit event: a refused
commissioning is not an identity mutation, and the existing refusals in this
path record none either.

**Concurrency and ordering.** The gate runs inside the existing `changeMu`
critical section, so it cannot interleave with another completion attempt.

### Proposed architecture - TASK-99ea6c3b

```text
cmd/nilo/main.go  (composition root)
  |
  |-- appliance.NewDurableCommissioner(...) ------------> network.Commissioner
  |-- appliance.NewPublishedCredential(identitySvc) ----> network.PublishedCredential   [new]
  |                                                             |
  |                                                             v
  |                                              identity.Service.HoldsPassword        [new]
  v
network.ManagementAccessService.commission
  provisioning? -> commissioner? -> PublishedCredential.StillOpensAppliance   [new gate]
                                     |            |
                                     |            +-- true  -> ErrPublishedCredentialUnchanged
                                     +-- error/nil -------- -> ErrPublishedCredentialUnknown
  -> refresh -> validate selection -> apply -> CompleteCommissioning
```

**Alternatives and tradeoffs.**

- *A persisted "seeded" marker cleared on password change*, rather than testing
  the credential. Rejected: it needs a migration and a clearing hook on every
  password-setting path, and it answers the wrong question. A marker records
  that an identity was once seeded; the requirement is about whether the
  published password still opens the appliance now. The marker also stops
  firing if a future commissioning reset reseeds the identity, which issue #12
  leaves open.
- *The check in `internal/httpapi/commissioning.go`, before calling the
  service.* Rejected although it would cover every caller today, the HTTP route
  being the only one (OBSERVED: no other non-test caller of `Commission`). A
  security precondition belongs with the operation it protects, so that a
  second caller cannot be added without it.
- *Reusing `identity.Service.Login`* instead of adding `HoldsPassword`.
  Rejected: it creates a session, writes an audit event and consumes the login
  throttle budget, all of which are wrong for an internal precondition check.

**Expected files.**

- `internal/network/commissioning.go`, `commissioning_test.go`
- `internal/network/management_access.go`
- `internal/appliance/published_credential.go`, `published_credential_test.go`
- `internal/identity/service.go`, `service_test.go`
- `internal/httpapi/commissioning.go`, `commissioning_test.go`
- `cmd/nilo/main.go`, `cmd/nilo/discovery_runtime.go`,
  `cmd/nilo/discovery_runtime_test.go`
- `06_SoftwareDocu/Network_Operations.md`,
  `07_Delivery/NILO_First_Boot_Provisioning_Verification.md`

**Risks.**

- Making the checker required refuses commissioning in any existing test that
  builds a provisioning service without one. That is the intended fail-closed
  behavior; the cost is that those constructions must be updated in build.
- Compiling the published password in means changing it later is a rebuild.
  Acceptable: it is one published constant per image, and per-appliance
  credentials are out of scope.

**Open decisions.** None. Both decisions the Frame left to design are taken
above: the credential test rather than a marker, and the gate in
`internal/network` rather than in the HTTP layer.

## Log

- 2026-09-13 frame: framed issue #13 on branch ddp/nilo-first-boot-signin from
  bdcd757. Full path chosen: four of the five quick-path criteria fail, the
  decisive ones being new observable behavior and a security-behavior change.
  No child tasks; the three strands share one acceptance. Two contradictions
  raised against issue #12 - the fixed address of acceptance item 7, which
  #12's link-local direction does not provide, and which first-contact name the
  shipped certificate must carry. User proposed mid-frame publishing
  `admi_password` rather than `admin`, which would satisfy the existing 12-byte
  minimum and remove the password-rule exception from scope, and asked for a
  configurable minimum length and character-class setting, which is new scope
  beyond #13. Scope presented; awaiting acceptance.
- 2026-09-13 frame: published password settled as `admin_password`
  (RULING-001). The password-rule exception leaves scope entirely: no change to
  internal/auth/password.go or internal/httpapi/administration.go is expected,
  and acceptance items 2 and 3 are met by the existing rule. Issue #13's
  "A known first credential" paragraph needs amending on the ticket. Questions
  1, 2 and 4 remain open.
- 2026-09-13 frame: user paused at the frame stage boundary. Scope is not yet
  accepted and the stage is not advanced. Resuming needs answers to question 1
  (the fixed address of acceptance item 7 against issue #12's link-local
  direction), question 2 (nilo.local or <system-hostname>.local in the shipped
  certificate) and question 4 (whether the configurable password policy belongs
  here or in its own ticket).
- 2026-09-14 frame: fast-forwarded ddp/nilo-first-boot-signin from bdcd757 to
  origin/master a9573fc, "feat: improve NILO network discovery and recovery",
  which is issue #12's work landing. No product work existed on this branch, so
  the merge was a fast-forward with no conflict and the header base was moved
  from bdcd757 to a9573fc; leaving it would have made check --task measure
  #12's 105 files as this task's diff. Verified in this worktree after the
  merge: go build ./... exit 0, and scripts.tests.test_build_environment 51
  tests OK, the five failures recorded before the merge having been caused by a
  GUI lock digest that a9573fc regenerated.
  The merge answers two of the three open decisions from repository evidence:
  question 2 is settled - first boot requires two certificate names,
  <system-hostname>.local and nilo.local, from
  requiredDiscoveryTLSNames(systemHostname, true)
  (OBSERVED cmd/nilo/discovery_runtime.go:13-19, main.go:405);
  question 1 is settled as option (a) - the required names are DNS names only,
  no address appears anywhere in the certificate identities, matching issue
  #12's link-local direction. A new fact for P5 and acceptance item 8: a
  provisioning appliance whose certificate lacks a required discovery SAN now
  refuses to start with exit 2 (OBSERVED main.go:411-414, 476-479) rather than
  warning, so "reports that in the interface, as it does today" no longer
  describes the provisioning path. The Frame card still records questions 1 and
  2 as open and needs that short revision on resume; question 4, the
  configurable password policy, remains undecided.
- 2026-09-14 frame: question 4 closed by RULING-002. The configurable password
  policy was raised as issue #16, "Configure the appliance password policy", and
  is out of scope here; this task keeps the compiled-in rule unchanged. All four
  decision-blocking questions are now answered. The Frame card's questions 1 and
  2 still need the short revision against a9573fc, after which the only thing
  outstanding is the user's scope acceptance.
- 2026-09-14 frame: Frame card revised against merged commit a9573fc. Questions
  1 and 2 marked resolved from repository evidence; acceptance item 7 read as
  the name pair `<system-hostname>.local` and `nilo.local` with no address, and
  acceptance item 8 read as a9573fc's two reporting behaviors - exit 2 for a
  missing discovery name, a warning for a missing management name. P5 narrowed
  accordingly to confirmation and documentation rather than new logic. Verified
  that a9573fc left internal/auth/, internal/identity/,
  internal/network/commissioning.go and internal/httpapi/commissioning.go
  untouched, so the remaining seams are as framed. Recorded the commissioning
  reset as an interaction: it returns a commissioned appliance to provisioning
  without touching identities, which is why P4 must test the credential rather
  than the lifecycle state. Scope presented for acceptance; stage not advanced.
- 2026-09-14 specify: scope accepted by the user; stage advanced frame ->
  specify. Created 3, revised 0, reused 3, retired 0. Three of the six framed
  outcomes turned out to need no new requirement: P1 is served by the existing
  `nilo admin bootstrap` command, which already creates one administrator under
  a chosen user name with the password read from standard input and refuses a
  second, so the image build calls it and this task adds no seeding surface;
  P2 and P3 are served by the existing login and password-bounds behavior
  because `admin_password` is an ordinary compliant password. The task's own
  new behavior is the commissioning gate, the first-boot certificate names, and
  the disclosure documentation. Recorded while specifying: no requirement
  covered the certificate-name check a9573fc implemented, so REQ-PLAT-3c35bacd
  closes a coverage gap rather than describing new logic.
- 2026-09-14 design: stage advanced specify -> design; note written in
  `## Design`, no spec needed. Both decisions the Frame left open are taken
  there, and no open decision remains. `affected_paths` on REQ-SEC-c9e034de and
  REQ-PLAT-3c35bacd now match the note's expected file list, and
  REQ-PLAT-3c35bacd's verification was realigned onto the extracted decision
  seam. One deliberate in-repository break is recorded: a provisioning
  management access service now requires a published-credential checker, so
  existing test constructions must supply one.
- 2026-09-14 build red: REQ-SEC-c9e034de; command=`go test ./internal/network/
  -run TestCommissioningIsRefused`; exit=1; expected missing behavior=the gate
  does not exist - `PublishedCredential`, the `ManagementAccessOptions` field,
  `ErrPublishedCredentialUnchanged` and `ErrPublishedCredentialUnknown` are all
  undefined, so the package does not compile.
- 2026-09-14 build green: REQ-SEC-c9e034de gate; command=`go test
  ./internal/network/`; exit=0. The gate is a precondition inside `commission`,
  refusing with `ErrPublishedCredentialUnchanged` while the published credential
  still opens the appliance and with `ErrPublishedCredentialUnknown` when it
  cannot be checked, leaving the allowlist, firewall, persisted record and
  lifecycle state untouched in both cases.
- 2026-09-14 build red: REQ-SEC-c9e034de checker; command=`go test
  ./internal/identity/ ./internal/appliance/`; exit=1; expected missing
  behavior=`identity.Service.HoldsPassword`, `appliance.NewPublishedCredential`
  and the two published-credential constants are undefined, so neither package
  compiles.
- 2026-09-14 build green: REQ-SEC-c9e034de complete; command=`go test
  ./internal/identity/ ./internal/appliance/ ./internal/network/
  ./internal/httpapi/`; exit=0. The real checker is `identity.HoldsPassword`
  behind the `appliance.PublishedCredential` adapter, wired in cmd/nilo/main.go;
  the refusal reaches the browser as 409 `published_password_unchanged` and the
  unanswerable case as 503 `commissioning_unavailable`.
- 2026-09-14 build finding, pre-existing and not fixed here: the commissioned
  management-access construction in cmd/nilo/main.go supplies no `Commissioner`,
  so an appliance returned to provisioning by a hardware recovery reset
  (management_access.go:488 sets provisioning back to true at runtime) already
  could not complete commissioning before this task, failing with
  `ErrCommissioningUnavailable`. That belongs to the recovery path delivered by
  a9573fc, not to this scope. The published-credential checker was wired into
  that construction anyway, so the gate is not missing if the Commissioner is
  added later.
- 2026-09-14 build red: REQ-PLAT-3c35bacd; command=`go test ./cmd/nilo/ -run
  TestCheckDiscoveryCertificate`; exit=1; expected missing
  behavior=`checkDiscoveryCertificate` is undefined because the decision is
  still inline in `runServe` where no test can reach it.
- 2026-09-14 build green: REQ-SEC-c9e034de and REQ-PLAT-3c35bacd; command=`go
  test ./cmd/nilo/ ./internal/network/ ./internal/identity/
  ./internal/appliance/ ./internal/httpapi/`; exit=0; `go build ./...` exit=0.
  The decision moved out of `runServe` into `checkDiscoveryCertificate` beside
  the names it checks, and both of the command's call sites now go through it;
  behavior is unchanged. Three constructions in cmd/nilo/hardware_recovery_test.go
  had to supply a published-credential checker, which is the deliberate
  fail-closed break the design recorded, landing exactly where it predicted.
- 2026-09-14 build manual observation: REQ-SEC-d0d3d4c4, inspection. Read both
  documents after writing them. 06_SoftwareDocu/Network_Operations.md gains
  "First contact and the published credential", which names the published pair,
  the expected certificate warning in the same place as the address to visit,
  the shared certificate private key as not secret, the bounded exposure of each
  and that commissioning ends the credential exposure;
  07_Delivery/NILO_First_Boot_Provisioning_Verification.md gains the same five
  facts in its implemented contract. Observed while there and reconciled: the
  closing paragraph of "What first boot does not weaken" still said a
  certificate missing an advertised name produces a warning, which a9573fc had
  made untrue for discovery names, where the appliance now refuses to start.
- 2026-09-14 validate: diff reviewed hunk by hunk against scope, exclusions,
  requirements, design, error paths, portability and security. Two findings in
  this task's own candidate were fixed here rather than carried: routing the
  provisioning path through `checkDiscoveryCertificate` had collapsed an
  unreadable certificate (exit 1) and a missing discovery name (exit 2) into one
  exit code, against the design's promise of unchanged behavior, so
  `errDiscoveryNameMissing` now separates them; and the second
  `ValidateCertificateNames` call in that path could only ever return empty, so
  it was removed and `TLSMissing: nil` stated explicitly. `go build ./...`
  exit=0, `go vet` exit=0 and the focused set exit=0 after those edits.
  Knowledge reconciled: bench-core-foundation.md gains two invariants, two Key
  behaviors entries, the published-credential precondition under Completing
  commissioning, both new files under Files, and `appliance.PublishedCredential`
  in the current-state architecture diagram, which matches the Design proposal
  with no departure; project.md's first-boot bullet now states the published
  credential and the refusal.
- 2026-09-14 validate environment: the first strict run failed with
  `command-failed (run) - test exit 1`, the appliance stage reporting
  `Repository-local Go 1.27.0 is missing at
  /repo/02_Tools/runtime/linux-x86_64/go/1.27.0/go/bin/go`. That was this
  worktree's provisioning, not the candidate: `02_Tools/runtime/linux-x86_64`
  was an NTFS junction, which a Docker bind mount does not traverse, so the
  container saw an empty directory. Replaced with a real 394 MB copy and
  re-ran. No candidate file changed between the two runs.
- 2026-09-14 validate proof: command=`python .agentic/aegis.py check --task
  TASK-99ea6c3b --strict --run`; exit=0; `strict: ok`; run commands `test` ok
  (358.2s, the full Go, Python, GUI and appliance-stage suite) and `lint` ok
  (7.5s); summary errors=0 warnings=39 notes=2. The 39 warnings are the
  pre-existing master-state findings this task did not touch - stale
  requirement paths, unticked deliver sections and invalid Handoff status in
  other tasks' journals - plus this task's own two, both since corrected. Note
  `manual-verification` for REQ-SEC-d0d3d4c4 is expected: it is
  inspection-verified and its observation is recorded above.
- 2026-09-14 validate review round 1: specification compliance approved and
  implementation quality approved, both against the final proven candidate.
  Compliance: all three requirements are built as written, every stated
  exclusion holds - internal/auth/, sessions and CSRF are untouched by the diff,
  no image-build work, no discovery change, no per-appliance credential and no
  configurable password policy - and the design was followed including the
  credential test over a marker and the fail-closed gate. Quality: the gate
  precedes every read and apply so a refusal has nothing to undo; the interface
  answers one boolean and an API test asserts no response carries the published
  password; `HoldsPassword` is unrouted, takes no part in throttling, and a test
  proves repeated calls leave the login throttle intact; the code added is pure
  Go with no OS-specific paths. Two accepted observations, neither requiring
  change: `HoldsPassword` answers a malformed or out-of-bounds verifier as a
  plain false, which is commented and is the honest answer for a password the
  rule would refuse to set; and the published credential was wired into the
  commissioned-path construction as well as the provisioning one, which is
  beyond the minimum but is the reason given in the code - a hardware recovery
  reset returns that instance to provisioning at runtime. Recorded during
  review: `cmd/nilo/development.go:19` already defines the same
  `admin`/`admin_password` pair for the development administrator, confined to
  the simulation profile, so RULING-001's value is consistent with practice this
  repository already had; the two constants are deliberately separate because
  one is a development convenience and the other is appliance behavior.

- 2026-09-14 validate review round 2, human review: both verdicts return to
  changes_required and the task leaves deliver without publishing. Four findings
  and one evidence gap were raised against the proven candidate.
  ACCEPTED AND FIXED - `identity.HoldsPassword` did not check `user.Enabled()`,
  and `CanPasswordLogin` is orthogonal to it (OBSERVED internal/auth/user.go:35,
  39), so a disabled published identity still reported as holding its password
  and would have blocked commissioning on an appliance where that identity could
  no longer sign in at all. Fixed, with
  `TestHoldsPasswordRejectsADisabledIdentity` covering it; `go test
  ./internal/identity/ -run TestHoldsPassword` exit=0.
  ACCEPTED, DECISION OWED - the gate cannot distinguish a seeded administrator
  from a console administrator who chose the published pair, because the two are
  indistinguishable in stored state without a provenance marker. The frame
  recorded this risk and the design accepted it; the review is right that no
  test pins the behavior and that issue #13 item 6 reads the other way.
  DISPUTED ON PREMISE, DECISION OWED - the fixed-address SAN of item 7 cannot be
  built because no fixed address exists anywhere in the product: issue #12's
  landed direction uses IPv4 link-local and explicitly adds no fixed recovery
  subnet, and REQ-FUNC-554b9bbd states the appliance publishes each interface's
  current addresses rather than a fixed one. This was raised at the frame
  checkpoint and the reading was accepted before specify.
  ACCEPTED AS A PRODUCT QUESTION, DECISION OWED - refusing to start on a missing
  discovery SAN does make item 8 unsatisfiable for those names, and an appliance
  that will not start cannot be commissioned at all, which is the failure this
  ticket exists to remove. The behavior is a9573fc's, not this task's (OBSERVED
  `git show a9573fc -- cmd/nilo/main.go`); this task preserved it deliberately.
  ACCEPTED AS AN EVIDENCE GAP - acceptance item 1 is end-to-end over an image,
  and image placement is out of scope here, so this branch cannot prove it. It
  needs delivery or image evidence recorded outside this task.
  Stage returned to build. The candidate has changed since the recorded proof,
  so the strict run and both verdicts no longer describe it.

- 2026-09-14 build round 2: the three rulings applied. RULING-004 reversed the
  round-1 fix rather than extending it: `HoldsPassword` had just been given an
  `Enabled()` check, and the ruled invariant is the opposite, because a disabled
  published verifier could be re-enabled after commissioning. Renamed to
  `identity.Service.PasswordVerifierMatches` and
  `appliance.PublishedCredential.PublishedVerifierRemains`, which name the
  property rather than the ability to sign in, with `network.PublishedCredential`
  following. RULING-003 replaced `checkDiscoveryCertificate` with
  `missingDiscoveryNames`: both start paths now warn and pass the gap through
  `TLSMissing` instead of exiting, so a misprepared image stays reachable and
  issue #13 item 8 is met; an unreadable or malformed certificate still stops the
  start. Both requirements were rewritten to the ruled behavior, including
  REQ-PLAT-3c35bacd's title, and their linked tests refreshed. Tests added for
  the three cases the review named - a console-created `admin` holding the
  published pair, a different name or password, and a disabled identity still
  holding it - against the real identity service rather than a stub, because
  provenance is exactly what stored state does not record. `go build ./...`
  exit=0, `gofmt -l` clean, and `go test ./cmd/nilo/ ./internal/network/
  ./internal/identity/ ./internal/appliance/ ./internal/httpapi/` exit=0.
- 2026-09-14 build round 2 documentation: Network_Operations.md, the delivery
  document and the component page all stated the refuse-to-start behavior and
  were reconciled to reporting; the knowledge page's commissioning section now
  states the verifier-based rule and the renamed seam. RULING-005 was carried to
  the ticket: issue #13 was amended in place - the published pair, the
  verifier-based rule, item 6, item 7, item 8, the out-of-scope discovery line
  and every fixed-address reference - with a dated note recording what changed
  and why.

- 2026-09-14 validate round 2 proof: command=`python .agentic/aegis.py check
  --task TASK-99ea6c3b --strict --run`; exit=0; `strict: ok`; run commands
  `test` ok (462.0s, the full Go, Python, GUI and appliance-stage suite) and
  `lint` ok (8.2s); summary errors=0 warnings=39 notes=2. The 39 warnings are
  the pre-existing master-state findings in other tasks' journals and
  requirements, untouched here. Inspection for REQ-SEC-d0d3d4c4 was carried out
  again because both documents changed after the round-1 observation: the
  published pair, the shared certificate key as not secret, the expected
  certificate warning named beside the address to visit, the bounded exposure of
  each and commissioning as what ends the credential exposure are all present in
  06_SoftwareDocu/Network_Operations.md and
  07_Delivery/NILO_First_Boot_Provisioning_Verification.md. A sweep for stale
  refuse-to-start claims and for the old `HoldsPassword`, `StillOpensAppliance`
  and `checkDiscoveryCertificate` names found none outside the unrelated
  lifecycle-state rule.
- 2026-09-14 validate review round 2: specification compliance approved and
  implementation quality approved against the re-proven candidate. Compliance:
  the three rulings are built as ruled, both requirements were rewritten to the
  ruled behavior before the proof rather than after it, issue #13 now matches the
  delivered contract, and every original exclusion still holds. Quality: the
  renamed seam states the property it enforces, the three review cases are tested
  against the real identity service rather than a stub, an unreadable certificate
  remains a hard failure while an incomplete one is reported, and the gate still
  precedes every read and apply. Accepted and unchanged from round 1: the gate is
  wired into the commissioned construction as well as the provisioning one,
  because a hardware recovery reset returns that instance to provisioning at
  runtime.

- 2026-09-14 build round 3, human review: three findings, all accepted and
  fixed. The credential gate is now atomic rather than merely narrowed. The
  second verifier check added in round 2 was removed as the redundant half-
  measure it was; instead `sqlite.CommissioningRepository.CommissionUnlessPasswordMatches`
  reads the published user's verifier and writes the lifecycle state in one
  `BEGIN IMMEDIATE` transaction, so a password reset either lands entirely
  before it and is seen, or entirely after it, when the appliance is already
  commissioned. `appliance.DurableCommissioner` routes through it when the
  writer supports it, and the network service recognizes the refusal through a
  `PublishedCredentialPresent() bool` method rather than a shared sentinel, so
  neither package has to import the other. The early check stays as a fast,
  well-worded refusal, no longer as the guarantee. The legacy `--bind` appliance
  listener now reports a missing discovery name and serves, like the other two
  modes, rather than exiting; and the management path merges the configured and
  advertised missing-name lists through `mergeMissingNames` instead of
  concatenating them, so an overlapping name is no longer reported twice to the
  API and the interface. `go build ./...` exit=0, `go vet` exit=0, and `go test`
  across store/sqlite, appliance, network, identity, httpapi and cmd/nilo
  exit=0.
- 2026-09-14 deliver attempt withdrawn and hardware evidence: the candidate was
  deployed to the bench Raspberry Pi 4 `ddp-remote` (aarch64, Debian 13) at
  10.44.118.42, found on Wi-Fi rather than the DDP LAN because its management
  access is bound to `nilo0` and `wlan0`. Built `linux/arm64` CGO-free,
  sha256 31f31fc4, previous binary backed up to
  /var/backups/nilo/20260914T210651Z/nilo (sha256 3938b5a6). The service
  restarted active with NRestarts=0 and served the GUI (200) and the API (401
  unauthenticated) over HTTPS from this workstation. This proves the candidate
  runs on real appliance hardware without regressing a commissioned appliance;
  it does not exercise the commissioning gate, because that appliance is
  commissioned and `Commission` refuses earlier. Acceptance items 1, 4, 5 and 6
  remain unproven on hardware.

- 2026-09-14 build round 4, human review: the transaction claim was corrected
  by making it true rather than by trimming the comment. `BeginTx(ctx, nil)` is
  a deferred transaction and the DSN sets no `_txlock`, so the comment claimed a
  lock mode that was not configured; correctness held only because
  `SetMaxOpenConns(1)` serializes this process's writes, which is an emergent
  property of pool size rather than a stated guarantee, and does not cover a
  second process holding the same data directory - `nilo admin user
  reset-password --data-dir` is exactly that. The method now reserves a
  connection and issues `BEGIN IMMEDIATE`, the idiom
  `IdentityRepository.WithinAdministrativeTransaction` already uses for the same
  reason, so the write lock is taken before the verifier is read. `go build
  ./...` exit=0 and `go test ./internal/store/sqlite/ ./internal/network/
  ./internal/appliance/` exit=0.
- 2026-09-14 build round 4: the round-3 strict run failed on this task's own
  requirement file, not on the candidate - `run: test ok (374.6s)` and `lint ok`
  both passed, while `req-parse` reported REQ-SEC-c9e034de.toml as invalid TOML
  because an edit removing a renamed test left a truncated entry in
  `linked_tests`. The list was rewritten and validated; `check --task` now
  reports no errors. No candidate file was involved.
- 2026-09-14 build round 4 open decision: acceptance item 8 remains unmet for
  the legacy `--bind` appliance listener. That mode now serves and warns on
  stderr, but it composes no `ManagementAccessService`, so the management-access
  route that carries `TLSMissing` is never registered
  (OBSERVED internal/httpapi/server.go:307) and the gap cannot reach the
  interface. Either that mode gains interface-visible warning state, or item 8
  is narrowed to the first-boot and interface-managed listeners. Awaiting the
  user's decision; not guessed.

- 2026-09-14 build round 4 resolved: item 8 narrowed by RULING-006. The legacy
  `--bind` listener keeps reporting the gap on stderr and in the journal and
  gains no interface state; REQ-PLAT-3c35bacd now states the distinction between
  the three listener modes rather than implying one behavior, and issue #13 item
  8 was amended in place with a dated second amendment recording why.
  06_SoftwareDocu/Network_Operations.md and the component page were reconciled to
  the same wording. Two tests were added at cmd/nilo: one asserting every mode
  answers through the same decision and differs only in where it reports, and one
  asserting an overlapping name is merged once rather than repeated. `go test
  ./cmd/nilo/` for those selectors exit=0.

- 2026-09-14 build round 5: the version bump missed a fifth mirror. The project
  version is declared in `VERSION` and mirrored by `internal/buildinfo` and
  `nilo_gui/package.json`, but bumping the GUI package rewrites
  `package-lock.json`, whose digest is recorded in `02_Tools/toolchain.versions`,
  in `02_Tools/containers/build/build-environment.lock.json` and a third time in
  `05_Software/scripts/tests/test_toolchain_bootstrap.py` - the same coupling
  that left master red at bdcd757 when 4df2b14 changed the lock without
  regenerating the digest. All three were regenerated and
  `scripts.tests.test_build_environment` with
  `scripts.tests.test_toolchain_bootstrap` ran 59 tests OK. A strict run then
  failed on four GUI assertions hard-coding `Version 0.4.0` while the interface
  correctly rendered `0.5.0`; those were corrected and the GUI suite is 490/490.
  Recorded because the coupling is not obvious from the version files alone.
- 2026-09-14 deliver: the user approved delivery and reported testing the
  appliance behavior themselves, which is the hardware evidence acceptance items
  1, 4, 5 and 6 need and which this branch cannot produce - the bench appliance
  is commissioned, so `Commission` refuses before the gate is consulted. That
  observation is the user's, not this task's; no automated evidence here covers
  it. The candidate is deployed to `ddp-remote` as 0.5.0
  (sha256 9eb31094), with the pre-task binary retained at
  /var/backups/nilo/20260914T210651Z/nilo (sha256 3938b5a6) as the rollback.
  Investigated a report that the appliance could not be reached at
  https://ddp-remote.local:8443/ : the appliance was serving, resolving its own
  name through avahi and presenting an unchanged certificate that verifies
  against its own SANs, and the journal showed the client aborting the handshake
  with `tls: unknown certificate`, which is the browser refusing the self-signed
  signer rather than a fault introduced here. `git diff a9573fc` on
  cmd/nilo/main.go touches no mDNS or announcer line.

## Rulings

- RULING-001 | date=2026-09-13 | decision=the published first credential is user name `admin` with password `admin_password`, not the `admin`/`admin` pair issue #13 specifies | reason=`admin_password` is 14 bytes and already satisfies the compiled-in 12-byte minimum, so the explicit confined password-rule exception issue #13 asks for is no longer needed; the credential-bound commissioning refusal that carries the security argument is unaffected | source=user | supersedes=none
- RULING-002 | date=2026-09-14 | decision=the configurable password policy requested mid-frame is issue #16, "Configure the appliance password policy", and is out of scope for this task, which keeps the compiled-in rule unchanged | reason=it governs every password set on the appliance rather than first boot alone, it needs a persisted administrator-settable settings surface that does not exist yet (internal/config is startup-only), and folding it in would make this task's acceptance untestable against issue #13 | source=user | supersedes=none

- RULING-003 | date=2026-09-14 | decision=a certificate missing a required discovery name no longer refuses the start; the appliance serves, records the missing names in `TLSMissing` and reports them in the interface, treating itself as a degraded or misprepared appliance. Item 7 remains the requirement a successful image must meet | reason=an appliance that will not start cannot be commissioned at all, which is the failure issue #13 exists to remove, and the interface already has the reporting path issue #13 item 8 asks for. This supersedes the startup-refusal behavior issue #12 introduced in a9573fc | source=user | supersedes=none
- RULING-004 | date=2026-09-14 | decision=commissioning is refused whenever the `admin` identity still holds a verifier matching the published password, regardless of how that identity was created and regardless of whether it is currently disabled. Issue #13 item 6 is amended to match, and the concept is named after the property it states rather than after signing in | reason=seeded and console-created identities are indistinguishable in stored state, and the property worth enforcing is that no published verifier survives commissioning. Excluding a disabled identity would let a published credential outlive commissioning and be re-enabled afterwards | source=user | supersedes=none
- RULING-005 | date=2026-09-14 | decision=issue #13 is amended so the first-contact certificate requirement is the name pair `<system-hostname>.local` and `nilo.local`, with every associated fixed-address reference removed or revised across the problem statement, the certificate section, the acceptance criteria and the out-of-scope list | reason=no fixed address exists in the product: issue #12's landed direction uses IPv4 link-local and adds no fixed recovery subnet, and REQ-FUNC-554b9bbd states the appliance publishes each interface's current addresses rather than a fixed one. A literal fixed-IP first-contact path would mean reopening issue #12 | source=user | supersedes=none

- RULING-006 | date=2026-09-14 | decision=issue #13 item 8 is narrowed to the first-boot and interface-managed listeners. The legacy `--bind` appliance listener reports a certificate name gap on stderr and in the journal and keeps serving; it gains no interface-visible warning state | reason=that mode is explicitly operator-configured, with the listener named and the certificate supplied by hand, and it composes no ManagementAccessService, so it has no management-access interface contract to report into. Adding an endpoint and GUI state solely for the legacy mode would be disproportionate to what it reports | source=user | supersedes=none

## Handoff

current: deliver
status: waiting
next: obtain the human's publication decision, then commit on ddp/nilo-first-boot-signin
must_read: .agentic/tasks/TASK-99ea6c3b.md, .agentic/steps/6_deliver.md
blockers: none. The candidate is validated and unchanged since the proof; publication needs the human's approval and the commit is the human's to run per CLAUDE.md

## Stage report

stage: build
result: blocked
current: build
changed: since the last full proof - internal/store/sqlite/commissioning_repository.go and its test, internal/appliance/{commissioner,published_credential}.go, internal/network/commissioning.go and its test, cmd/nilo/{main,discovery_runtime}.go, and the requirement file
evidence: go build ./... exit=0; go test across store/sqlite, appliance, network, identity, httpapi and cmd/nilo exit=0. The last strict run failed only on this task's own requirement TOML, with `run: test ok (374.6s)` and `lint ok`; the TOML is repaired and `check --task` reports no errors. A fresh full strict run is owed and has not been made since these changes.
issues: one decision is owed before this can be finished - acceptance item 8 cannot be met for the legacy --bind appliance listener without giving that mode interface-visible warning state, because it composes no ManagementAccessService and so never registers the route that carries TLSMissing. Acceptance items 1, 4, 5 and 6 remain unproven on hardware, because the bench appliance ddp-remote is commissioned and Commission refuses before the gate.
next: take the item 8 decision, then re-prove with check --strict --run
needs_user: yes
spec_compliance: changes_required
implementation_quality: changes_required
review_round: 3
