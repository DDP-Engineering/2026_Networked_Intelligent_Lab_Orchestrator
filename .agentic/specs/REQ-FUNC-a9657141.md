# REQ-FUNC-a9657141 — Return bounded camera snapshot result or pending id

## Behavior

Camera snapshot submission creates a durable job before acknowledgement. CLI and MCP wait for a configured finite interval. On completion, MCP returns bounded image content with the transaction id; the CLI writes the validated artifact only when `--out` was requested and completion occurred. If the job is waiting for Ask-mode approval, queueing or execution, both return a structured pending state and transaction id. A lost call or elapsed wait does not cancel accepted work. The full artifact remains available through the authorized artifact reader.

## Data / interface

The MCP pending result contains `status: pending`, `transaction_id` and polling guidance; completed results contain the same id and an image MIME type validated as JPEG or PNG. MCP image content is a bounded preview no larger than 1280 pixels along either axis and within a separate decoded-pixel ceiling. The CLI prints the id and pending state and writes no file while pending. MCP always requires an explicit device id; the CLI may select its sole configured camera when exactly one exists.

## Examples

An Auto-mode simulated snapshot completed inside the wait returns image content and id. An Ask-mode job returns pending and id; approval and later polling can still complete it.

## Acceptance

Client and job tests exercise completion, pending approval, slow execution, timeout and disconnect without cancellation or unauthorized image disclosure.

## Assumptions and open decisions

Exact wait and preview pixel ceilings will be chosen in design within the existing job and image limits. No preview stream is part of this phase.
