# NILO integration contract kit

The contract kit exercises an install-shaped runner through newline-delimited
JSON. It launches a supplied executable plus an independent argument array
with no shell, reads protocol records only from stdout, and treats stderr as
diagnostics. The deterministic `fake_python` package is the first fixture: it
has no network, shell, hardware, or repository-path dependency outside the
temporary staging directory explicitly supplied to its artifact operation.

Run the fake package checks from its package directory:

```powershell
uv run pytest -q
uv run ruff check .
uv run ruff format --check .
```
