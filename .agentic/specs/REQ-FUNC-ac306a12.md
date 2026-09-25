# REQ-FUNC-ac306a12 — Configure runner before device use

## Behavior
For each runner process, `hello` establishes the existing protocol version; the next hardware-capable lifecycle step is `configure`, then describe, discover or execute may run. Configuration refusal prevents hardware use and marks runner health failed. The configuration carries plugin settings and the target device's settings and revisions; discovery uses a plugin-only target where no device has yet been selected.
## Data / interface
A versioned configure payload uses the existing `configure` method with `schema_version: 1`, `plugin: {revision, settings}` and `device: null | {id, revision, settings}`. Each settings object is validated against its confined installed manifest schema before launch. Neither public requests nor untrusted runner replies choose paths in the payload. Reconfiguration starts a new runner or redoes configure before hardware work; a running process never silently retains stale settings.
## Examples
A runner acknowledges plugin-only configuration before device discovery. Before
a snapshot on cam-2, a runner receives the plugin limit and that device's
control baseline and acknowledges configuration. A runner that rejects a
settings schema is never asked to execute snapshot.
## Acceptance
Supervisor and SDK contract tests assert order, one configuration per startup,
rejection behavior and Sigrok compatibility. The linked Camera test checks the
configured plugin image limit; a direct baseline-control assertion is still
needed before this draft is marked active.
## Assumptions and open decisions
Exact payload field names may be adjusted during design to match existing protocol codec constraints; the order and validation semantics are fixed.
