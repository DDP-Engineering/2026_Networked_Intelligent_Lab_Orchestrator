# Build-environment archive escrow

`current.json` is normal Git metadata. Files ending in `.partNNN` are the only
build-image payloads permitted in Git LFS. Generation `noble-20260928-01` has
completed a local archive import and offline suite; its manifest and metadata
record the measured identities and evidence. Gitea publication and independent
backup remain pending and must be completed by a human operator.

Keep only the current generation's parts in this directory: inspection rejects
extra, missing, reordered or corrupt parts. Prior generation payloads can be
retained in a separate ignored recovery directory or recovered from their
historical manifest and LFS objects. This checkout retains the previous archive
under `02_Tools/cache/issue-38/previous-archive/`.

After exporting a future generation, run `verify-archive-round-trip.sh` to
import it, execute the offline suite, and promote its initially pending
verification status. Review it before the human publication preflight. Never
publish a subset or a pending round trip as a usable archive.
