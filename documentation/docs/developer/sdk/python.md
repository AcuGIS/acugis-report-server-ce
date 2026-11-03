# Python SDK

Package: `acugis-report` (import `acugis_report`) · Repository path: [`sdk/python/`](https://github.com/AcuGIS/acugis-report-server-ce/tree/main/sdk/python).

The CLI is a thin wrapper around the same client.

## Installation

```bash
cd sdk/python
pip install -e ".[dev]"
```

## Authentication

```python
client.login(username, password)
client.create_token("CI Server", expires_in="90d")
client.list_tokens()
client.rotate_token(token_id)
client.delete_token(token_id, purge=False)
```

Use a Bearer API Token via `ACUGIS_REPORT_TOKEN` for non-interactive clients. See [Developer authentication](../authentication.md).

## Creating a client

```python
import os
from acugis_report import ReportServer

with ReportServer(
    url=os.environ["ACUGIS_REPORT_URL"],
    token=os.environ["ACUGIS_REPORT_TOKEN"],
) as client:
    me = client.auth.me()
    print(me.username, me.groups)
```

### Service namespaces

Prefer namespaced APIs so examples stay aligned with .NET, Java, and Go:

| Property | Examples |
|----------|----------|
| `auth` | `client.auth.me()`, tokens |
| `reports` | `list()`, `run()`, `run_to_file()`, publish/update |
| `folders` | `tree()`, browse, CRUD |
| `resources` | `upload()`, download, replace |
| `datasources` | `list()`, create, delete |
| `map_reports` | `run()`, features, extent |
| `embedded_maps` | `debug()` |
| `explorer` | `query()`, views, spatial |
| `health` | `check()`, ready, doctor |
| `users` / `groups` / `schedules` / `jobs` / `recipients` | Reserved |

### Flat helpers

Backward-compatible flat methods remain, including:

| Method | REST |
|--------|------|
| `list_reports()` | `GET /api/v1/reports` |
| `get_report(key)` | `GET /api/v1/reports/{key}` |
| `publish_report(...)` | `POST /api/v1/reports` |
| `run_report(key, format=..., params=...)` | `/reports/execute` |
| `run_to_file(key, path, ...)` | execute + write file |
| `folders_tree()` | `GET /api/v1/folders/tree` |
| `list_resources()` | `GET /api/v1/resources` |
| `list_datasources()` | `GET /api/v1/datasources` |
| `map_features(key)` | `GET /api/v1/map/{key}/features`. Requires Report Server Professional. The Open Source server does not implement this route. |
| `doctor()` | Aggregated health checks |

## Listing reports

```python
items = client.reports.list()
```

## Running reports

```python
pdf, content_type = client.reports.run(
    "Invoice",
    format="pdf",
    params={"InvoiceID": 123},
)
client.reports.run_to_file("Invoice", "invoice.pdf", format="pdf", params={"InvoiceID": 123})
```

## Uploading reports

```python
client.reports.publish(...)  # multipart JRXML publish
# or flat helper:
client.publish_report(...)
```

## Downloading reports

Binary execute results are returned as bytes (or written with `run_to_file`). Thumbnails and resource downloads use the corresponding service methods — save bytes to a file; do not parse them as JSON. See [Binary responses](../rest-api/binary-responses.md).

## Error handling

Typed errors include authentication, validation, not found, permissions, server, and network failures. The CLI maps these to exit codes 2–7.

## Environment variables

```bash
export ACUGIS_REPORT_URL=https://reports.example.com
export ACUGIS_REPORT_TOKEN=jrs_xxxxxxxxx
```

## Common patterns

- Prefer namespaced `client.reports.*` APIs for cross-language alignment
- Use `run_to_file` for binary outputs
- Map typed errors to your framework’s exception handlers

## Related

- [CLI overview](../cli/overview.md)
- [Python FastAPI tutorial](../tutorials/python-integration.md)
- [sdk/python README](https://github.com/AcuGIS/acugis-report-server-ce/tree/main/sdk/python)
