# AcuGIS Report Server CLI

The official CLI and Python SDK live in [`sdk/python`](../../sdk/python).

**Full documentation:** [sdk/python/docs/cli.md](../../sdk/python/docs/cli.md)

## Quick start

```bash
cd sdk/python
pip install -e .
acugis-report login
acugis-report reports list
```

Architecture:

```text
acugis-report (CLI) → Python SDK → REST API → Report Server
```
