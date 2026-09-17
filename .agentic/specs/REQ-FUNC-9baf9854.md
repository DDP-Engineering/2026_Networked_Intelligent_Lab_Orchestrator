# REQ-FUNC-9baf9854 - Discover appliances from the command line

## Behavior

`nilo discover` enumerates every eligible up non-loopback interface and runs
mDNS/DNS-SD plus SSDP concurrently for five seconds. `--timeout` accepts 1-60
seconds. One protocol/interface failure is a bounded diagnostic; no result is a
successful empty set, but inability to start either protocol anywhere is
`discovery_unavailable`.

Records merge only by canonical UUID. Conflicting shared fields are diagnosed
and unsafe URLs omitted rather than guessed. Addresses de-duplicate with IPv6
zone where required. Friendly name is display only; serial then UUID sorts.

## Data / interface

```text
nilo discover [--timeout 5s] [--json] [--label <serial-or-friendly-name>]
```

Table columns are `NAME SERIAL STATE VERSION URL ADDRESSES`. JSON has schema 1,
`duration_ms`, `appliances` and bounded `diagnostics`. Each appliance contains
`id`, `name`, `serial`, optional `friendly_name`, sorted `addresses`, safe
`url`, `state` (`provisioning|commissioned`), `version`, and sorted
`sources` (`mdns`, `ssdp`). JSON stdout contains no human commentary.

## Examples

Same-UUID mDNS and SSDP become one result. Two appliances sharing a friendly
name remain separate and later single-target commands refuse ambiguity. A URL
disagreement is omitted and diagnosed.

## Acceptance

- Injectable Windows/macOS/Linux socket tests need no external executable.
- Fake time settles deadlines and late-packet exclusion.
- Golden table/JSON tests settle schema, sorting, zones and merge.
- Direct-cable acceptance measures discovery on all three platforms.
- Output contains no secret or raw hardware serial.

## Assumptions and open decisions

Use raw standard sockets to keep the CLI cross-platform. Design must prototype
Windows multicast interface enumeration with the pinned Go runtime.
