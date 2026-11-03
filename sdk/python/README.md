# acugis-report — Python SDK

Official Python client and CLI for [AcuGIS Report Server](https://acugis.com).

Package: **acugis-report** · Import: **acugis_report** · Version: **0.1.0** · Python **3.10+**

API contract: [`../openapi.bundle.yaml`](../openapi.bundle.yaml)

Product docs: MkDocs site under [`documentation/`](../../documentation/) · [Python SDK page](../../documentation/docs/sdk/python.md)

## Install

```bash
cd sdk/python
pip install -e ".[dev]"
acugis-report --help
```

## Quick start

```python
from acugis_report import ReportServer

client = ReportServer("https://reports.example.com", token="jrs_…")

me = client.auth.me()
reports = client.reports.list()
pdf, content_type = client.reports.run("Invoice", format="pdf", params={"city": "NYC"})
client.reports.run_to_file("Invoice", "invoice.pdf", format="pdf")

tree = client.folders.tree()
client.resources.upload("logo.png", folder_id=0)
client.health.check()
client.explorer.query({"datasource": "main", "sql": "SELECT 1"})
client.map_reports.run("Parcels", geometry="…", srid=4326)
client.embedded_maps.debug("Parcels")
client.datasources.list()
```

Flat methods remain available for backward compatibility (`client.list_reports()`, `client.run_report(…)`, etc.).

### Environment

| Variable | Purpose |
|----------|---------|
| `ACUGIS_REPORT_URL` | Base URL (used by CLI profiles / docs) |
| `ACUGIS_REPORT_TOKEN` | Bearer token |

## Service namespaces

| Property | Coverage |
|----------|----------|
| `auth` | Me, login/logout, PAT list/create/delete/rotate |
| `reports` | List/get/publish/update/delete/config/thumbnail, parameter lookup, `run` / `run_to_file` |
| `folders` | Tree, browse, search, CRUD, move, permissions |
| `resources` | List, upload, download, replace, delete, images |
| `datasources` | List, create, delete, context (`get`/`update` reserved) |
| `map_reports` | Features, extent, columns, run |
| `embedded_maps` | Debug |
| `explorer` | Datasources, tables, metadata, query, export, spatial, views, dashboard, view permissions |
| `health` | `/health`, `/ready`, diagnostics, Jasper status, doctor |
| `users` / `groups` / `schedules` / `jobs` / `recipients` | Reserved → `NotImplementedByServerError` |

## Errors

Typed errors derive from `ReportServerError`:

- `AuthenticationError` (401)
- `PermissionError` (403)
- `ValidationError` (400 / 422)
- `NotFoundError` (404)
- `ServerError` (5xx)
- `NetworkError`
- `NotImplementedByServerError`

## CLI

```bash
acugis-report --help
```

See [docs/cli.md](docs/cli.md) and the MkDocs [CLI overview](../../documentation/docs/cli/overview.md).

## Build & test

```bash
cd sdk/python
pip install -e ".[dev]"
pytest
```

## License

MIT — see [LICENSE](LICENSE).
