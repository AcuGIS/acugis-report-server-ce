# API compatibility

Guidance for integrators building against the current REST surface.

## Prefer stable paths

| Prefer | Avoid when possible |
|--------|---------------------|
| `/api/v1/*` | Deprecated PHP-shaped aliases |
| `/reports/execute` | `/reports/execute_report.php` |
| Bearer API Tokens | Scraping HTML forms |

## Known gaps (honest inventory)

Not yet available as JSON REST (HTML/admin or reserved CLI only):

- Users / groups CRUD
- Schedules / jobs / executions / recipients CRUD
- GeoServer connection CRUD
- ArcGIS connection CRUD
- Datasource detail GET and dedicated PUT
- Standalone report permissions endpoint
- Full dashboard layout CRUD beyond Explorer view flags

The CLI prints `Not yet implemented by Report Server.` for reserved `users`, `groups`, `schedules`, and `jobs` groups until endpoints ship — without breaking CLI command structure.

## Datasource API limitations

REST today:

1. List names
2. Create / upsert by name (form-urlencoded)
3. Delete by id
4. Jasper context generation

No detail GET, no dedicated update, no GeoServer/ArcGIS connection CRUD via REST.

## Package boundaries

Data Explorer routes (`/explorer`, `/api/v1/explorer/*`) are compiled into the Professional package and allowed before a license is activated. They are not in the Open Source package. Dashboard routes require feature `dashboards` and a valid license. Map report routes (`/api/v1/map/*`) are also Professional-binary routes and require a license. OIDC and SAML routes are Professional-binary routes and require a license. Audit administration, usage analytics, and audit export follow the same license rule. Audit event recording does not. See [Auditing](../../administration/auditing.md) and the [compatibility matrix](../../reference/compatibility-matrix.md).

## Response consistency

Expect mixed success envelopes (`ok` vs `success`), JSON vs `text/plain` on gateways, and occasional HTTP 200 soft failures on operational tools. Validate against OpenAPI examples for the specific operationId you call.

## Versioning

`info.version` in the served OpenAPI document tracks `JRS_APP_VERSION`. Documentation site versioning can use [mike](https://github.com/jimporter/mike); pin client code to operationIds rather than prose descriptions.
