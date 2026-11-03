# AcuGIS Report Server — OpenAPI

The official REST API contract lives in [`openapi/`](../openapi/).

It is the authoritative source for SDK generation (Python, Go, .NET, Java, TypeScript),
the CLI, Swagger UI, ReDoc, and Postman collections.

## Modular layout

```text
openapi/                 # source of truth (YAML only — no Go package)
  openapi.yaml
  schemas.yaml
  *.yaml
internal/openapi/spec/   # embed mirror (keep in sync via make openapi-sync)
```

Edit files under `openapi/`, then run `make openapi-sync` before build/test
(or rely on Makefile targets that sync automatically). Do not add a `go.mod` or
`.go` files under `openapi/` — that would make Go try to resolve a separate
module from GitHub.

## Live endpoints

| Method | Path | Purpose |
|--------|------|---------|
| GET | `/api/openapi.yaml` | Bundled OpenAPI 3.0 YAML (`info.version` = `JRS_APP_VERSION`) |
| GET | `/api/openapi.json` | Same document as JSON |
| GET | `/swagger` | Swagger UI (loads `/api/openapi.yaml`) |

With `JRS_BASE_PATH=/go`, use `/go/api/openapi.yaml` and `/go/swagger`.

## Using Swagger UI

1. Start the Report Server.
2. Open `/swagger` (or `/go/swagger`).
3. Click **Authorize**:
   - **BearerAuth** — paste a PAT (`jrs_…`) from Profile → API Tokens, or `POST /api/v1/auth/tokens`
   - **SessionCookie** — log in first via `POST /api/v1/auth/login` (Try it out) so `jrs_session` is set
4. Use **Try it out** on any operation. Binary downloads (PDF, CSV, images, ZIP) are supported.

## Authentication

See [`authentication.md`](./authentication.md).

- Session cookie: `jrs_session`
- Bearer: `Authorization: Bearer jrs_…`

Most `/api/v1/*` routes accept either. Health/ready/diagnostics/login are public.

## Binary responses

| Area | Formats |
|------|---------|
| Report execute (`/reports/execute`) | `html`, `pdf`, `xlsx`, `xls`, `csv`, `docx`, `rtf`, `odt`, `ods`, `pptx`, `xml` |
| Explorer export (Professional; not served by this Open Source server) | `csv`, `excel`, `geojson`, `gpkg` (ZIP), `shapefile` (ZIP) |
| Thumbnails / images | PNG and resource image MIME types |

Execute/image gateway errors are often `text/plain`, not JSON.

## Explorer examples

These routes are not implemented by the Open Source server. They require Report Server Professional.

```bash
# List explorer datasources
curl -s -H "Authorization: Bearer $TOKEN" \
  "$BASE/api/v1/explorer/datasources"

# Run a query
curl -s -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d '{"datasource":"reporting_db","schema":"public","table":"parcels","columns":["id","name"],"limit":50}' \
  "$BASE/api/v1/explorer/query"

# Export CSV
curl -s -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d '{"datasource":"reporting_db","schema":"public","table":"parcels","columns":["id","name"]}' \
  "$BASE/api/v1/explorer/export?format=csv" -o parcels.csv
```

## Map Report examples

These routes are not implemented by the Open Source server. They require Report Server Professional.

```bash
# GeoJSON features
curl -s -H "Authorization: Bearer $TOKEN" \
  "$BASE/api/v1/map/nycdata/features"

# Spatial extent → report params
curl -s -H "Authorization: Bearer $TOKEN" \
  "$BASE/api/v1/map/nycdata/extent?srid=4326&xmin=-74.1&ymin=40.6&xmax=-73.7&ymax=40.9"
```

## Generating SDKs

Use the bundled spec (served or exported):

```bash
# Export from a running server
curl -o openapi.yaml "$BASE/api/openapi.yaml"

# OpenAPI Generator examples
openapi-generator-cli generate -i openapi.yaml -g python -o ./sdk-python
openapi-generator-cli generate -i openapi.yaml -g go -o ./sdk-go
openapi-generator-cli generate -i openapi.yaml -g typescript-fetch -o ./sdk-ts
openapi-generator-cli generate -i openapi.yaml -g csharp -o ./sdk-dotnet
openapi-generator-cli generate -i openapi.yaml -g java -o ./sdk-java
```

Stable `operationId` values (e.g. `login`, `listReports`, `runReport`, `runExplorerQuery`)
are intentional — generators must not invent names.

## Generating Postman collections

```bash
openapi2postmanv2 -s openapi.yaml -o acugis-report-server.postman_collection.json
# or import openapi.yaml directly into Postman / Insomnia
```

## ReDoc

```bash
npx @redocly/cli preview-docs openapi/openapi.yaml
# Prefer the bundled document for preview:
curl -o /tmp/openapi.yaml http://127.0.0.1:9088/api/openapi.yaml
npx @redocly/cli preview-docs /tmp/openapi.yaml
```

## Validation

```bash
go test ./internal/openapi/
make openapi-check
make openapi-export   # writes openapi.bundle.yaml

# External lint (bundled document + project rules):
npx @redocly/cli lint openapi.bundle.yaml --config openapi/redocly.yaml
```

Structural checks also run in `internal/openapi` tests (unique `operationId`, required paths, YAML/JSON marshal).
## Legacy aliases

PHP-shaped paths such as `/reports/execute_report.php` and
`/reports/parameter_lookup.php` remain documented with `deprecated: true`.
Prefer `/api/v1/*` and `/reports/execute`.

## Related docs

- [`api-inventory.md`](./api-inventory.md) — full endpoint inventory
- [`api-gaps.md`](./api-gaps.md) — known gaps (users/groups/schedules REST pending)
- [`authentication.md`](./authentication.md)
- Historical auth-only stub: [`openapi-auth.yaml`](./openapi-auth.yaml) (superseded by `openapi/`)
