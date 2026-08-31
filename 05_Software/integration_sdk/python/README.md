# NILO Python integration SDK

The SDK is the Python authoring surface for an isolated NILO integration runner.
It uses strict Pydantic v2 models for `nilo.integration/1` and newline-delimited
JSON on standard input/output.

Implement `nilo_integration.Integration`, then run it behind `ProtocolServer`.
Production runner entry points should connect the server to stdin and reserve
stdout exclusively for protocol records. Use `EventSink.emit()` to send execute
events; ordinary `print()` calls made by an integration are redirected to stderr
during a request.

An integration handler owns any asyncio tasks it creates. It must await or
cancel them before returning; a runner can only reliably tear down the direct
handler task. Task 13 adds the install-shaped runner boundary that enforces this
authoring contract during process shutdown.

For development:

The install-shaped fake package in `05_Software/integrations/fake_python` and
the generic suite in `05_Software/tests/contract` are the reference authoring
examples. Use the repository-level Increment 1 scripts for the full locked
SDK, package, protocol, and Go-supervisor gate.

```powershell
uv sync --all-groups
uv run pytest -q
uv run ruff check .
uv run ruff format --check .
```
