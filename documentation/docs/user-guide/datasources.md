# Datasources

**Datasources** are named connections used when a report is filled. The admin UI offers the same engines on the Open Source and Professional packages.

## What a datasource is

A named connection record that the report definition references by name. Connection **names** must stay consistent between Designer, Server admin entries, and JRXML.

JDBC engines that use Tomcat JNDI are also written into `context.xml` when you generate Jasper context. Cassandra and BigQuery use collection fill instead of a JNDI resource. DuckDB uses DuckDB JDBC and is not written as a `context.xml` pool.

## Engines in the datasources UI

| Key | Label |
|-----|-------|
| `postgresql` | PostgreSQL |
| `mysql` | MySQL |
| `oracle` | Oracle |
| `sqlserver` | Microsoft SQL Server |
| `snowflake` | Snowflake |
| `sqlite` | SQLite / GeoPackage |
| `cassandra` | Apache Cassandra |
| `bigquery` | Google BigQuery |
| `duckdb` | DuckDB |

These types are not edition-gated. `mariadb` is stored as `mysql`. `geopackage` and `gpkg` are stored as `sqlite`.

`db2` is recognized by server code, but it is not listed in this UI, and Jasper context generation skips it. Db2 Spatial map loading is a different Professional Data Explorer provider. Do not treat those as the same feature.

See the [compatibility matrix](../reference/compatibility-matrix.md) and [supported datasources](../reference/supported-datasources.md).

## GeoServer and ArcGIS connections

GeoServer connections are configured in the HTML admin page (`/reports/geoserver_connections.php`) on both packages. That page is not license-gated. QGIS connection administration is in the Professional package and is not license-gated. The ArcGIS connection page is mounted only in the Professional package and requires the ArcGIS Designer provider entitlement. Using ArcGIS as a map provider requires that same entitlement. Names must match Report Designer Connection Manager entries. Properties files are consumed by Tomcat at fill time.

Public ArcGIS FeatureServer layers may need no credentials if the JRXML already snapshots the service URL. That does not remove the ArcGIS Designer provider entitlement.

## Who manages datasources?

Creating and deleting datasources and regenerating Jasper context are **service-admin** operations. Day-to-day report users typically select an existing datasource when publishing or running reports.

## Related

- [Reports](reports.md)
- [Embedded Maps](embedded-maps.md)
- [Supported datasources](../reference/supported-datasources.md)
- [Compatibility matrix](../reference/compatibility-matrix.md)

---

## For Developers

REST datasource APIs have intentional limitations (list returns names only; no detail GET; GeoServer/ArcGIS CRUD is HTML-only).

| Integration path | Go here |
|------------------|---------|
| REST caveats | [REST compatibility](../developer/rest-api/compatibility.md) · [REST reference](../reference/rest-api.md) |
| CLI | [CLI datasources](../developer/cli/datasources.md) |
| SDK | [SDK overview](../developer/sdk/overview.md) |
