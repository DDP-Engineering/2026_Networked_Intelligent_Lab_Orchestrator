# REQ-FUNC-3d711f4f — Validate manifest and adapter agreement

## Behavior
The package manifest is authoritative for operation ids, input and result schema references, approval policy, timeout, state-changing declaration, and artifact ids/media types. A registered adapter canonicalises family inputs and supplies device identity, dynamic resource keys, curated runner arguments and core-owned paths. Load fails before health or execution if the adapter names an undeclared operation or artifact, contradicts a manifest declaration, or fails to implement a required family hook.
## Data / interface
Registry lookup uses the manifest package id. An adapter does not inject a new public operation or widen a schema. The adapter may derive a resource such as `camera/device/cam-2` only after an id is resolved from a managed record. Static manifest resources remain additive. Sigrok's capture and decode behavior remains the compatibility oracle.
## Examples
A camera adapter asks for `snapshot` with the manifest's 30-second timeout and `image` artifact: load succeeds. The same adapter advertises `snapshot` when the manifest omits it: load fails with a package-contract error.
## Acceptance
A hostile mismatch fixture fails before process launch; a matching fixture and unchanged Sigrok workflow pass.
## Assumptions and open decisions
The exact Go interface is a design decision. Manifest schema changes must stay compatible with the existing `nilo.integration/1` envelope.
