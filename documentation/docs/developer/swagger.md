# Swagger

Swagger UI is served by Report Server for interactive exploration of the live OpenAPI contract.

## Open Swagger UI

On a running server (include `JRS_BASE_PATH` when set):

```text
{origin}{JRS_BASE_PATH}/swagger
```

Example for the packaged default base path: `https://reports.example.com/swagger`.

When `JRS_BASE_PATH=/go`, the same UI is `https://reports.example.com/go/swagger`.

## Authorize

1. Open `/swagger`
2. Click **Authorize**
3. Enter a Bearer API Token (`jrs_…`) or use a session login flow where available
4. Try operations — binary downloads (PDF, CSV, images, ZIP) are supported

## Contract files

| Path | Purpose |
|------|---------|
| `/api/openapi.yaml` | OpenAPI 3.0 YAML |
| `/api/openapi.json` | OpenAPI 3.0 JSON |

See [OpenAPI](openapi.md) for generation, Postman, and stable `operationId` rules.

## Related

- [OpenAPI](openapi.md)
- [REST API overview](rest-api/overview.md)
- [REST reference](../reference/rest-api.md)
