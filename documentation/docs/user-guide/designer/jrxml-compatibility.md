# JRXML compatibility

Report Server executes JasperReports definitions (JRXML / compiled `.jasper`) through the Tomcat JasperReports engine. Designer and Server must agree on JasperReports version expectations, datasource names, and resource paths.

## What Server executes

- Main report JRXML published to the catalog
- Optional compiled `.jasper` uploaded alongside the JRXML
- Subreports and images resolved from the reports directory / resource store
- Parameters supplied at execute time

## Practical rules

| Topic | Guidance |
|-------|----------|
| Parameter names | Keep stable; UI and automation pass them as named fields |
| Datasource | Catalog datasource / publish `data_source` must match a configured name |
| Images | Prefer uploaded resources or paths under the server reports directory |
| Fonts | Install fonts under the AcuGIS fonts directory used by Tomcat |
| JDBC drivers | Provide drivers in Tomcat `lib/` (not invented by the API) |
| Map elements | Embedded Map providers and Map Reports follow the [compatibility matrix](../../reference/compatibility-matrix.md) |

## Embedded Maps

Embedded Map elements are filled by the server-side map renderers packaged with the WAR. Use Embedded Map debug when a map image fails to render.

The Open Source package allows embedded PostGIS, MySQL, and GeoServer. QGIS embedded maps require the Professional package. ArcGIS requires a licensed Professional package. Authenticated GeoServer or ArcGIS layers need matching server-side connection properties.

## Map Reports

Map Reports add an interactive map paired with the Jasper report. Publishing a Map Report expects a spatial datasource (PostgreSQL/PostGIS, MySQL, SQL Server, or Oracle). Map Reports require a **licensed Professional** package. They are not in the Open Source package.

## Unsupported assumptions

- Do not assume a Designer connection type is a Report Server datasources-UI engine. The UI list is PostgreSQL, MySQL, Oracle, SQL Server, Snowflake, SQLite, Cassandra, BigQuery, and DuckDB
- Do not treat Db2 Spatial as a datasources-UI engine. `db2` is not listed there, and Jasper context generation skips it
- Data Explorer does not generate or rewrite JRXML; handoff payloads are for Report Builder workflows without writing JRXML from Explorer

## Related

- [Publishing](publishing.md)
- [Supported formats](../../reference/supported-formats.md)
- [Supported datasources](../../reference/supported-datasources.md)

---

## For Developers

Debug and execute details: [REST API](../../developer/rest-api/overview.md). Map Report CLI commands require Professional and are not documented as Open Source server routes.
