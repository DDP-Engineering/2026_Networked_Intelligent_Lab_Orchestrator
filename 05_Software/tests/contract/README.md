# NILO integration contract kit

The contract kit exercises an install-shaped runner through newline-delimited
JSON. It launches a supplied executable plus an independent argument array
with no shell, reads protocol records only from stdout, and treats stderr as
diagnostics. The deterministic Go package under `tests/fixtures/contract_runner`
is built into a temporary directory for these tests. It has no hardware or
network dependency; its artifact operation writes only into an explicit
temporary staging directory.

Run the contract suite from the repository root with the locked SDK test environment:

```powershell
.\02_Tools\env\with-tools.ps1 uv run --project 05_Software/integrations/sdk/python --all-groups --locked pytest -q 05_Software/tests/contract/test_integration_contract.py
```
