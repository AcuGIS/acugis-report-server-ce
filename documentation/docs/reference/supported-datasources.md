# Supported datasources

The datasources admin UI offers the same engines on the Open Source and Professional packages. Edition does not select JDBC types. See the [compatibility matrix](compatibility-matrix.md) for what a package and a license actually change.

## Datasources UI

| Key | Label | Notes |
|-----|-------|-------|
| `postgresql` | PostgreSQL | JDBC. Aliases `postgres`, `pgsql`, `pg`. |
| `mysql` | MySQL | JDBC. Alias `mariadb`. |
| `oracle` | Oracle | JDBC. A normal database value is the service name (`jdbc:oracle:thin:@//host:port/name`). A SID requires the `sid:` prefix. |
| `sqlserver` | Microsoft SQL Server | JDBC. Aliases `mssql`, `sql server`. |
| `snowflake` | Snowflake | JDBC. |
| `sqlite` | SQLite / GeoPackage | JDBC (`org.sqlite.JDBC`) written to `context.xml`. The database field is the file path. Aliases `geopackage`, `gpkg`. |
| `cassandra` | Apache Cassandra | Collection fill, not a Tomcat JNDI resource. Aliases include `astra`. |
| `bigquery` | Google BigQuery | Collection fill, not a Tomcat JNDI resource. Database field is the project ID. |
| `duckdb` | DuckDB | DuckDB JDBC. Not written as a `context.xml` pool. |

## Db2

`db2` is a recognized type in server code, and JDBC context generation skips it. It is **not** in the datasources admin UI. Do not treat it as one of the engines above.

Db2 Spatial map loading is a separate Professional Data Explorer provider. It is not the same as a Db2 row in the datasources UI, and it reports that a Db2 driver must be configured.

## Map connections

GeoServer connection administration is an HTML page on both packages and is not license-gated. QGIS connection administration is in the Professional package and is not license-gated; the Open Source package answers 404. The ArcGIS connection page is mounted only in the Professional package and requires the ArcGIS Designer provider entitlement, as does using ArcGIS as a map provider. There is no REST CRUD for these connection tables.

## Data Explorer

Data Explorer is in the Professional package, including before a license is activated. It is not in the Open Source package.

Interactive engines accepted by Explorer are PostgreSQL, MySQL (including MariaDB), Oracle, and SQL Server. Spatial Analysis execution requires PostgreSQL/PostGIS.

## REST limitations

Datasource REST supports list (names), create/upsert, delete, and Jasper context generation. It does not support detail GET, a dedicated update, or GeoServer/ArcGIS CRUD.
