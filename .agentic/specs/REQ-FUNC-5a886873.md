# REQ-FUNC-5a886873 - Announce management presence through SSDP

## Behavior

The helper joins IPv4 SSDP `239.255.255.250:1900` separately on each effective
management interface and binds outbound packets/responses to that interface.
Admission sends alive, normal operation refreshes before expiry, and removal or
orderly shutdown sends byebye. Valid searches for `ssdp:all` and
`upnp:rootdevice` receive one jittered unicast response on ingress. Malformed or
unsupported searches are ignored with rate-bounded behavior.

## Data / interface

```text
NT/ST: upnp:rootdevice
USN: uuid:<appliance-uuid>::upnp:rootdevice
LOCATION: <interface-reachable description URL>
CACHE-CONTROL: max-age=120
SERVER: NILO/<version> UPnP/1.1
BOOTID.UPNP.ORG: <positive boot id>
CONFIGID.UPNP.ORG: <positive description generation>
```

Search requires `MAN: "ssdp:discover"`; `MX` is clamped to 1-5 seconds. The
helper receives validated UUID, version, port/path and interfaces and constructs
wire bytes. IPv6 may be added only with equal scoping; IPv4 is required.

The presentation URL is always HTTPS. Prototype whether Windows 11 accepts a
self-signed HTTPS `LOCATION`; if not, add only an interface-scoped read-only
HTTP description listener while retaining the HTTPS presentation URL.

## Examples

Removing `eth1` sends byebye and drops only that membership while `eth0`
continues. A search on a device-network bridge receives no response.

## Acceptance

- Packet tests settle headers, CRLF framing, UUID, jitter and rate limits.
- Membership/alive/refresh/byebye track management changes and shutdown.
- Excluded-interface captures contain no NILO SSDP packet.
- Windows appearance/open/disappearance is measured within 60 seconds.

## Assumptions and open decisions

WS-Discovery stays excluded unless Windows acceptance proves SSDP insufficient.
The `LOCATION` scheme is recorded from the early Windows probe.
