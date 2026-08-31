# REQ-FUNC-880eac8d - Execute immutable operation lifecycle

## Behavior

Bench Core accepts only a validated typed operation request. The accepted request is an immutable value containing its request ID, authenticated actor, integration package ID, operation descriptor ID, stable device ID, canonical JSON parameters, configuration snapshot ID, policy snapshot ID, creation time, and deadline.

The request owns one session. The defined session states and transitions are:

```text
pending_approval -> queued -> running -> completed
                                      -> failed
                                      -> cancelled
                                      -> incomplete
```

`completed`, `failed`, `cancelled`, and `incomplete` are terminal. Rejected and expired approvals remain approval records and do not dispatch a runner operation. Undefined transitions fail with a typed state-conflict error.

## Data / interface

Runtime identities use strongly typed UUID values. Operation descriptor IDs use stable validated strings rather than runtime UUIDs. The accepted request stores all authoritative fields privately and exposes value-returning getters only; canonical parameter JSON is copied when the request is constructed, copied again for every accessor result, and limited to 1 MiB.

The orchestration boundary is:

```go
type Service interface {
    Submit(context.Context, SubmitCommand) (domain.Session, error)
    Approve(context.Context, domain.SessionID, domain.Actor) (domain.Session, error)
    Deny(context.Context, domain.SessionID, domain.Actor, string) error
    Cancel(context.Context, domain.SessionID, domain.Actor) error
    Get(context.Context, domain.SessionID) (domain.Session, error)
}
```

Allowed work follows this order: validate package, operation, device, parameters, and deadline; create request and policy snapshot; resolve policy; append the decision audit event; create pending or queued session; atomically acquire leases for queued work; transition to running; execute the runner; validate and persist results and required artifacts; append the terminal audit event; transition to the truthful terminal state; initiate lease release.

## Examples

- Ask mode with an auto-eligible operation creates `pending_approval` and does not invoke a runner.
- Approval of the unchanged unexpired request advances it to `queued`; approval of changed parameters is rejected.
- Auto mode with an auto-eligible operation proceeds to `queued` without approval.
- A runner crash without a trustworthy terminal response produces `incomplete`, never `completed`.

## Acceptance

Tests cover every allowed edge, representative forbidden edges, policy-driven pending and automatic paths, immutable approval, successful evidence, cancellation, crash, artifact failure, resource conflict, and restart reconciliation.
