# REQ-SEC-dd110b06 — Keep package preparation offline

## Behavior

Preparation is a separate code path from connected fetch. It validates the
target and complete cache, then observes the running Linux network namespace.
It proceeds only when `lo` is the sole interface and neither the IPv4 nor IPv6
route tables contain a default route. It creates a deterministic receipt
atomically after every check succeeds.

Preparation on Windows, macOS, a normal Linux host, or a namespace with another
interface or default route is refused. The supported entry point runs inside
the repository's locked OCI build environment, whose engine invocation already
uses network-none isolation; the runtime observation defends that boundary
from accidental wrapper regression. Kernel-generated IPv6 reject routes on
loopback are non-forwarding isolation records, not usable default routes, and
do not cause a network-none namespace to be misclassified as connected.

## Data / interface

The command is:

```text
prepare --targets <targets.json> --lock <inputs.lock.json>
        --cache <directory> --target <target-id> --output <receipt.json>
```

The POSIX wrapper is:

```text
02_Tools/packaging/build/prepare-offline.sh <target-id> <receipt-path>
```

It uses the repository defaults for configuration and
`02_Tools/cache/packaging`, and is intended to be invoked by
`02_Tools/env/with-build-environment.sh package <target-id>`, which calls the
new wrapper explicitly through `sh` and therefore does not depend on its file
mode.

The receipt is canonical UTF-8 JSON with schema `1`, the complete target
definition, raw lock-file SHA-256, selected verified inventory, the network
observation (`platform`, sorted interfaces, IPv4/IPv6 default-route booleans),
and hardware-acceptance state. It contains no wall-clock time or host path.

## Examples

Inside a network-none Linux container with only `lo`, preparation writes the
receipt. On the development host with `eth0`, it returns `2` and does not touch
an existing receipt.

## Acceptance

Unit tests inject platform/interface/route observations and assert the positive
and every refusal path. A call spy proves preparation never calls the
downloader. An OCI boundary test executes the actual `package` action with a
complete fixture, records the engine arguments, and requires network-none, a
read-only packaging-cache mount, and a receipt observing only loopback with no
default route.

## Assumptions and open decisions

The first increment prepares and records verified inputs; it does not yet
construct `.deb`, `.run`, or `.img.xz` artifacts.
