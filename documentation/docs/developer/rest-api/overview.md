# REST API overview

Report Server exposes a versioned REST API under `/api/v1/*`, plus the Jasper execute gateway and live OpenAPI documents.

## Live documentation (server-relative)

On a running Report Server (include `JRS_BASE_PATH` when set, for example `/go`):

| Path | Purpose |
|------|---------|
| `/swagger` | Swagger UI |
| `/api/openapi.yaml` | Bundled OpenAPI 3.0 YAML (`info.version` = `JRS_APP_VERSION`) |
| `/api/openapi.json` | Same document as JSON |

## Major areas

| Tag / area | Examples |
|------------|----------|
| Auth | login, me, API Tokens |
| Reports | list/get/create/update/delete, config, thumbnail, parameter lookup |
| Execute | `/reports/execute` binary output |
| Folders | tree, browse, permissions |
| Resources | upload, download, images |
| Datasources | list names, create, delete, context |
| Map Reports | features, extent, columns. Professional binary and a valid license. |
| Embedded Maps | debug |
| Explorer | query, statistics, spatial-analysis, views, export. Professional binary. Allowed before license activation. |
| Dashboards | widget flags on Explorer views. Professional binary and a valid license. |
| Diagnostics | `/api/v1/diagnostics`, `/health`, `/ready` |

## Authentication

Most `/api/v1/*` routes accept either:

- `Authorization: Bearer jrs_…`
- Session cookie `jrs_session`

Health, ready, diagnostics, edition, and login are public. See [API authentication](authentication.md).

## Design notes

- Prefer `/api/v1/*` over deprecated PHP-shaped aliases
- Datasource mutations use form encoding today
- Execute/image gateway errors are often `text/plain`
- Some admin surfaces (users, groups, schedules, GeoServer/ArcGIS connections) remain HTML-only

## Related

- [OpenAPI](../openapi.md)
- [Swagger](../swagger.md)
- [REST reference](../../reference/rest-api.md)
- [Errors](../../reference/error-codes.md)
- [Binary responses](binary-responses.md)
- [Compatibility](compatibility.md)
