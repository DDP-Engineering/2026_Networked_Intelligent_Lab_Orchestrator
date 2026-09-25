# REQ-PLAT-532864ed — Bundle self-contained production integrations

## Behavior

Each target's release payload includes the production Sigrok and Camera
integrations at their current state. Sigrok carries its native worker, adapter
runtime, required shared libraries, schemas, notices and provenance. Camera
carries its target Python interpreter, pinned wheels, dependency closure,
schemas and third-party notices. Their launchers use confined bundle paths and
need neither system Python nor network access. Development fixtures cannot be
selected by a release payload.

## Data / interface

Each bundle has a canonical `manifest.json`, `files.sha256.json` and
`runtime.json`; Sigrok also records executed runtime health in
`runtime-health.json`. Sigrok's closure is embedded except for the target's
documented base ABI, kernel, udev and USB facilities. Camera's build requires
the reviewed target runtime, wheelhouse and dependency archive. Bundle members
are ordinary files and directories; absolute links, escaping relative links
and special files are forbidden. Measured count and byte totals plus 20 percent
headroom, rounded up, become bounded verifier limits.

## Examples

The amd64 Sigrok bundle launches when `/usr/bin/python` is absent. Removing one
embedded library or replacing the worker with the host architecture fails its
health check. Camera bundle construction refuses a wrong-architecture wheel,
an absent dependency or missing third-party notices. A release scan rejects a
development fixture by name.

## Acceptance

For both release targets, verify the Sigrok and Camera manifests, architecture,
runtime closure, notices and source obligations, and run their applicable
health checks without system Python or network access. A missing redistributable
closure blocks that target rather than silently dropping either integration.
The checked-in input lock is incomplete, so this acceptance is not yet a claim
that a production release was assembled.

## Assumptions and open decisions

GPL/LGPL corresponding-source material or a compliant written offer is emitted
beside the release before an artifact may be distributed.
The linked Camera unit test checks interpreter architecture and notices; an
executed no-network Camera health check on each exact target remains release
acceptance work.
