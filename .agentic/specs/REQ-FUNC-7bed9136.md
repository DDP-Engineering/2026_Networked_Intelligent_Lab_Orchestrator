# REQ-FUNC-7bed9136 — Test a Modbus connection through finite policy admission

## Behavior

An administrator selects an enabled endpoint and a saved profile. Test submits the same canonical `connect` policy intent as a live session, with pinned endpoint/profile revisions and no poll groups. Under Ask, the ordinary approval job waits for the existing human approval path; under Auto it executes immediately. Once the supervised runner reports readiness, Core closes the temporary runner and canonical transport claim before reporting a successful terminal test. Failure, cancellation and timeout close the same resources; there is no read, write, polling plan or retained live session. A terminal audit/job outcome may remain.

## Data / interface

`POST /api/v1/modbus/endpoints/{id}/test` accepts `endpoint_revision`, `profile_id`, `profile_revision`, `idempotency_key` and `timeout_seconds`. It returns the finite Bench job shape, including pending approval or terminal outcome. Core adds a canonical `test_only` Boolean to the `connect` parameters. A test is accepted only for an administrator with `plugins.manage` and `bench.execute`; the existing plugin policy still decides Ask or Auto. Timeout is bounded by the existing finite-job envelope, with a 30-second default and a 60-second ceiling for a probe. Stale definitions return conflict before I/O. CLI `nilo modbus test` and MCP `modbus_test` call this same Core behavior.

## Examples

- A configured TCP target with revision 2 and profile revision 3 under Auto yields one completed test job and no active session.
- The same input under Ask yields a pending job and opens no socket until ordinary approval.
- A stale profile revision is rejected before the runner starts; a timed-out probe releases its claim.

## Acceptance

Fake runner and policy tests confirm one readiness probe, no Modbus PDU, terminal success/error, no live session after completion and reusable transport claim. HTTP/CLI/MCP checks confirm authorization, strict input and parity.

## Assumptions and open decisions

The existing `connect` policy operation is the intentional authority for a connection test. The test owns no data-path action. Physical target success is measured later on real equipment.
