# Installation

Report Server installs from a local Debian package, a release tarball, or Docker Compose. There is no hosted APT repository in this project. Installing the JasperReports WAR by itself is not a Report Server installation. The WAR is the fill engine used by the package and by the Docker image.

The packaged server listens on `http://127.0.0.1:9088/` with base path `/`. Apache on a Debian install proxies `/` to that address. A custom `JRS_BASE_PATH` changes the URL prefix.

## Debian package

Install the `.deb` you downloaded. `apt-get` pulls dependencies; `dpkg -i` does not.

```bash
sudo apt-get -y install ./acugis-report-server_<version>_amd64.deb
```

Professional package:

```bash
sudo apt-get -y install ./acugis-report-server-professional_<version>_amd64.deb
```

The package depends on OpenJDK 17, PostgreSQL, Apache, and PHP CLI. On first configure it can create the `jasper` database user and `jasper_reports` database, download Tomcat 10.1 when `/opt/tomcat` is missing, seed the admin user, and start `acugis-report-server.service` and `acugis-tomcat.service`.

Generated database and admin passwords are written to `/etc/acugis-report-server/initial-credentials` when they were not supplied. The service environment is `/etc/acugis-report-server/env`.

Open `http://127.0.0.1:9088/`.

Tomcat’s HTTP connector uses port `8080`. The installer does not restrict that connector to loopback. The package sets Report Server to reach Jasper at:

```text
JRS_JASPER_INTERNAL_REPORT_URL=http://127.0.0.1:8080/report
```

An Open Source install can be replaced later with the Professional `.deb`. That keeps `/etc/acugis-report-server/env` and application data. The Professional package compiles modules the Open Source package does not, including Data Explorer and embedded QGIS before a license is activated. License-gated modules stay off until a license is valid. See the [compatibility matrix](../reference/compatibility-matrix.md). Installing the Open Source package over Professional is refused.

`apt remove` and `apt purge` stop the units and do not drop PostgreSQL or the report files.

## Release tarball

The tarball is a precompiled archive. `install.sh` is the installer copied into that archive. It refuses to run from a source checkout: `bin/jrs-server` must already be present. It does not download the product.

```bash
tar -xzf acugis-report-server-<edition>-<version>-linux-amd64.tar.gz
cd acugis-report-server
sudo ./install.sh
```

`<edition>` is `oss` or `professional`. The installer calls the same setup used by the Debian package: Java 17, Tomcat 10.1, PostgreSQL, the env file, then schema and forward migrations, and the systemd units. Default install root is `/opt/acugis-report-server`.

If `JRS_DB_PASS` and `JRS_ADMIN_PASSWORD` are unset, they are generated into `/etc/acugis-report-server/initial-credentials`.

```bash
sudo JRS_DB_PASS='…' JRS_ADMIN_PASSWORD='…' ./install.sh
```

Open `http://127.0.0.1:9088/`.

## Docker Compose

Edit `.env` and replace the example passwords before the first start. That workflow is documented in [Docker](docker.md). Customer Compose pulls the published `acugis/report-server` image. A local source build uses `compose.build.yml`.

## What the package runs

| Unit | Role |
|------|------|
| `acugis-report-server.service` | Go UI and API on `127.0.0.1:9088` |
| `acugis-tomcat.service` | JasperReports WAR. Tomcat’s HTTP connector uses port `8080`. Report Server calls `http://127.0.0.1:8080/report`. The installer does not bind that connector to loopback. |
| `jrs-schedule-<id>.timer` | Created when a schedule is saved |

## Developer checkout

`make run` from `go-report-server` starts the Go process from source. It is not a product install. The packaged `.deb`, the release tarball, and Docker Compose are the installation methods above.

If `JRS_JASPER_INTERNAL_REPORT_URL` is unset, the Go process falls back to `http://127.0.0.1:9099/report`. That fallback is not used by the Debian package or by Docker Compose. Both of those set port `8080`. When you run Tomcat 10 yourself for a checkout, set the Jasper URL to `http://127.0.0.1:8080/report`.

!!! warning "Local auth bypass"
    `JRS_AUTH_DISABLED=1` injects a synthetic admin for local smoke tests only. Never enable it on a production host.

## Verify

```bash
curl -sS http://127.0.0.1:9088/health
curl -sS http://127.0.0.1:9088/api/openapi.yaml | head
```

The catalog is at `/`, login at `/login.php`, and Swagger at `/swagger`. With `JRS_BASE_PATH=/go`, those paths are under `/go`.

## Next

- [Create an API Token](../getting-started/authentication.md)
- [Quickstart](../getting-started/quickstart.md)
- [Upgrades](upgrades.md)

## Related

- [Configuration](configuration.md)
- [Docker](docker.md)
- [Deployment](deployment.md)
- [Health checks](health-checks.md)
