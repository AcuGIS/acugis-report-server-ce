# AcuGIS Report Server — native Linux installation

This archive is a **precompiled** binary distribution. Installation does not
require Go, gcc, make, Git, or the source repository.

The Open Source and Professional packages use the same layout and installer.
The compiled binaries determine the edition.

## Install

```bash
tar -xzf acugis-report-server-<edition>-<version>-linux-amd64.tar.gz
cd acugis-report-server
sudo ./install.sh
```

If `JRS_DB_PASS` / `JRS_ADMIN_PASSWORD` are unset, the installer generates them
and writes `/etc/acugis-report-server/initial-credentials`. You can still pass
explicit values.

Embedded Analytics needs a signing secret. When `ACUGIS_EMBED_TOKEN_SECRET`
and `JRS_EMBED_TOKEN_SECRET` are both unset, the installer generates a
32-byte hex secret on the server and stores it in
`/etc/acugis-report-server/env` (mode 640, root:www-data). That file is what
systemd reads. The secret is not printed, it is not copied into
`initial-credentials`, and it is not copied into `$JRS_INSTALL_DIR/.env`.
An existing value is kept on upgrade. A secret left in `.env` by an older
package is removed on upgrade. To supply your own before first configure:

```bash
sudo ACUGIS_EMBED_TOKEN_SECRET='…' ./install.sh
```

```bash
sudo JRS_DB_PASS='…' JRS_ADMIN_PASSWORD='…' ./install.sh
```

Default install root: `/opt/acugis-report-server`.

Unattended / Agent-style install (no prompts):

```bash
sudo JRS_ASSUME_YES=1 \
  JRS_DB_HOST=localhost \
  JRS_DB_NAME=jasper_reports \
  JRS_DB_USER=jasper \
  JRS_DB_PASS='…' \
  JRS_ADMIN_USER=admin \
  JRS_ADMIN_PASSWORD='…' \
  ./install.sh
```

The installer does **not** download this product package. Supply the archive
yourself (or via AcuGIS Agent).

## Layout

```text
acugis-report-server/
├── bin/                 jrs-server, jrs-run-schedule
├── database/            schema.sql + migrations
├── tomcat/              WAR + Linux JDBC jars
├── scripts/             seed_admin.php, run_schedule.php, native-setup.sh, configure-instance.sh
├── systemd/             native unit templates
├── config/              env.example
├── docs/                this guide
├── reports/             portal.css + json/
├── shared/ui/           catalog shell assets
├── assets/js/           map viewer JS (edition-filtered)
├── sdk/javascript/dist/ browser embed SDK (acugis-report.js and related)
├── lib/                 PHP helpers for admin seed + systemd schedules
├── email_tmpl/
├── RELEASE              product / version / edition / revision
├── release.json
└── install.sh
```

`database/schema.sql` is the schema for **this** edition (OSS or Professional).
The installer applies the same filename for both packages.

## Runtime dependencies

The installer may install or check:

- Java 17
- Apache Tomcat 10.1
- PostgreSQL server (localhost) and `psql`
- Apache reverse proxy (optional)
- PHP CLI + php-pgsql (admin seed and native systemd schedule runner)

It does **not** install golang, gcc/g++, make, or Git.

## Services

| Unit | Role |
|------|------|
| `acugis-tomcat.service` | JasperReports WAR on `127.0.0.1:8080` |
| `acugis-report-server.service` | Go UI/API on `127.0.0.1:9088` |
| `jrs-schedule-<id>.timer` | Created by the app when a schedule is saved |

Open `http://127.0.0.1:9088/` (or the Apache vhost that proxies to it).

## Configuration

First install writes `/etc/acugis-report-server/env` from the installer
template plus installer environment variables. Upgrades do not replace that
file. They refresh `JRS_APP_VERSION` and, when embed signing is not already
configured, add `ACUGIS_EMBED_TOKEN_SECRET`.

Also first-install only:

- `$JRS_INSTALL_DIR/.env` (hand-start mirror of the other settings, mode 640; embed signing secrets are not included)
- Tomcat `conf/context.xml` and map connection property files
- `acugis.properties`

## Upgrades

Supported:

- OSS → newer OSS
- Professional → newer Professional
- OSS → Professional (binaries/assets + missing Professional migrations)

Not supported (refused):

- Professional → OSS

Upgrades replace binaries, WAR, JDBC jars, UI assets, schema files, and
scripts. They do **not** delete or replace:

- `/etc/acugis-report-server/env` (an unset embed signing secret is added; existing secrets stay)
- database credentials
- `reports/json/`
- uploads, resources, thumbnails, licenses
- schedule output, logs, and env files
- Tomcat `acugis/` report files
- existing `context.xml` / connection properties

Database migrations are forward-only (`IF NOT EXISTS` / idempotent SQL).
The installer never drops Professional tables or downgrades schema.

## OSS → Professional

An existing OSS install can become Professional by installing the Professional
package on top of it:

1. Extract the Professional archive.
2. Run `sudo ./install.sh` with the same `JRS_INSTALL_DIR`.
3. The installer replaces OSS binaries/assets and applies Professional
   migrations (`009`–`015`, `migrate_qgis.sql`, `migrate_arcgis.sql`).
4. It does **not** write `ACUGIS_EDITION=professional` and does not contact
   the license server. The Professional package is capable of Professional
   features, but they stay off until an administrator activates an
   `ACUGIS-...` key under Administration → License. Installation ID and
   the ACL1 token live in `/var/lib/acugis-report-server/` and are kept
   across upgrade. Remove/purge does not delete them.

No separate migration tool is required.

