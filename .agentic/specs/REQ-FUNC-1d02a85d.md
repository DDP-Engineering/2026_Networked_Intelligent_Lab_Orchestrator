# REQ-FUNC-1d02a85d - Load runtime integration packages

## Behavior

An integration is an external-process package installed independently of Bench Core. Enabling a compatible package validates its manifest and local schemas before the supervisor launches its declared runner. Installing or upgrading a package does not recompile Bench Core.

## Data / interface

Each package contains `manifest.json`, referenced schemas, a runner entry point, optional profiles and normalizers, contract fixtures, and documentation. The manifest is strict JSON with `additionalProperties: false`, is limited to 1 MiB, and declares:

- package identifier, version, publisher, and integrity metadata;
- the `nilo.integration/1` protocol;
- supported operating-system and CPU-architecture pairs;
- runner executable and a separate argument array;
- an application configuration schema and an optional device configuration schema;
- operation descriptors with input/result schemas and default policy;
- resource claims, timeout bounds, cancellation, and safe-shutdown support;
- event and artifact declarations;
- health checks and contract fixtures.

All referenced paths are relative to the package root and are resolved with a component-aware path check. Remote schema references, absolute runner paths, path escapes, concatenated command strings, duplicate operation IDs, unsupported platforms, and incompatible protocol ranges are rejected.

## Examples

```json
{
  "package": {
    "id": "example",
    "version": "0.1.0",
    "publisher": "DDP Engineering",
    "integrity": "sha256:0000000000000000000000000000000000000000000000000000000000000000"
  },
  "protocol": "nilo.integration/1",
  "platforms": [{"os": "linux", "architecture": "amd64"}],
  "runner": {"executable": "runner", "arguments": ["--stdio"]},
  "app_configuration_schema": "schemas/config.schema.json",
  "operations": [{
    "id": "echo",
    "policy": "auto_eligible",
    "input_schema": "schemas/echo.input.schema.json",
    "result_schema": "schemas/echo.result.schema.json",
    "timeout": "5s",
    "cancellable": true,
    "state_changing": false,
    "resources": []
  }],
  "cancellation": {"supported": true, "safe_shutdown": "finish-current"},
  "events": [],
  "artifacts": [],
  "health_checks": [],
  "contract_fixtures": []
}
```

This example has the current schema's required shape. A loadable package must
also contain the referenced schemas, executable runner and matching integrity
inventory. The equivalent runner value `"runner --stdio"` is invalid because
it is a command string rather than an executable plus argument array.

## Acceptance

Fixture tests load a complete compatible package and reject missing fields, unknown fields, unsafe paths, remote references, duplicate operations, incompatible protocol/platform declarations, and raw command strings before any process starts.
