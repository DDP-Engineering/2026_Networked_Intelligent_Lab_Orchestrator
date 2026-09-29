# Modbus Core definitions and shared transport ownership

Requirements: REQ-DATA-a8c75050, REQ-DATA-2988e315,
REQ-FUNC-eafa69bc, REQ-SEC-4b8a52fa, REQ-REL-5af947a8,
REQ-REL-4e21290d.

## Contract

The accepted MB-01 endpoint/profile JSON contract remains authoritative.
Core services expose typed records internally; public HTTP/CLI/MCP and GUI
adapters are implemented by their respective later tickets. Saving and browsing
perform database operations only. A profile creator is assigned from the
authenticated actor and retained on updates. Readers need Bench read permission;
mutations need plugin management permission and generate audit events.

Definitions use opaque IDs, schema version 1 and positive signed 32-bit revisions.
Creation expects revision zero; replacement expects the stored revision. A stale
revision returns conflict with no partial publication. Endpoints and profiles
live in additive SQLite tables; a profile references an existing endpoint.
An endpoint transport change is rejected while profiles reference it. Live claims
prevent definition/settings changes. Disabling an endpoint retains its identity
and configuration and cancels its own active consumers.

RTU endpoints carry Core-owned USB identity only. Trusted discovery assigns that
identity; administrator input cannot replace it. Network endpoints carry host,
port, unit range and an administrator-assigned bus/resource ID only. TCP permits
units 0..255; RTU-over-TCP permits 1..247. Host values are normalized DNS/IP
names, without URLs, paths, credentials or IPv6 zones. Network endpoints remain
configured and browseable while offline. Their readiness starts offline and is
updated only by a trusted consumer observation. USB presence comes from discovery;
settings never manufacture presence. Disabled USB readiness is unconfigured.

Profiles require explicit response/connect timeouts, inter-request gap and read
retry count, with the signed 32-bit bounds from MB-01. Serial framing is required
for RTU and forbidden for network transports: baud 50..4000000, eight data bits,
even/odd parity with one stop bit, or no parity with one/two stop bits; local echo
is explicit. Missing and unknown JSON fields are rejected. Profile settings are
validated against the endpoint before publication. There are no implicit I/O
defaults.

## USB reconciliation

Trusted USB observations use the existing verified Serial discovery format and
managed-device repository under the Modbus family, independent of Serial's
installed/enabled state. Strong serial identity includes interface/port suffixes;
weak/duplicate serial identity is sticky and port-bound. Reconciliation projects
each managed adapter into a retained Modbus endpoint and changes its revision only
when identity/binding/presence/readiness changes. Names, enablement and profiles
are retained on disappearance. Startup marks USB bindings absent and network
readiness offline until fresh trusted observations. Profile creation remains
possible for absent/offline endpoints; lease admission requires enabled/present
USB or enabled/configured network records and matching definition revisions.

## Canonical claims

One Manager owns resource admission. Claims are separate from finite job leases
and exist for the complete transport consumer lifetime. A USB claim atomically
reserves its physical port-path key and, for a strong serial, its vendor/product/
serial identity key. Keys exclude family, plugin instance, managed record and tty
alias. Port keys also prevent differing weak-serial classifications from granting
two owners. Independent weak ports remain independent.

Network consumers atomically claim the configured bus key and normalized host/
port socket key. Unit IDs and transport family names are excluded. Explicit shared
bus IDs join DNS aliases or different sockets on one gateway; the socket key
prevents two bus labels on the same configured peer from bypassing exclusivity.
DNS resolution and remote probing belong to the later runner ticket. USB and
network namespaces are distinct and cannot collide through administrator labels.

Admission checks all keys before installing any. Competing claims return busy.
Registry ownership uses a unique claim token; stale cleanup cannot release another
owner. Consumers retain immutable endpoint/profile snapshots. Internal Modbus
reservation is a trusted runner/session seam, not a public policy bypass.

## Cleanup and failure

Cancellation requests stop, but never releases ownership. Serial obtains its
claim before runner creation. Failed startup performs runner cleanup before
release; live Serial releases after Execute finishes, Shutdown succeeds and local
transport/workspace cleanup completes. Cleanup errors retain a fenced claim and
are reported. No retry replays a device action.

Trusted Modbus consumer lifetimes accept a cancellation context and a lifetime
function whose return confirms cleanup (a returned error fences the claims). Expiry, endpoint disable, trusted unplug/offline, runner exit, family
disable and Core shutdown request cancellation. Cleanup runs outside Manager.mu;
success releases claims and failure fences them. Shutdown waits for consumers.
Family-level changes inspect that family's claims; physical configuration/rebind/
forget checks also inspect shared USB claims. Camera behavior remains unchanged.

Fenced claims remain unavailable for the Manager lifetime; recovery requires an
operator-confirmed process restart with existing supervised child-process cleanup.
Physical driver and appliance acceptance remain separate from fixture evidence.
