# REST API reference

Lookup tables for Report Server HTTP endpoints. For concepts and how-to guidance, see the [REST API overview](../developer/rest-api/overview.md). The live contract is OpenAPI — prefer `/api/openapi.yaml` on a running server.

## Auth

| Method | Path | Purpose |
|--------|------|---------|
| `POST` | `/api/v1/auth/login` | Session login |
| `POST` | `/api/v1/auth/logout` | Logout |
| `GET` | `/api/v1/auth/me` | Current user |
| `GET` | `/api/v1/auth/tokens` | List token metadata |
| `POST` | `/api/v1/auth/tokens` | Create token |
| `PUT` | `/api/v1/auth/tokens/{id}` | Update token |
| `DELETE` | `/api/v1/auth/tokens/{id}` | Revoke |
| `POST` | `/api/v1/auth/tokens/{id}/rotate` | Rotate |

## Reports

| Method | Path |
|--------|------|
| `GET` | `/api/v1/reports` |
| `GET` | `/api/v1/reports/{key}` |
| `POST` | `/api/v1/reports` (multipart) |
| `PUT` | `/api/v1/reports/{key}` (multipart) |
| `DELETE` | `/api/v1/reports/{key}` |
| `GET` | `/api/v1/reports/{key}/config` |
| `GET` | `/api/v1/reports/{key}/thumbnail` |
| `GET`/`POST` | `/api/v1/reports/{key}/parameters/{name}/lookup` |
| `GET`/`POST` | `/reports/execute` |

## Folders

| Method | Path |
|--------|------|
| `GET` | `/api/v1/folders/tree` |
| `GET` | `/api/v1/folders/browse?folder_id=` |
| `GET` | `/api/v1/folders/search?q=` |
| `POST` | `/api/v1/folders` |
| `GET` | `/api/v1/folders/{id}` |
| `PUT` | `/api/v1/folders/{id}` |
| `DELETE` | `/api/v1/folders/{id}` |
| `POST` | `/api/v1/folders/{id}/move` |
| `POST` | `/api/v1/folders/{id}/duplicate` |
| `GET`/`PUT` | `/api/v1/folders/{id}/permissions` |
| `POST` | `/api/v1/content/move` |

## Resources

| Method | Path |
|--------|------|
| `GET` | `/api/v1/resources` |
| `POST` | `/api/v1/resources` |
| `GET` | `/api/v1/resources/{uuid}` |
| `PUT` | `/api/v1/resources/{uuid}` |
| `PUT` | `/api/v1/resources/{uuid}/content` |
| `GET` | `/api/v1/resources/{uuid}/download` |
| `DELETE` | `/api/v1/resources/{uuid}` |
| `GET` | `/api/v1/resources/{uuid}/thumbnail` |
| `GET` | `/api/v1/resources/images` |

## Datasources

| Method | Path | Notes |
|--------|------|-------|
| `GET` | `/api/v1/datasources` | Names only |
| `POST` | `/api/v1/datasources` | Form upsert |
| `DELETE` | `/api/v1/datasources/{id}` | |
| `POST` | `/api/v1/datasources/context` | Jasper context |

## Embedded Maps

| Method | Path |
|--------|------|
| `GET`/`POST` | `/api/v1/embedded-map/debug` |

## Map Reports (Professional)

These routes are not implemented by the Open Source server. They require Report Server Professional.

| Method | Path |
|--------|------|
| `GET` | `/api/v1/map/{key}/features` |
| `GET` | `/api/v1/map/{key}/extent` |
| `GET` | `/api/v1/map/{key}/columns` |

## Explorer

Registered on the Professional binary only. Data Explorer calls are allowed before a license is activated. Dashboard widget calls require a valid Report Server Professional license. The Open Source package does not mount these paths.

| Method | Path |
|--------|------|
| `GET` | `/api/v1/explorer/datasources` |
| `GET` | `/api/v1/explorer/tables` |
| `GET` | `/api/v1/explorer/metadata` |
| `POST` | `/api/v1/explorer/query` |
| `POST` | `/api/v1/explorer/report-query` |
| `POST` | `/api/v1/explorer/features` |
| `POST` | `/api/v1/explorer/export` |
| `POST` | `/api/v1/explorer/summary` |
| `POST` | `/api/v1/explorer/statistics` |
| `POST` | `/api/v1/explorer/spatial-analysis` |
| `GET`/`POST` | `/api/v1/explorer/views` … |
| `GET` | `/api/v1/explorer/dashboard-widgets` |

## Probes

| Path | Purpose |
|------|---------|
| `/health` | Liveness |
| `/ready` | Readiness |
| `/api/v1/diagnostics` | Diagnostics |
| `/swagger` | Swagger UI |
| `/api/openapi.yaml` | OpenAPI YAML |
| `/api/openapi.json` | OpenAPI JSON |

## Related

- [OpenAPI](openapi.md)
- [Error codes](error-codes.md)
- [Developer REST overview](../developer/rest-api/overview.md)
