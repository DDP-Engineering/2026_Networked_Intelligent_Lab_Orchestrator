# REQ-PLAT-532864ed — Bundle self-contained production integrations

## Behavior

Each target catalogue contains only production integrations. Sigrok carries a
target-native worker, adapter runtime, required shared libraries, schemas,
licences and provenance beneath its bundle root. Its launcher sets confined
runtime paths and opens no registry or network dependency. `fake_python` is a
test fixture and cannot be selected by a release catalogue.

## Data / interface

The builder writes `catalog.json`, one canonical package manifest per
integration, `files.sha256.json`, `runtime-inventory.json` and `LICENSES/`.
The Sigrok closure is embedded except for the target's documented base ABI,
kernel, udev and USB facilities. Bundle members are ordinary files and
directories; absolute links, escaping relative links and special files are
forbidden. Measured count and byte totals plus 20 percent headroom, rounded up,
become bounded verifier limits.

## Examples

The amd64 catalogue launches Sigrok when `/usr/bin/python` is absent. Removing
one embedded library or replacing the worker with the host architecture fails
the health check. A release scan that finds `fake_python` fails by name.

## Acceptance

Both target bundles pass manifest verification and real-runtime health under a
networkless environment. Architecture, runtime closure, licence and source
obligation inventories are complete. An unavailable redistributable closure
blocks that target rather than silently dropping Sigrok.

## Assumptions and open decisions

GPL/LGPL corresponding-source material or a compliant written offer is emitted
beside the release before an artifact may be distributed.
