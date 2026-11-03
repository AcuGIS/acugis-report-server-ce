# Developer Guide

Integrate with AcuGIS Report Server using the REST API, CLI, and official SDKs (Python, .NET, Java, Go).

This guide is for developers building applications, automations, and CI pipelines. For product concepts (what reports, folders, and Explorer are), start with the [User Guide](../user-guide/index.md). For lookup tables, use [Reference](../reference/rest-api.md).

## Architecture

```text
Your app / CI
    │
    ├─ SDK (Python / .NET / Java / Go)
    ├─ CLI (acugis-report)
    └─ REST (curl / OpenAPI clients)
            │
            ▼
        Report Server REST API + execute gateway
```

## Start here

| Goal | Page |
|------|------|
| Authenticate | [Authentication](authentication.md) |
| Understand REST | [REST API overview](rest-api/overview.md) |
| Use the CLI | [CLI overview](cli/overview.md) |
| Pick an SDK | [SDK overview](sdk/overview.md) |
| OpenAPI / Swagger | [OpenAPI](openapi.md) · [Swagger](swagger.md) |
| Embed in an app | [Embedded analytics](embedded-analytics.md) |
| Workflow tutorials | [Tutorials](tutorials/publishing-a-report.md) |

## Client environment

```bash
export ACUGIS_REPORT_URL=https://reports.example.com
export ACUGIS_REPORT_TOKEN=jrs_xxxxxxxxx
```

## Live contract

| Path | Purpose |
|------|---------|
| `/swagger` | Swagger UI |
| `/api/openapi.yaml` | OpenAPI 3.0 YAML |
| `/api/openapi.json` | OpenAPI 3.0 JSON |

Include `JRS_BASE_PATH` when set (for example `/go/swagger`).
