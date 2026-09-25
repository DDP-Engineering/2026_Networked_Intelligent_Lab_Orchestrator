# REQ-DATA-8082c1dc — Retain validated camera image evidence

## Behavior
Snapshot yields one retained `image` artifact. Its bytes, digest, size, declared media type and sanitized filename are verified before registration. Only `image/jpeg` and `image/png` may be served inline; all other media uses attachment. Artifact reads retain owner-or-administrator access, `nosniff` and `no-store`. A source image's EXIF and ancillary metadata are stripped by encoding. The result records the device id, configuration and calibration revisions, capture time, selected endpoint identity without node path, format/size/interval, effective controls and actual read-back, warm-up, mounting and request transforms, encoding and byte size.
## Data / interface
The retained artifact ceiling remains 8 MiB. Declared media type must agree with decoded file signature and manifest artifact declaration. The presentation filename contains only a bounded device id and timestamp component. A request for an image exceeding pixel, output-dimension or byte limits fails with a typed code rather than silently lowering quality.
## Examples
A valid JPEG snapshot is read inline by its owner. A PNG declared as JPEG or a `text/html` artifact is rejected.
## Acceptance
Artifact-store, HTTP and runner tests inspect signature, digest, headers, ownership, provenance and oversized behavior.
## Assumptions and open decisions
Exact result JSON property names will be frozen with the camera package schemas in design.
