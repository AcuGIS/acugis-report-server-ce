# Upgrades

Install a newer package or re-run `install.sh` from a newer release archive. Both paths call the same setup. That setup keeps the existing environment file, refreshes `JRS_APP_VERSION`, and applies the database changes. There is no hosted APT repository, so `apt-get upgrade` alone does not fetch a new Report Server package.

Take a [backup](backups.md) first.

## Debian package

Install the new `.deb` the same way as the first install:

```bash
sudo apt-get -y install ./acugis-report-server_<version>_amd64.deb
```

Professional:

```bash
sudo apt-get -y install ./acugis-report-server-professional_<version>_amd64.deb
```

`postinst` runs the packaged setup. On an existing database (`dashboard_users` already present) it applies forward migrations only. On an empty database it applies `database/schema.sql`, then migrations. It does not replace `/etc/acugis-report-server/env`. It updates `JRS_APP_VERSION` in that file. Report files, uploads, licenses, schedule data, and the PostgreSQL database are kept.

Supported package moves:

- Open Source to a newer Open Source package
- Professional to a newer Professional package
- Open Source to Professional

Professional to Open Source is refused.

Replacing Open Source with Professional installs the Professional binary. Data Explorer and embedded QGIS are available before a license is activated. Map Reports and the other license-gated modules stay off until a license is valid. See the [compatibility matrix](../reference/compatibility-matrix.md).

`apt remove` and `apt purge` stop the services and do not drop the database or report files.

## Release tarball

Extract the new archive and run `install.sh` against the same install directory (default `/opt/acugis-report-server`):

```bash
tar -xzf acugis-report-server-<edition>-<version>-linux-amd64.tar.gz
cd acugis-report-server
sudo ./install.sh
```

`<edition>` is `oss` or `professional`. The installer requires the packaged `bin/jrs-server`. It is not a source-checkout installer. It preserves the same configuration and data as the Debian path, and it applies the same schema-then-migrations sequence.

## Docker Compose

From `docker/`, pull the published image and recreate the containers:

```bash
docker compose pull
docker compose up -d
```

See [Docker](docker.md). Named volumes (`postgres-data`, `report-resources`, `report-tomcat-acugis`, `report-tomcat-conf`) are kept. `docker compose down -v` deletes them. These commands do not regenerate `.env`. `JRS_DB_PASS` and `JRS_ADMIN_PASSWORD` stay as you set them. An image upgrade does not rotate those passwords.

## After upgrade

Confirm the services and a sample report:

```bash
curl -sS http://127.0.0.1:9088/health
curl -sS http://127.0.0.1:9088/ready
```

Open `/swagger` and check `info.version` against the package you installed.

## Developer checkout

A git checkout is not the production upgrade. Packaged upgrades are the Debian and tarball sections above.

If you apply schema files yourself, follow the same order as native setup. When `dashboard_users` is absent, apply `database/schema.sql`, then `database/migrations/*.sql` and `database/migrate_*.sql` in order. When that table is already present, skip `schema.sql` and apply those migration files in order. Do not use this manual step as the upgrade procedure for a `.deb` or tarball install.

## Related

- [Installation](installation.md)
- [Backups](backups.md)
- [Health checks](health-checks.md)
- [Troubleshooting](troubleshooting.md)
