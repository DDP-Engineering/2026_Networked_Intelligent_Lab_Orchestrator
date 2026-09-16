# REQ-FUNC-17f9c447 - Print an appliance label from the command line

## Behavior

`nilo discover --label <name>` performs the normal bounded dual-protocol
discovery, selects exactly one appliance by serial or friendly name, and writes
one UTF-8 SVG label to standard output. It writes diagnostics only to standard
error. Table output is suppressed, and `--label` is incompatible with `--json`.

The label is a 62 mm by 29 mm black-on-white vector document containing the
permanent serial name, normalized management-interface MAC addresses and a QR
code for `https://<serial>.local/`. Friendly name and current IP addresses may
change and therefore do not appear in the permanent artifact.

Zero matches, ambiguous matches, incomplete records and invalid public fields
return non-zero before any standard-output byte is written. The command does
not create a file; the workshop may redirect its atomic standard output to a
chosen `.svg` path.

## Data / interface

```text
nilo discover --label nilo-abcd234567 > nilo-abcd234567.svg
```

The shared Go label renderer accepts a validated serial name, a sorted unique
MAC-address list and an HTTPS URL. It builds the QR matrix with a pinned pure-Go
dependency and emits escaped fixed-layout SVG; callers cannot inject markup.

## Acceptance

- The SVG parses, renders at the documented dimensions and its QR decodes to
  the exact permanent-name HTTPS URL.
- Serial and MAC text are stable regardless of discovery arrival order.
- Invalid, missing or ambiguous selection emits no partial document.
- CLI, authenticated About and loopback status use the same label model and QR
  renderer.

## Resolved design decisions

SVG standard output is portable, printable and automation-safe without an
implicit filesystem write. A single server-side renderer avoids three QR
implementations and prevents browser-side untrusted SVG injection.
