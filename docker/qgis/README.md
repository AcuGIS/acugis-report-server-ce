# QGIS Server companion (optional Compose profile)

Place QGIS Desktop projects here. Report Server does not create or edit them.

```text
docker/qgis/projects/planning.qgz
docker/qgis/data/           # GeoPackage, shapefiles, rasters referenced by those projects
```

Inside the container:

```text
./qgis/projects  →  /qgis/projects
./qgis/data      →  /qgis/data
```

In QGIS Desktop, point file datasources at `/qgis/data/...` when the project will be served from Docker. PostGIS/MySQL connections should use a hostname reachable from the Compose network.

Start:

```bash
cd docker
docker compose --profile qgis up -d
```

`docker compose up -d` without the profile is unchanged and does not start QGIS Server.

Designer URL: `http://localhost:9088/qgis/` (or `https://reports.example.com/qgis/` behind TLS).
Report Server catalog URL for the same connection name: `http://qgis-server`.

Existing Docker databases (schema already applied) need `database/migrate_qgis.sql` once; the report-server entrypoint applies it on boot. Native OS installs do not bundle QGIS Server — point the catalog at an existing QGIS endpoint if you have one.
