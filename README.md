# AcuGIS Report Server 1.0.21

AcuGIS Report Server publishes, runs, and manages JasperReports. It provides a web catalog, scheduling, a versioned REST API, and client libraries for automation.

The server application is written in Go. JasperReports runs under Tomcat 10. PostgreSQL stores the application data. A native package deployment puts Apache in front of the Go process. Docker Compose is also supported.

Report Designer Desktop is the separate application used to author JRXML. Designer Web is a separate browser editor that Report Server can proxy when it is installed and licensed on its own. A Report Server Professional license does not license Designer Web.

## Features

- JRXML catalog, publishing, and execution to PDF, Excel, HTML, and [other formats](documentation/docs/reference/supported-formats.md)
- Folders, resources, and report access control
- Scheduling, including email delivery
- REST API and OpenAPI, the `acugis-report` CLI, and SDKs for Python, .NET, Java, and Go
- Report embedding with short-lived embed tokens
- Local accounts, browser sessions, and personal access tokens
- The same datasource administration screen: PostgreSQL, MySQL, Oracle, SQL Server, Snowflake, SQLite, Cassandra, BigQuery, and DuckDB
- Embedded maps for PostGIS, MySQL, and GeoServer
- Audit recording when auditing is enabled
- QGIS, ArcGIS, Data Explorer, dashboards, Map Reports, Dataset Intelligence, audit administration, usage analytics, governance, audit export, OpenID Connect, SAML, and MCP are Professional features. Their source is not in this tree.

Edition boundaries are in the [compatibility matrix](documentation/docs/reference/compatibility-matrix.md). Datasource behavior is in [Supported datasources](documentation/docs/reference/supported-datasources.md).

## Architecture

| Piece | Role |
|-------|------|
| Go Report Server | Web UI and REST API. Packaged installs listen on port 9088. |
| Tomcat 10 | Runs the JasperReports engine that fills reports. |
| PostgreSQL | Application database. |
| Apache | Reverse proxy in the native package deployment. |
| Docker Compose | Runs the server, PostgreSQL, and a scheduler. |

The JasperReports WAR is the fill engine used by the package and by the Docker image. Installing that WAR by itself is not a Report Server installation.

The packaged site uses base path `/`. A native install reaches the Go process at `http://127.0.0.1:9088/`. When Apache is configured, it proxies public traffic to that address. Docker Compose publishes host port 9088.

## Editions

This repository is the AcuGIS Report Server Open Source source tree.

The Open Source server includes the catalog, execution, scheduling, datasources, local login, personal access tokens, report embedding, and embedded maps for PostGIS, MySQL, and GeoServer. Professional modules are not in this source tree. A license file cannot add them.

Professional is a separate proprietary product. Data Explorer, embedded QGIS, Map Reports, dashboards, Dataset Intelligence, audit administration, usage analytics, governance, audit export, OpenID Connect, SAML, and ArcGIS are not included here.

`ACUGIS_EDITION` does not activate Professional features.

See [Licensing](documentation/docs/administration/licensing.md) and the [compatibility matrix](documentation/docs/reference/compatibility-matrix.md).

## Quick start

Supported installs are a local Debian package, a release tarball, or Docker Compose. Full steps, upgrades, and verification are in [Installation](documentation/docs/administration/installation.md).

### Debian or Ubuntu package

Install the `.deb` you downloaded. `apt-get` pulls dependencies.

```bash
sudo apt-get -y install ./acugis-report-server_<version>_amd64.deb
```

The Professional package is a separate proprietary product and is not built from this source tree.

Open `http://127.0.0.1:9088/`.

### Release tarball

The tarball is a precompiled Open Source archive. `<edition>` is `oss`.

```bash
tar -xzf acugis-report-server-<edition>-<version>-linux-amd64.tar.gz
cd acugis-report-server
sudo ./install.sh
```

`install.sh` expects packaged artifacts, including `bin/jrs-server`. It refuses to run from a source checkout.

### Docker Compose

Edit `.env` before the first start. Replace the example passwords. There is no password-generation step.

```bash
git clone https://github.com/AcuGIS/report-server-docker
cd report-server-docker

nano .env
docker compose up -d
```

Set strong, unique values for `JRS_DB_PASS` (PostgreSQL password used by Report Server) and `JRS_ADMIN_PASSWORD` (initial administrator password). `JRS_ADMIN_USER` defaults to `admin`. The example `change-me-*` values cannot be used to start Report Server.

This source repository also keeps Compose files under `docker/` for a local source build. The standard installation is the Docker deployment repository above.

Open `http://127.0.0.1:9088/`.

The Compose file pulls `acugis/report-server:1.0.21-oss`. Professional modules are not in this tree. A local source build is `docker compose -f compose.yml -f compose.build.yml up -d --build`. Optional services are documented in [Docker](documentation/docs/administration/docker.md).

## Documentation

The MkDocs source under [`documentation/docs/`](documentation/docs/index.md) is the authoritative documentation. Preview it locally with the steps in [`documentation/README.md`](documentation/README.md).

| Topic | Page |
|-------|------|
| Product overview | [Overview](documentation/docs/getting-started/overview.md) |
| Run a report | [Quickstart](documentation/docs/getting-started/quickstart.md) |
| Installation | [Installation](documentation/docs/administration/installation.md) |
| Docker | [Docker](documentation/docs/administration/docker.md) |
| Licensing | [Licensing](documentation/docs/administration/licensing.md) |
| Datasources | [Supported datasources](documentation/docs/reference/supported-datasources.md) |
| Editions | [Compatibility matrix](documentation/docs/reference/compatibility-matrix.md) |
| Authentication | [Authentication](documentation/docs/administration/authentication.md) |
| Report Designer Desktop | [Designer overview](documentation/docs/user-guide/designer/overview.md) |
| Designer Web | [Designer Web](documentation/docs/administration/designer-web.md) |

## Development

`make run` in `go-report-server` starts the Open Source Go process (`-tags oss`). That is a developer run, not a product install. See [Developer checkout](documentation/docs/administration/installation.md#developer-checkout).

`install.sh` belongs to the release tarball. It is not a source-build installer.

The root `Makefile` builds the Open Source release artifacts. `package` and `deb` are the Open Source package targets.

## License

AcuGIS Report Server Open Source is licensed under the Apache License, Version 2.0. See [LICENSE](LICENSE). Copyright 2026 AcuGIS.

Professional is a separate proprietary product. Its source is not included in this tree.

Professional feature activation (`license.lic`) applies to that proprietary product. It is not the copyright license for this source tree. See [Licensing](documentation/docs/administration/licensing.md).

The client SDKs in `sdk/python`, `sdk/go`, `sdk/java`, `sdk/dotnet`, and `sdk/javascript` remain under their own MIT License. Copyright AcuGIS. Those MIT licenses do not license the server.
