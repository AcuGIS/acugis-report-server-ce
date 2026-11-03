# Compatibility matrix

Report Server is published as two packages. This page describes what those packages compile and what a license entitles. It does not list pricing.

| Package | How it is built | Debian package |
|---------|-----------------|----------------|
| Open Source | `go build -tags oss` | `acugis-report-server` |
| Professional | untagged build | `acugis-report-server-professional` |

`ACUGIS_EDITION` does not grant features. A Professional process without a valid license keeps the community allow-list. Its display name is **Open Source**. Feature denials say **Community Edition**. That is the unlicensed state of the Professional package, not a third product.

## Datasource administration UI

The datasources screen offers the same engines on both packages. Edition does not select them:

PostgreSQL, MySQL, Oracle, SQL Server, Snowflake, SQLite, Cassandra, BigQuery, and DuckDB.

These engines do not share one Jasper path. Cassandra and BigQuery are not JDBC/JNDI datasources. DuckDB is not written as a normal `context.xml` pool. See [Supported datasources](supported-datasources.md).

## Open Source package

Compiled and available:

- Jasper report catalog, execution, folders, resources, and datasources
- Local login and personal access tokens
- Report embedding at `/embed/report/{key}`
- Embedded map providers PostGIS, MySQL, and GeoServer
- GeoServer connection administration (HTML). It is not license-gated

Not in this binary:

- Data Explorer, dashboards, and saved-view embedding
- Map Reports and `/embed/map-report/{key}`
- Dataset Intelligence and the MCP endpoint
- Audit administration, usage analytics, governance, and audit export
- OpenID Connect and SAML
- QGIS embedded maps
- QGIS connection administration (the handler answers 404)
- ArcGIS connection administration and the ArcGIS Designer provider (the handler answers 404)

Audit events can still be recorded when `JRS_AUDIT_ENABLED` is on. The audit administration UI is not in this package.

## Professional package, no valid license

The Professional binary includes the modules the Open Source package omits. Until a license is valid, the community allow-list applies. The build decides what exists. The license decides which compiled features are entitled.

| Capability | Allowed |
|------------|---------|
| Datasource engines in the admin UI | Yes |
| Data Explorer | Yes |
| Embedded maps: PostGIS, MySQL, GeoServer, QGIS | Yes |
| Designer map providers: PostGIS, MySQL | Yes |
| GeoServer connection administration | Yes. Also on the Open Source package. Not license-gated |
| QGIS connection administration | Yes. The page is mounted and is not the QGIS Designer provider check |
| Designer providers GeoServer and QGIS | No |
| ArcGIS Designer provider and the ArcGIS connection page | No |
| Dashboards, Map Reports, Dataset Intelligence | No |
| Audit administration, usage analytics, governance, audit export | No |
| OpenID Connect, SAML | No |

Embedded maps, Designer connection pages, and Designer provider entitlements are different checks. GeoServer connection administration is HTML on both packages. QGIS connection administration is HTML on the Professional package and does not require a license. The QGIS and GeoServer Designer providers still require a license. The ArcGIS connection page calls the ArcGIS Designer provider check before it renders.

## Licensed Professional

A valid license on the Professional package entitles the compiled modules the allow-list denies. That includes Map Reports, dashboards, Dataset Intelligence, audit administration, usage analytics, governance, audit export, OpenID Connect, SAML, the ArcGIS Designer provider and its connection page, and the Designer providers GeoServer and QGIS.

Data Explorer and embedded QGIS are already allowed before that license.

## API surface

Reports, folders, resources, datasource limitations, and personal access tokens are on both packages. Explorer, map-report, dashboard, and view routes are mounted only in the Professional binary.

HTML administration does not share one edition rule:

- Users, groups, and schedules are on both packages.
- GeoServer connection administration is HTML on both packages and is not license-gated.
- QGIS connection administration is HTML in the Professional package and is not license-gated. The Open Source package answers 404.
- ArcGIS connection administration is HTML in the Professional package and requires the ArcGIS Designer provider entitlement. The Open Source package answers 404.
