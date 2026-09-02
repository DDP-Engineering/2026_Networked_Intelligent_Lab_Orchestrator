# NILO Software

This directory contains the NILO Go Bench Core, browser GUI, language-neutral
integration contract, Python integration SDK, reference integrations, and the
verification entry points that exercise them together.

Increment 1 establishes the portable foundation and integration boundary. It
does not yet provide real hardware behavior, an MCP service, a connected
browser application, remote networking, appliance packaging, or PLC flashing;
the current HTTP service and GUI remain development foundations.

## Directory map

```text
05_Software/
├── nilo_gui/                React and TypeScript browser GUI
├── nilo_assistant/          Go Bench Core and `nilo` command
├── integration_sdk/
│   └── python/              Python integration authoring SDK
├── integrations/
│   └── fake_python/         Deterministic reference integration package
├── schemas/
│   └── integration/v1/      Public integration-package JSON schemas
├── scripts/                 Repository verification and portability scripts
├── tests/
│   └── contract/            Language-neutral package contract tests
└── INCREMENT_1_ACCEPTANCE.md
                             Acceptance scope, commands, and evidence
```

## `nilo_assistant/` — Go Bench Core

The Go module owns the trusted orchestration and persistence boundary. External
integrations run as supervised processes and communicate with the core through
`nilo.integration/1`; integration-specific Python code is not loaded into the
Go process.

Important subdirectories:

- `cmd/nilo/` — executable entry point: `nilo version`, the short-lived
  `serve-foundation-check` composition smoke command, the development token
  command, the simulation server, and the `nilo admin` local administration
  commands for the first administrator, users, and API tokens.
- `internal/app/` — constructs the foundation services and performs startup
  reconciliation.
- `internal/domain/` — immutable operation requests, UUID identities,
  artifacts, sessions, and allowed state transitions.
- `internal/config/` — process configuration, absolute data-directory rules,
  and platform-specific physical path confinement.
- `internal/auth/` — Argon2id password verifiers, the fixed role-to-permission
  matrix, browser session and API-token secrets, and the failed-login brake.
- `internal/identity/` — identities, roles, browser sessions, scoped API
  tokens, and the authorization decision shared by every interface. It gates
  which actions an actor may *request*; whether an operation may *run* without
  a human approval stays with `internal/policy/`.
- `internal/policy/` — Ask, Auto, and Full Access resolution, operation policy
  overrides, immutable policy snapshots, and request-bound approvals.
- `internal/operation/` — end-to-end operation lifecycle orchestration across
  policy, sessions, runners, artifacts, and audit evidence.
- `internal/session/` — session transitions, exclusive resource leases,
  cancellation, and interrupted-session reconciliation.
- `internal/audit/` — append-only structured audit-event contract.
- `internal/artifact/` — bounded, digest-verified, storage-confined artifact
  placement with Unix and Windows filesystem adapters.
- `internal/integration/manifest/` — validates install-shaped integration
  manifests, schemas, supported platforms, and executable-plus-argument runner
  declarations.
- `internal/integration/protocol/` — strict Go types, codecs, correlation, and
  lifecycle enforcement for `nilo.integration/1` NDJSON messages.
- `internal/integration/supervisor/` — starts and isolates runner processes,
  bounds diagnostics, coordinates cancellation and safe shutdown, and maps
  crashes or malformed output to typed failures.
- `internal/store/sqlite/` — durable SQLite connection configuration,
  digest-verified migrations, and repositories for configuration, operations,
  policy, sessions, leases, credentials, audit events, and artifacts.
- `internal/acceptance/` — black-box Increment 1 fixture using the real Go
  services, SQLite stores, supervisor, and fake Python child process.
- `internal/buildinfo/` — version and integration-protocol metadata reported by
  the command.

See `nilo_assistant/README.md` for the current command boundary.

## `nilo_gui/` - browser interface

The Windows-native GUI project uses React, TypeScript, and Vite. Its direct npm
dependencies are exact and the committed lockfile records every transitive
package and integrity digest. The current scaffold visibly identifies
simulation but deliberately makes no backend request; authentication and the
status/inventory client are the next vertical slice.

Use the repository-local Node.js toolchain for dependency restore, development,
formatting, linting, unit tests, and static production builds. See
`nilo_gui/README.md` for exact commands.

## `integration_sdk/python/` — Python integration SDK

This locked Python project is the supported authoring surface for Python
integration runners.

- `src/nilo_integration/api.py` defines the asynchronous lifecycle interface
  implemented by integrations.
- `src/nilo_integration/models.py` contains strict Pydantic models for protocol
  envelopes and payloads.
