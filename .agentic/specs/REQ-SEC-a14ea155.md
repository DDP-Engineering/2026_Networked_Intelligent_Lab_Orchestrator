# REQ-SEC-a14ea155 — Exclude secrets and development fixtures from releases

## Behavior

The release scanner evaluates extracted payload inventories and bounded file
content without echoing secret values. A forbidden path, marker or undeclared
executable prevents acceptance and names only the artifact and safe rule ID.

## Data / interface

`config/forbidden-release-content.json` is a reviewed allowlist-free policy of
path globs and byte markers for development credentials, `.git`, editor files,
cache/staging state, simulation packages, `fake_python`, host absolute paths,
private/signing keys and undeclared executables. Expected production keys and
certificates are generated on the appliance, never packaged. Scans are size
bounded and stream files.

## Examples

Literal `admin_password`, a Windows checkout path, `.git/config`, Python cache
and `integrations/fake_python` each fail with a safe rule name. A third-party
notice mentioning the word "test" does not fail unless its path/content matches
a reviewed rule.

## Acceptance

Positive and one-rule-at-a-time fixtures prove detection and secret-free
diagnostics. Every executable payload member is referenced by package or
integration manifests.

## Assumptions and open decisions

This scanner complements input provenance and human licence review; it is not a
general malware or secret-scanning guarantee.
