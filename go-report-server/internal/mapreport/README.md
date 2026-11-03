# Map Report package

Ports PHP Map Report runtime APIs used by the interactive viewer:

- `reports/map_features.php` → `GET /api/v1/map/{key}/features`
- `reports/map_extent.php` → `GET /api/v1/map/{key}/extent`

Source of truth: `lib/map_report.php`, `lib/map_definition.php`.

Requires a spatial-capable report datasource (looked up via `internal/datasources`):

- PostgreSQL / PostGIS
- MySQL / MariaDB (spatial)
- Microsoft SQL Server (`geometry` / `geography`)
- Oracle Spatial (`SDO_GEOMETRY`)

Interactive map features are loaded as GeoJSON (EPSG:4326). Spatial filter predicates in the JRXML (`ST_Intersects`, `SDO_RELATE`, etc.) must use the target engine's spatial SQL; the server binds `p_geometry` / `p_srid` as WKT + SRID.