## Discover edition / version

```bash
cat RELEASE
# or
bin/jrs-server -version
```

Open Source builds print `Edition: Open Source`. Internal entitlement identifiers
such as `community` are unchanged.

## Debian / Ubuntu (`.deb`)

The same edition staging used for the `.tar.gz` packages also produces:

```text
dist/acugis-report-server_<version>_amd64.deb
dist/acugis-report-server-professional_<version>_amd64.deb
```

`acugis-report-server` is the Open Source edition (the public package name is
not `community`).

Build on a Debian/Ubuntu amd64 machine (no Docker). Requires Go, gcc
(`build-essential`), Node.js/npm (JavaScript embed SDK), and `dpkg-dev`:

```bash
make deb VERSION=1.0.2
make deb-oss VERSION=1.0.2
make deb-professional VERSION=1.0.2
make deb-clean VERSION=1.0.2
```

`make deb` always compiles `jrs-server` and `jrs-run-schedule` from the
current source tree (OSS: `-tags oss`; Professional: untagged) and packs
with host `dpkg-deb`. It does not use Docker, cached go-builder images, or
existing `dist/*.tar.gz`. `make deb-clean` deletes staging/output artifacts
and then runs a fresh `make deb`.

Docker remains optional for Windows/cross-platform builds:

```bash
PACKAGE_USE_DOCKER=1 make deb VERSION=1.0.2
```

Install (no prompts; no debconf). Use `apt`, not `dpkg -i`, so dependencies
are pulled:

```bash
sudo apt-get -y install ./acugis-report-server_1.0.0_amd64.deb
# or, after the AcuGIS APT repository is configured:
sudo apt-get -y install acugis-report-server
```

That installs Java 17, PostgreSQL, PHP, Apache, and the application. On first
configure the package generates database and admin passwords when unset,
creates `jasper` / `jasper_reports`, seeds `admin`, downloads Tomcat 10.1
if needed, enables the Apache reverse proxy to `127.0.0.1:9088`, and starts
the systemd services. Open the host over HTTP (Apache) or
`http://127.0.0.1:9088/` and read:

```text
/etc/acugis-report-server/initial-credentials
```

Existing `/etc/acugis-report-server/env` is not replaced (upgrades and
AcuGIS Agent). A missing `ACUGIS_EMBED_TOKEN_SECRET` is added when neither
that key nor `JRS_EMBED_TOKEN_SECRET` is already set; an existing secret is
not rotated. To use a remote database, set `JRS_DB_HOST` and `JRS_DB_PASS`
in that file before install, or run:

```bash
sudo /opt/acugis-report-server/scripts/configure-instance.sh
```

The `.deb` uses the same filesystem layout as `./install.sh`:

| Path | Role |
|------|------|
| `/opt/acugis-report-server/` | Application tree |
| `/etc/acugis-report-server/env` | Administrator-owned config (created if missing; never replaced on upgrade) |
| `/etc/acugis-report-server/initial-credentials` | First-boot generated passwords (mode 600; not rotated on upgrade) |
| `/lib/systemd/system/acugis-report-server.service` | Go service |
| `/lib/systemd/system/acugis-tomcat.service` | Jasper Tomcat |
| `/etc/sudoers.d/acugis-report-server` | NOPASSWD `ln`/`rm`/`systemctl` for `jrs-schedule-*` units (created on configure; not a shipped conffile) |
| `/opt/tomcat/` | Tomcat 10.1.57 (downloaded on first configure if absent; not a `Depends`) |
| `/opt/tomcat/acugis/` | Reports, uploads, fonts, licenses, resource-paths |

**Depends:** Java 17, `postgresql`, `postgresql-client`, `apache2`, `php-cli`,
`php-pgsql`, `php-xml`, `php-curl`, `php-gd`, `curl`, `ca-certificates`, `openssl`, `tar`, `adduser`,
`sudo`.

Apache is configured automatically via the native helper (`a2enmod proxy
proxy_http headers rewrite`, `a2enconf acugis-report-server`, reload/restart).
The snippet is copied to `/etc/apache2/conf-available/` only if that file does
not already exist. Report Server remains bound to `127.0.0.1:9088`.

OSS → Professional:

```bash
sudo apt-get -y install acugis-report-server-professional
```

This replaces `acugis-report-server` while preserving env and application data.
Professional → Open Source is refused.

`apt remove` removes packaged binaries/units and does **not** delete env,
reports, uploads, licenses, schedule data, Tomcat AcuGIS data, or the database.
`apt purge` is equally conservative: those paths are left in place.

These `.deb` files are suitable for a later signed APT repository. They do not
embed repository URLs.

### `lib/map_report.php` (technical debt)

The Open Source tarball and `.deb` include `lib/map_report.php` because
`lib/bootstrap.php` (used by the native PHP schedule runner
`scripts/run_schedule.php`) requires it. This is **not** a Professional feature
shipment: Ad Hoc, Dashboards, Map Reports UI/API, QGIS/ArcGIS, Audit, Usage
Analytics, Governance, OIDC/SAML, SIEM export, Data Intelligence, and
migrations `009`–`015` remain absent from OSS. Do not treat this file as a
reason to add further Professional PHP to the OSS package. Separate the
schedule runner dependency in a later change; do not refactor scheduling as
part of Debian packaging.

## Docker

Docker images are a separate release channel:

```bash
make docker-build-oss VERSION=1.0.0
make docker-build-professional VERSION=1.0.0
```

Images are tagged `acugis/report-server:<version>-oss` (and `:oss`) or
`acugis/report-server:<version>-professional` (and `:professional`).
Package builds do not push images.
