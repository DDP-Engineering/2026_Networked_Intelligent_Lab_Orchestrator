# REQ-FUNC-4249d837 — Activate a temporary direct-connect recovery link

## Behavior

Activation occurs under the physical-adapter role coordinator after a fresh
read of durable management, device-network/integration, interface-settings, and
helper ownership state. Any protected or uncertain state refuses before a
privileged mutation. Recovery takes an exclusive ephemeral `recovery` claim role
for the selected stable adapter ID without editing persistent management or
device claims.

The helper prepares a root-owned recovery record before NetworkManager changes.
It creates and activates one volatile, helper-owned Ethernet connection on the
selected interface. IPv4 uses NetworkManager `link-local`; IPv6 is disabled;
autoconnect is false; default-route, automatic routes, automatic DNS, mDNS
route promotion, gateway, forwarding, NAT, DHCP server, DNS server, and router
advertisement are absent. Existing connections and profiles are never edited.

After a link-local address is observed, NILO adds the interface to an ephemeral
management-admission overlay and publishes `nilo.local` A/AAAA records only for
that interface through an owned Avahi entry group. Permanent DNS-SD remains on
the persistent management interfaces. Activation is complete only after
NetworkManager, firewall/socket admission, and alias publication are all
observed; otherwise restoration starts.

## Data / interface

Core sends typed activate, extend, and restore requests containing a recovery
record ID, stable adapter ID, expected current interface name and hardware
identity, deadline, and validated discovery/admission data. The helper
re-resolves and compares the live device before every mutation. Requests cannot
carry NetworkManager dictionaries, nftables text, responder text, commands,
paths, or secrets.

The volatile connection carries the recovery record UUID and exact stable
adapter, current interface and MAC binding. Restore removes it only when it is
still an in-memory profile and its normalized complete settings match the
profile NILO would create for that durable record: type, identity, interface,
MAC, autoconnect, IPv4LL, IPv6-disabled, `may-fail`, no-default-route,
no-automatic-route/DNS, and empty explicit address/gateway/route/DNS fields.
Normalization permits only NetworkManager's named false defaults, empty
permissions/address/route collections, and the generated empty `proxy` section;
any populated `proxy` property, other populated or unexpected property, or
other unexpected section is foreign. A foreign or modified object is a
fail-closed restoration failure, never authority to replace it.

## Examples

A newly connected, unclaimed `eth1` with no profile receives a probed
`169.254/16` address and answers `nilo.local`; authenticated HTTPS on the normal
management port works. `ip route` shows no default route from the recovery
connection and DHCP/DNS probes receive no server response.

If a static profile appears between selection and helper prepare, the helper's
fresh identity/profile check refuses and no recovery object is installed.

## Acceptance

- Eligibility is rechecked while holding the role coordinator and again at the
  privileged boundary.
- Exactly one volatile link-local connection, admission overlay, and scoped
  alias are installed.
- No existing profile, connection, address, route, DNS setting, or forwarding
  configuration is modified.
- Partial activation enters restoration and cannot be reported active.
- Startup drives any active, restoring, failed, or expired owned record through
  exact restoration before another hardware event is handled.

## Resolved design boundary

The helper exposes typed activate, extend, and restore operations and uses
NetworkManager D-Bus to create one in-memory owned profile. The core receives
the helper record identity even on partial activation failure so cleanup can
name the exact effect.
