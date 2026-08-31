# REQ-FUNC-1d02a85d - Load runtime integration packages

## Behavior

An integration is an external-process package installed independently of Bench Core. Enabling a compatible package validates its manifest and local schemas before the supervisor launches its declared runner. Installing or upgrading a package does not recompile Bench Core.

## Data / interface

Each package contains `manifest.json`, referenced schemas, a runner entry point, optional profiles and normalizers, contract fixtures, and documentation. The manifest is strict JSON with `additionalProperties: false`, is limited to 1 MiB, and declares:

- package identifier, semantic version, publisher, and integrity metadata;
- supported `nilo.integration/1` protocol range;
- supported operating-system and CPU-architecture pairs;
- runner executable and a separate argument array;
- application configuration and secret schemas;
- operation descriptors with input/result schemas and default policy;
- resource claims, timeout bounds, cancellation, and safe-shutdown support;
- event, measurement, error, and artifact declarations;
- health checks and contract fixtures.

All referenced paths are relative to the package root and are resolved with a component-aware path check. Remote schema references, absolute runner paths, path escapes, concatenated command strings, duplicate operation IDs, unsupported platforms, and incompatible protocol ranges are rejected.

## Examples

```json
{
  "package_id": "nilo.fake",
  "version": "0.1.0",
  "protocol": ">=1.0.0 <2.0.0",
  "platforms": [{"os": "windows", "arch": "amd64"}],
  "runner": {
    "executable": "python",
    "arguments": ["fake_integration.py"]
  },
  "operations": []
}
```

The equivalent runner value `"python fake_integration.py"` is invalid because it is an arbitrary command string rather than an executable plus argument array.

## Acceptance

Fixture tests load a complete compatible package and reject missing fields, unknown fields, unsafe paths, remote references, duplicate operations, incompatible protocol/platform declarations, and raw command strings before any process starts.
