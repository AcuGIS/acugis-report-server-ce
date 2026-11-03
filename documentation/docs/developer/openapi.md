# OpenAPI

The OpenAPI 3.0 document is the authoritative contract for SDKs, the CLI, Swagger UI, ReDoc, and Postman.

## Where to get it

| Source | Location |
|--------|----------|
| Live YAML | `/api/openapi.yaml` |
| Live JSON | `/api/openapi.json` |
| Swagger UI | `/swagger` |
| Repo bundle | `sdk/openapi.bundle.yaml` |
| Modular sources | `go-report-server/openapi/` |

With `JRS_BASE_PATH=/go`, prefix paths accordingly (`/go/swagger`, `/go/api/openapi.yaml`).

## Using Swagger UI

1. Start Report Server
2. Open `/swagger`
3. Authorize with Bearer API Token or session login
4. Try operations — binary downloads (PDF, CSV, images, ZIP) are supported

## Stable operationIds

Examples used by generators and docs:

- `login`, `getCurrentUser`, `createToken`
- `listReports`, `createReport`, `runReport` (execute via gateway)
- `runExplorerQuery`, `exportExplorerQuery`
- `getMapFeatures`, `getMapExtent`
- `listDatasources`, `createDatasource`, `generateJasperContext`

Do not invent alternate operation names in generated clients.

## Regenerate the repo bundle

Sources live under `go-report-server/openapi/`. The checked-in contract for SDKs is `sdk/openapi.bundle.yaml`:

```bash
# Unix
./sdk/scripts/regenerate-openapi.sh

# Windows PowerShell
./sdk/scripts/regenerate-openapi.ps1

# Or from go-report-server/
make openapi-export
# then copy openapi.bundle.yaml → ../sdk/openapi.bundle.yaml
```

CI fails PRs when the bundle drifts (`.github/workflows/sdk-openapi-check.yml`).

## Generate clients

```bash
curl -o openapi.yaml "$ACUGIS_REPORT_URL/api/openapi.yaml"

openapi-generator-cli generate -i openapi.yaml -g python -o ./sdk-python
openapi-generator-cli generate -i openapi.yaml -g go -o ./sdk-go
openapi-generator-cli generate -i openapi.yaml -g csharp -o ./sdk-dotnet
openapi-generator-cli generate -i openapi.yaml -g java -o ./sdk-java
openapi-generator-cli generate -i openapi.yaml -g typescript-fetch -o ./sdk-ts
```

## Postman

```bash
openapi2postmanv2 -s openapi.yaml -o acugis-report-server.postman_collection.json
```

Or import `openapi.yaml` directly into Postman / Insomnia.

## Legacy aliases

PHP-shaped paths such as `/reports/execute_report.php` and `/reports/parameter_lookup.php` remain documented with `deprecated: true`. Prefer `/api/v1/*` and `/reports/execute`.
