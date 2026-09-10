# Build-environment archive escrow

`current.json` is normal Git metadata. Files ending in `.partNNN` are the only
build-image payloads permitted in Git LFS. The checked-in manifest deliberately
records `pending-export`; no binary or checksum is invented before a real image
is built and exported. Generate the complete set with the command in
`current.json`, then run `verify-archive-round-trip.sh` to import it, execute the
offline suite, and promote its initially pending verification status. Review it
before the human publication preflight. Never publish a subset or a pending
round trip as a usable archive.
