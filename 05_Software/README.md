# NILO software

`05_Software` is organized by ownership. The Go Bench Core remains one module,
while the browser, external integrations, public contracts, and cross-component
tests have their own roots. Repository setup, build, packaging, and diagnostic
tools live in `02_Tools`.

```text
05_Software/
  frontend/                 React, TypeScript, Vite, and browser tests
  core/                     Go module: nilo CLI, HTTPS/MCP services, helper, persistence
  integrations/
    sdk/python/             Python runner authoring SDK
    sigrok/                 Current curated instrument integration
  contracts/integration/v1/ Public integration-package JSON schemas
  tests/
    contract/               Black-box runner contract suite
    fixtures/contract_runner/ Test-only Go runner and package declaration
  INCREMENT_1_ACCEPTANCE.md
```

## Component boundaries

The [core](core/README.md) owns identity, authorization, policy, integration
supervision, jobs, artifacts, networking, audit, SQLite, and the `nilo` and
`nilo-nethelper` commands. The browser [frontend](frontend/README.md) uses the
core's authenticated HTTP API. It does not access devices, integration processes,
or the database directly.

External packages use `nilo.integration/1`: correlated NDJSON over a supervised
child process. The [public schemas](contracts/integration/v1/) describe package
manifests and declarations; matching embedded copies in
`core/internal/integration/manifest/schemas/` are used at runtime. The
[Python SDK](integrations/sdk/python/README.md) supplies models and a stdio
server for Python integrations. The [Sigrok package](integrations/sigrok/) is the
current instrument adapter. `core/internal/bench/` owns its typed admission and
execution path.

The [contract suite](tests/contract/README.md) builds and launches a test-only
Go runner from `tests/fixtures/contract_runner/`. That fixture is not an
installable product integration and is excluded from appliance packages.
Package-specific tests remain next to their component; the cross-component
suite covers the shared process boundary.

## Verification and development

Repository-local toolchains and verification entry points live under
`02_Tools/`. From the repository root, use:

```powershell
.\02_Tools\env\with-tools.ps1 go -C .\05_Software\core run .\cmd\nilo version
.\02_Tools\scripts\test-increment-1.ps1
```

On Linux:

```sh
./02_Tools/env/with-tools.sh go -C ./05_Software/core run ./cmd/nilo version
./02_Tools/scripts/test-increment-1.sh
```

See [Development and Simulation Flow](../06_SoftwareDocu/Development_and_Simulation_Flow.md)
for the Windows browser and WSL2 backend topology, and
[Increment 1 acceptance](INCREMENT_1_ACCEPTANCE.md) for the foundation gate.
