# Concepts

Shared vocabulary used throughout this documentation.

## Report Designer

Desktop authoring tool for JasperReports JRXML. Publish to Report Server with the same datasource and map connection names the server has configured.

Report Server does not edition-gate the engines in the datasources UI. Map Reports on the server require a licensed Professional package. See the [compatibility matrix](../reference/compatibility-matrix.md).

## Report Server

The hosted catalog and execution platform: web UI, REST API, JasperReports engine, folders, resources, and datasources.

Two packages:

- **Open Source** — Jasper reports, the datasources UI, embedded maps for PostGIS, MySQL, and GeoServer, local login, and personal access tokens
- **Professional** — the Open Source surface, plus Data Explorer and embedded QGIS before a license is activated. Map Reports, dashboards, Dataset Intelligence, MCP, ArcGIS, and other license-gated administration require a valid license

An unlicensed Professional process displays as Open Source and enforces that allow-list. `ACUGIS_EDITION` does not change it.

## Report

A published JRXML (and optional compiled `.jasper`) registered in the catalog with a stable **key**, datasource name, parameters, thumbnail, and access groups.

## Embedded Map

A map element authored in Report Designer and rendered inside a Jasper report fill. Server-side debug is available via `/api/v1/embedded-map/debug` and the CLI.

On the Open Source package, embedded PostGIS, MySQL, and GeoServer are allowed. QGIS embedded maps and ArcGIS require the Professional package; ArcGIS also requires a valid license.

## Map Report

A report type that pairs a Jasper report with an interactive map viewer (features, extent → report parameters). Map Reports require the Professional package and a valid license. They are not compiled into the Open Source package. Spatial checks accept PostgreSQL/PostGIS, MySQL, SQL Server, and Oracle.

## Data Explorer

Interactive browser analysis over a shared dataset (table or report query result). Widgets include Summary, Statistics, Table, Chart, Map, Spatial Analysis, Crosstab/Pivot, and Filter.

Data Explorer is compiled into the Professional package and is allowed before a license is activated. It is not in the Open Source package. Interactive engines are PostgreSQL, MySQL/MariaDB, Oracle, and SQL Server. Spatial Analysis execution requires PostgreSQL/PostGIS.

## API Token

A personal access token with prefix `jrs_` used as `Authorization: Bearer …` for CLI, SDKs, and CI. Managed under Profile → API Tokens or `/api/v1/auth/tokens`.

## Dataset Intelligence

The Professional catalog of report datasets: parameters, fields, business descriptions, governance, stored profiles, and lineage. It requires a valid license. That implementation is not in this Open Source source tree.

## Folder & ACL

Content is organized in a folder tree. Permissions can inherit from a parent or be set explicitly. Report visibility also respects `access_groups` set at publish/update time.

## Resource

Uploaded binary assets (images, fonts, files) referenced by reports. Addressable by UUID; image helpers exist under `/api/v1/resources/images`.

## Datasource

A named connection used at report fill time. The admin UI engines are not edition-gated. REST today supports list (names), create/upsert, delete, and Jasper context generation — not detail GET, dedicated update, or GeoServer/ArcGIS connection CRUD.

## Client environment variables

| Variable | Purpose |
|----------|---------|
| `ACUGIS_REPORT_URL` | Base URL of Report Server. Packaged default base path is `/`. Include `JRS_BASE_PATH` when it is set. |
| `ACUGIS_REPORT_TOKEN` | Bearer API Token (`jrs_…`) |