- `src/nilo_integration/server.py` provides the fail-closed NDJSON stdio
  server, event emission, request correlation, cancellation, and diagnostic
  separation.
- `tests/` verifies model strictness, every lifecycle method, event ordering,
  cancellation, and fatal stream behavior.
- `pyproject.toml` and `uv.lock` define the reproducible SDK development
  environment.

Generated `.venv-<platform>/` directories are ignored. Recreate the current
platform environment with `uv sync` through the repository-local tool wrapper
rather than committing it or sharing it between Windows and WSL.

## `integrations/fake_python/` — reference package

The fake integration is an install-shaped, deterministic package used to prove
the complete extension boundary before real hardware integrations are added.
It intentionally performs no network, shell, PLC, or instrument operation.

- `manifest.json` declares supported platforms, the external runner command,
  semantic operations, policies, resources, timeouts, events, and artifacts.
- `src/nilo_fake_integration/` implements `echo`, `artifact`, `sleep`, `fail`,
  and `crash`, plus the stdio console entry point.
- `schemas/` contains the operation input, result, event, progress, and artifact
  schemas referenced by the manifest.
- `tests/` verifies deterministic behavior and runner entry-point constraints.
- `pyproject.toml` and `uv.lock` pin the isolated package environment and its
  dependency on the local Python SDK.

This package is both an executable test fixture and the example to follow when
creating the first sigrok, electronics, or PLC integration package.

## `schemas/integration/v1/` — public integration contract

These JSON Schemas define the public, language-neutral package boundary:

- `manifest.schema.json` — package identity, platforms, runner declaration,
  operations, policies, resources, events, artifacts, and health checks.
- `operation.schema.json` — operation declaration shape.
- `event.schema.json` — event declaration shape.

The Go manifest loader embeds matching copies for portable runtime validation.
Changes to either copy must remain synchronized and preserve protocol-version
compatibility unless a deliberate new protocol version is introduced.

## `scripts/` — verification entry points

- `test-increment-1.ps1` and `test-increment-1.sh` run the complete foundation
  gate on Windows or Linux.
- `verify-portable-core.ps1` and `verify-portable-core.sh` run the Go suite and
  compile the portable command for Linux ARM64, Linux x86_64, and Windows
  x86_64 with CGO disabled.
- `aegis_test.py` discovers and routes the Go module, every locked Python
  project, and the Windows frontend project for full, impacted, and lint checks.
- `tests/` verifies the test router and repository-local toolchain contract.

The scripts use the pinned tools under `02_Tools`; no system Go, Python, uv,
Ruff, or pytest installation is required.

## `tests/contract/` — common package contract

The contract suite launches an integration through its declared executable and
argument array, then treats it as a black box. It verifies handshake,
description, health, strict messages, event ordering, exactly one terminal
response, cancellation, safe shutdown, artifacts, crashes, and stdout/stderr
separation.

- `test_integration_contract.py` is the reusable package-level suite.
- `fixtures/protocol-v1/` contains shared protocol examples.
- `README.md` documents focused package-contract commands.

Every future enabled integration package is expected to pass this common suite
in addition to its hardware-specific tests.

## How the folders relate

```text
public schemas ──► Go manifest/protocol validation
       │
       └────────► Python SDK models/server
                          │
                          └──► integration packages

Go Bench Core ──supervises──► external integration runner
       │
       └──persists──► SQLite metadata + confined artifact storage

scripts ──exercise──► Go core + SDK + fake package + contract suite
```

The direction is intentional: Bench Core depends on the public contract, not
on a particular hardware family or Python package. Hardware integrations can
be installed and evolved without recompiling the core.

## Common commands

From the repository root on Windows:

```powershell
.\02_Tools\bootstrap\bootstrap.ps1
.\02_Tools\env\with-tools.ps1 go -C .\05_Software\nilo_assistant run .\cmd\nilo version
.\05_Software\scripts\test-increment-1.ps1
```

On Linux ARM64 or Linux x86_64:

```sh
./02_Tools/bootstrap/bootstrap.sh
./02_Tools/env/with-tools.sh go -C ./05_Software/nilo_assistant run ./cmd/nilo version
./05_Software/scripts/test-increment-1.sh
```

See `INCREMENT_1_ACCEPTANCE.md` for the exact acceptance scope, observed host
evidence, known skips, and the mandatory native Raspberry Pi 4 ARM64 gate.

## Platform status

- **Linux ARM64:** first appliance and release target.
- **Linux x86_64:** later appliance target.
- **Windows x86_64:** development host and portable compile target, not a NILO
  appliance delivery target.
