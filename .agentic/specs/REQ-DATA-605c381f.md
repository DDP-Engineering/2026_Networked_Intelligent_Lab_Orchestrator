# REQ-DATA-605c381f - Derive a permanent printable appliance name

## Behavior

The privileged helper owns `/var/lib/nilo-nethelper/appliance-identity.json`
below a root-owned mode-`0700` directory; the atomically replaced record is
mode `0600`. A valid existing record is authoritative and is never recomputed
during boot or network commissioning reset.

Without a record, sources are tried in order: Raspberry Pi device-tree serial
at `/sys/firmware/devicetree/base/serial-number` then
`/proc/device-tree/serial-number`; x86_64 DMI product serial at
`/sys/class/dmi/id/product_serial`; then 16 cryptographically random bytes
persisted as a lowercase 32-character hex fallback seed. Source text has
trailing NULs and surrounding whitespace removed and is lowercased. Empty,
all-zero/all-`f`, `none`, `unknown`, `default string`, `system serial number`
and `to be filled by o.e.m.` are unusable. Raw hardware text is not persisted
or returned.

## Data / interface

Version 1 is exact:

```text
digest = SHA-256("nilo-appliance-name" || NUL || "v1" || NUL ||
                 source-kind || NUL || normalized-source)
short-id = lowercase Crockford base32 of the first 50 digest bits
alphabet = 0123456789abcdefghjkmnpqrstvwxyz
serial-name = "nilo-" || short-id
```

`source-kind` is `rpi-board`, `dmi-product` or `fallback`. The closed JSON
record carries `schema`, `serial_name`, `source_kind`, and `fallback_seed` only
for a fallback. Unknown versions/keys, invalid names, and inconsistent fallback
state are refused rather than regenerated. The public helper response omits the
source value and fallback seed.

## Examples

The same Pi serial yields the same name after reimaging. Placeholder DMI text
selects one stored fallback. Hardware evidence disappearing after initialization
does not change the recorded name.

## Acceptance

- Golden vectors fix normalization, framing, bit order and alphabet.
- Distinct valid sources are distinct in the test corpus.
- Restart and commissioning reset preserve the record.
- The fallback is durable before its name is exposed and is generated once.
- No public or diagnostic surface contains raw source text or fallback seed.

## Assumptions and open decisions

The public serial is not an authentication token. Collision handling never
suffixes or rewrites it; an observed collision is a diagnostic/manufacturing
fault. Delivery-image default hostnames are settled by the hostname design.
