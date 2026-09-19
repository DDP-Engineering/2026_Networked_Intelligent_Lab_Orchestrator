# REQ-PLAT-da89bf3b — Select an exact delivery target

## Behavior

Packaging commands load one repository-owned target registry and resolve a
target before loading its external-input lock. The registry is rejected as a
whole when its schema, target identity, OS metadata, architecture, artifact
set, or physical-acceptance checklist is malformed. Target selection never
uses a greater-than-or-equal OS comparison.

The initial registry contains exactly:

- `ubuntu-24.04-amd64`: Ubuntu `24.04`, `amd64`, `.deb` and `.run`;
- `rpi-debian13-arm64`: Raspberry Pi OS with Debian family/version `13`,
  `arm64`, `.deb`, `.run`, and `.img.xz`.

## Data / interface

`02_Tools/packaging/config/targets.json` is UTF-8 JSON:

```json
{
  "schema": 1,
  "targets": [
    {
      "id": "ubuntu-24.04-amd64",
      "os": {
        "id": "ubuntu",
        "family": "debian",
        "version_id": "24.04"
      },
      "architecture": "amd64",
      "artifacts": ["deb", "run"],
      "hardware_acceptance": [
        "boot-reboot",
        "integrations",
        "network",
        "services-gui",
        "usb-device-access"
      ]
    }
  ]
}
```

Rules:

- unknown top-level or target fields are errors;
- `schema` is the integer `1`;
- `targets` is a non-empty array, ordered by `id`, with unique IDs;
- IDs use lowercase ASCII letters, digits, dots, and hyphens;
- OS fields are non-empty strings;
- architecture is `amd64` or `arm64`;
- artifacts are unique, ordered values from `deb`, `run`, `img.xz`;
- hardware acceptance is the sorted canonical checklist for that exact target.

The CLI exposes `list-targets --targets <path>` and prints canonical JSON.
Adding `--target <id>` prints only that target. Validation errors go to stderr
and return exit code `2`.

## Examples

`list-targets --target ubuntu-24.04-amd64` reports Ubuntu `24.04` and `amd64`.
Selecting `ubuntu-25-amd64` returns exit code `2` even on a newer Ubuntu host.

## Acceptance

Both shipped definitions load deterministically. The registry must contain
both exact IDs, and each ID must retain its declared OS identity, family,
release, architecture, complete artifact set, and physical-acceptance
checklist. Unknown IDs, missing or duplicate entries, unsupported
architectures, reduced hardware checks, and an `img.xz` declaration on the
Ubuntu target are rejected before a lock is read.

## Assumptions and open decisions

The exact Raspberry Pi base-image release and digest are packaging inputs, not
target-registry fields, and remain intentionally absent until the lock is
completed.
