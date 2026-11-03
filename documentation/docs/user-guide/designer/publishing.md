# Publishing

Publish JRXML from Report Designer (or automation) into the Report Server catalog so others can browse, parameterize, and run the report.

## What publishing does

- Uploads the JRXML (and optional compiled `.jasper` / thumbnail)
- Binds a datasource name
- Sets display name, description, type (standard or Map Report)
- Sets `access_groups` for visibility
- Places the definition in the catalog under a stable key

## Report types

When publishing you can choose a Jasper report or a **Map Report**. Map Report pairs the definition with interactive map configuration and requires a spatial datasource (PostgreSQL/PostGIS, MySQL, SQL Server, or Oracle). Map Reports require a licensed Professional package.

## Access groups

Set `access_groups` at publish/update time so group ACL applies when users browse and run the report. Access groups are carried on create/update — there is no separate report-permissions screen that bypasses publish/update. Folder ACLs still apply. See [Permissions](../permissions.md).

## After publish

1. Confirm the report appears in the catalog
2. Open the viewer and inspect parameters/config
3. Run a test with representative parameters
4. For maps: verify Embedded Map output or Map Report interaction

## GeoServer / ArcGIS names

If the JRXML references GeoServer or ArcGIS Feature Server connections by name:

1. Create the same **Name** under Report Server admin (GeoServer Connections / ArcGIS Connections UI).
2. Ensure the generated properties file is available to Tomcat `conf/`.
3. Restart the report-server container/process if Java caches the properties file.

These connection UIs are HTML/form based — they are **not** exposed as REST CRUD.

## Related

- [Reports](../reports.md)
- [Designer overview](overview.md)
- [JRXML compatibility](jrxml-compatibility.md)

---

## For Developers

| Integration path | Go here |
|------------------|---------|
| CLI / REST publish | [Publishing a report](../../developer/tutorials/publishing-a-report.md) |
| CLI reports | [CLI reports](../../developer/cli/reports.md) |
| CI/CD | [GitHub Actions](../../developer/tutorials/github-actions.md) |
