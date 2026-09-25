# REQ-REL-de43509b — Preserve typed camera failures

## Behavior
The terminal job stores the allowlisted runner failure code when the runner returns one of the documented camera errors. Unknown codes become `execution_failed`; full diagnostics remain in bounded server-side diagnostic storage. Immediate submission refusals and later job reads use the same public vocabulary, so HTTP, CLI and MCP can report the same failure.
## Data / interface
Allowlist: `device_not_ready`, `device_initializing`, `device_absent`, `device_busy`, `unsupported_endpoint`, `unsupported_format`, `control_out_of_range`, `control_unsupported`, `frame_timeout`, `invalid_parameters`, `runtime_unavailable`. Error messages remain bounded and do not include device-node paths, ioctl values, runner stderr or secrets.
## Examples
A busy V4L2 opener yields device_busy. A runner code `vendor_internal_trace` yields execution_failed to every client.
## Acceptance
Fixture runner and transport tests compare the terminal job and each client representation.
## Assumptions and open decisions
The common HTTP error envelope remains the existing one; this spec fixes only code preservation and safe detail.
