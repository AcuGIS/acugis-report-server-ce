# Backups

Report Server has no backup command and no restore API. Back up the PostgreSQL database and the files the process uses, then restore both together.

Take the database dump and the file copy as close together as you can. Catalog rows point at JRXML, resources, and saved views on disk.

Configuration backups contain passwords and tokens. Encrypt them and restrict who can read them.

## PostgreSQL

The application database holds users, sessions, personal access tokens, the report catalog, datasource definitions (including secrets), folder permissions, schedules, audit events, and OIDC/SAML settings when those tables exist.

The package and Compose default database name is `jasper_reports`. Use `pg_dump` or your host's volume snapshot. Include the schema you run (`JRS_DB_SCHEMA`, default `public`).

A restore must match the server version you start afterwards. The installer applies migrations on upgrade. Restoring a dump from a newer server onto an older binary is not a supported path.

## Files on a Debian or tarball install

Paths below are the ones `native-setup.sh` writes. If you changed the env file, use those values instead.

| State | Default path |
|-------|----------------|
| Environment file | `/etc/acugis-report-server/env` |
| First-boot passwords | `/etc/acugis-report-server/initial-credentials` |
| License state (`installation-id`, `license.lic`, `report-designer-web.lic`) | `/var/lib/acugis-report-server` |
| JRXML and compiled reports | `/opt/tomcat/acugis/reports` |
| Jasper image resources | `/opt/tomcat/acugis/resource-paths` |
| Uploads, fonts, drivers | `/opt/tomcat/acugis/uploads`, `fonts`, `drivers` |
| Tomcat datasource context | `/opt/tomcat/conf/context.xml` |
| GeoServer connections | `/opt/tomcat/conf/geoserver-connections.properties` |
| QGIS connections | `/opt/tomcat/conf/qgis-connections.properties` |
| ArcGIS connections | `/opt/tomcat/conf/arcgis-connections.properties` |
| Folder resources | `/opt/acugis-report-server/resources` |
| Resource thumbnails | `/opt/acugis-report-server/thumbnails` |
| Schedules and output | `/opt/acugis-report-server/schedules` |
| Email templates | `/opt/acugis-report-server/email_tmpl` |
| Data Explorer saved views | `/opt/acugis-report-server/explorer/views` |

Saved views exist only when the Professional package has written them. The Open Source package does not mount Data Explorer.

SAML private keys and certificates live in the database or in the files named by `ACUGIS_SAML_SP_KEY_FILE` and `ACUGIS_SAML_SP_CERT_FILE`. Back up those paths when you set them.

`apt remove` and `apt purge` do not delete the database, the env file, report files, or the license directory. They are still your backup, not a product snapshot.

## Docker Compose

Persistent state is the Compose volumes, not the image:

| Volume | What it holds |
|--------|----------------|
| PostgreSQL data volume | Application database |
| `report-resources` | `/opt/acugis-report-server`, including license state at `var/license`, resources, schedules, and Explorer views |
| `report-tomcat-acugis` | JRXML, uploads, fonts, resource paths |
| `report-tomcat-conf` | `context.xml` and the GeoServer, QGIS, and ArcGIS properties files |
| Host `JRS_DATA_HOST_PATH` (default `./data`) | File datasources mounted at `/data` |

Also keep the Compose env file you pass in (database password, embed secret, admin password, Designer token). It is not inside the image.

## Restore

1. Stop `acugis-report-server` and `acugis-tomcat`, or stop the Compose project.
2. Restore the database.
3. Restore the files and volumes to the same paths.
4. Restore `/etc/acugis-report-server/env` (or the Compose env file) so the process points at those paths and can read the same embed secret and database password.
5. Start the services.

Confirm the catalog lists reports and that a known report runs. `acugis-report doctor` checks the public health endpoints when `ACUGIS_REPORT_URL` points at the restored server. There is no separate restore verification command in the product.

If you restore the database without `license.lic` and `installation-id`, the Professional package starts unlicensed. If you restore a license directory from a different installation, the token will not match the installation ID.

## Related

- [Licensing](licensing.md)
- [Upgrades](upgrades.md)
- [Environment variables](../reference/environment-variables.md)
- [Security](security.md)
