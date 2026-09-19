# REQ-REL-f4a90899 — Reject incomplete packaging caches

## Behavior

Cache verification validates both configuration documents, selects `shared`
and target-scoped entries, and evaluates every selected object. It returns a
sorted verified inventory only when the lock is complete and all objects match.
Configuration defects are reported independently because malformed metadata
may prevent safe object selection. Once selection is valid, one run accumulates
every missing, wrong-sized, digest-mismatched, symlinked, or otherwise
non-regular selected object it can inspect.

Preparation reads each configuration file once, parses and verifies those
captured bytes, and hashes that same lock snapshot into the receipt. Cache
object access rejects links and traverses derived object paths without
following filesystem links on the Linux offline boundary. The final object is
opened non-blocking so a FIFO or other special file cannot stall verification
before its file type is rejected.

No cache directory scan grants trust to an undeclared file. Extra immutable
objects may coexist because the cache is shared across targets, but they never
enter a target inventory unless named by the validated lock.

## Data / interface

The command is:

```text
verify-cache --targets <targets.json> --lock <inputs.lock.json>
             --cache <directory> --target <target-id>
```

On success it prints canonical JSON containing the selected target ID and
sorted entries with `name`, `scope`, `version`, `license`, `sha256`, `size`,
and repository-relative `cache_object`. It returns `0`.

Configuration, completeness, and object errors are printed one per line to
stderr and return `2`. The verifier writes no output file.

## Examples

A shared toolchain archive and Ubuntu-scoped package both appear in the Ubuntu
inventory. Removing the package and corrupting the shared archive reports both
defects in one run. An ARM64-only object does not appear in the Ubuntu
inventory.

## Acceptance

Tests independently cover incomplete, duplicate, unknown-scope, unsafe-name,
and cross-target configuration fixtures. Object fixtures cover missing, short,
long, corrupt, symlinked, and FIFO paths, including prompt rejection of a FIFO.
The aggregate regular-file defect fixture also confirms that preparation writes
no receipt. Configuration validation and `verify-cache` write no receipt by
interface. Snapshot replacement cannot mix parsed inputs and the receipt
digest.

## Assumptions and open decisions

The first checked-in production lock is intentionally incomplete until the
exact OS dependency closure and Raspberry Pi base image are resolved. That
state must fail closed and name its outstanding inputs.
