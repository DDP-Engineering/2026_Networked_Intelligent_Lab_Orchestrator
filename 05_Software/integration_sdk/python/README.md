# NILO Python integration SDK

The SDK is the Python authoring surface for an isolated NILO integration runner.
It uses strict Pydantic v2 models for `nilo.integration/1` and newline-delimited
JSON on standard input/output.

Implement `nilo_integration.Integration`, then run it behind `ProtocolServer`.
Production runner entry points should connect the server to stdin and reserve
stdout exclusively for protocol records. Use `EventSink.emit()` to send execute
events; ordinary `print()` calls made by an integration are redirected to stderr
during a request.

For development:

```powershell
uv sync --all-groups
uv run pytest -q
uv run ruff check .
uv run ruff format --check .
```
