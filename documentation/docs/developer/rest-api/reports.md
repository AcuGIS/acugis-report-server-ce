# Reports (REST)

Endpoint inventory for the report catalog and execute gateway. Product concepts: [Reports](../../user-guide/reports.md).

## Catalog

| Operation | Method | Path |
|-----------|--------|------|
| List | `GET` | `/api/v1/reports` |
| Get | `GET` | `/api/v1/reports/{key}` |
| Create | `POST` | `/api/v1/reports` (multipart) |
| Update | `PUT` | `/api/v1/reports/{key}` (multipart) |
| Delete | `DELETE` | `/api/v1/reports/{key}` |
| Config | `GET` | `/api/v1/reports/{key}/config` |
| Thumbnail | `GET` | `/api/v1/reports/{key}/thumbnail` |
| Parameter lookup | `GET`/`POST` | `/api/v1/reports/{key}/parameters/{name}/lookup` |

## Execute

| Method | Path |
|--------|------|
| `GET` / `POST` | `/reports/execute` |

See [Parameters & execute](parameters.md) and [Binary responses](binary-responses.md).

## Map Reports (Professional)

These routes are not implemented by the Open Source server. They require Report Server Professional.

| Operation | Method | Path |
|-----------|--------|------|
| Features (GeoJSON) | `GET` | `/api/v1/map/{key}/features` |
| Extent → params | `GET` | `/api/v1/map/{key}/extent` |
| Columns | `GET` | `/api/v1/map/{key}/columns` |

## Embedded Maps

```http
GET/POST /api/v1/embedded-map/debug
```

OperationId: `debugEmbeddedMap`.

## Related

- [REST reference](../../reference/rest-api.md)
- [OpenAPI](../openapi.md)
