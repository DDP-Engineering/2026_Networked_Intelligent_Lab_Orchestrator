# REQ-FUNC-8d482c89 - Run a Wi-Fi commissioning access point

## Behavior

Provisioning selects at most one radio in stable adapter-identity order. It must
be present, management-eligible, unclaimed by protected roles, have no saved or
active client profile and support WPA2 AP mode. Ethernet reachability does not
suppress the AP. No candidate is a supported unavailable state.

The helper owns a volatile NetworkManager profile and ownership record. It uses
SSID equal to serial name, WPA2-PSK with the configured published first-boot
password, gateway `192.168.247.1/24`, and no uplink sharing/NAT. It never edits,
disconnects or adopts a foreign profile. Commissioning stops captive services,
removes admission, and deletes only owned state; reset/restart reconciles it
back. Partial start rolls back owned pieces in reverse order.

## Data / interface

The closed helper plan carries serial SSID, reference to the installed
published-password source, stable adapter identity/name, gateway/prefix and
lifecycle—no arbitrary settings or plaintext password response. Safe status is:

```json
{"state":"active|unavailable|stopping","ssid":"nilo-4j7m2q9wxs",
 "adapter_id":"...","address":"192.168.247.1/24","reason":"..."}
```

Reason codes include `no_eligible_radio`, `route_overlap` and
`helper_unavailable`. Any current IPv4 route overlapping the subnet prevents
profile and captive-service activation.

## Examples

Saved client state on `wlan0` plus unused `wlan1` selects only `wlan1`.
Working Ethernet does not remove the AP. A conflicting route reports overlap
and starts nothing.

## Acceptance

- Selection is deterministic and role/profile aware.
- State follows provisioning, commissioning, reset and restart.
- Foreign/saved profiles remain byte-for-byte unchanged.
- Passphrase appears only at the protected helper input.
- Phone acceptance joins, reaches HTTPS, observes teardown and reset recovery.

## Assumptions and open decisions

Design must confirm AP driver support and add the helper-owned dnsmasq runtime
dependency without widening service scope outside provisioning.
