# Embedded Maps

An **Embedded Map** is a map element authored in Report Designer and rendered during Jasper report fill on Report Server.

## When to use

Use Embedded Maps when the map should appear as part of the generated report output (PDF, HTML, and other formats), rather than as a separate interactive Map Report viewer.

## Editions

- **Open Source** allows embedded PostGIS, MySQL, and GeoServer.
- **Professional**, including before a license is activated, also allows embedded QGIS.
- **ArcGIS** requires a licensed Professional package.

Connection **names** in JRXML must match server-side GeoServer or ArcGIS connection admin entries when credentials are required. QGIS and ArcGIS connection admin routes are not in the Open Source package.

## Operational checklist

1. Confirm the report datasource is spatial where required (Embedded Map prepare paths expect PostgreSQL/PostGIS for certain flows).
2. Confirm GeoServer/ArcGIS connection names and properties files are loaded by Tomcat.
3. Restart the report-server process after changing connection properties if Java caches them.
4. Use Embedded Map debug when the map image is blank, shows errors such as **Token Required**, or fails to compile map configuration — then execute the report again.

## Related

- Map Reports are a separate Professional viewer and are not part of this Open Source guide.
- [Designer overview](designer/overview.md)
- [Troubleshooting](../administration/troubleshooting.md)

---

## For Developers

| Integration path | Go here |
|------------------|---------|
| Debug API | [REST API](../developer/rest-api/overview.md) |
| CLI `embedded-map debug` | [CLI reference](../reference/cli.md) |
| SDK | [SDK overview](../developer/sdk/overview.md) |
