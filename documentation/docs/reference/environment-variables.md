# Environment variables

This page lists variables the current tree actually reads. Operator narrative is in [Configuration](../administration/configuration.md). `ACUGIS_REPORT_URL` and `ACUGIS_REPORT_TOKEN` are client settings. `jrs-server` does not read them.

`jrs-server` loads optional `.env` files and `/etc/acugis-report-server/env` without overriding variables already present in the process environment. The native unit `acugis-report-server.service` also passes that file with `EnvironmentFile=`.

Truthy flags accept `1`, `true`, `yes`, and `on`. Where an explicit off exists, `0`, `false`, `no`, and `off` turn the flag off. Durations accept a Go duration (`5s`, `15m`, `600s`) or a positive integer number of seconds.

`ACUGIS_EDITION` and `JRS_EDITION` do not grant features. See [Licensing](../administration/licensing.md).

## Core server

Read by `jrs-server` unless noted.

| Variable | Purpose | Default | Notes |
|----------|---------|---------|-------|
| `JRS_LISTEN_ADDR` | HTTP bind address | `127.0.0.1:9088` | Optional. Unset uses that default. The Debian package writes the same address. Docker Compose sets `0.0.0.0:9088` inside the container and publishes host port `9088`. |
| `JRS_BASE_PATH` | Public URL prefix | `/` | Packaged Apache proxies `/`. Use a prefix such as `/go` only when the reverse proxy is mounted there. |
| `JRS_ENVIRONMENT` | Environment name | `development` | The package writes `production`. `development` and `test` add a fallback embed-token secret and, in development, localhost embed origins when origins are unset. |
| `JRS_APP_VERSION` | Reported version, including OpenAPI `info.version` | Code fallback `1.0.0` | Release builds overwrite the compiled fallback. The installer writes the package version and refreshes it on upgrade. Installations do not all report `1.0.0`. |
| `JRS_JASPER_HOME` | Install root used for relative paths | Auto-detected in a checkout | Required when auto-detection fails. The package sets `/opt/acugis-report-server`. |
| `JRS_PROFILE` | Deployment profile | empty (standalone) | `qcarta` selects the QCarta login and catalog cut. `JRS_DEPLOYMENT_PROFILE` is an alias. Other values are ignored. |
| `ACUGIS_EDITION` | Diagnostics label only | `community` when unset | Alias `JRS_EDITION`. Accepted words include `professional` / `commercial` and `community` / `oss`. This does not activate Professional. |
| `JRS_REPORT_ENDPOINT` | Viewer execute URL | `execute_report.php` | Relative values stay under the report viewer. The name is a Go route, not a PHP program. |

## Application database

Read by `jrs-server`. `JRS_DB_HOST`, `JRS_DB_PORT`, `JRS_DB_NAME`, and `JRS_DB_USER` are required. `JRS_DB_PASS` is not required for process validation and is required for a real PostgreSQL login.

| Variable | Purpose | Code default | Package / Compose |
|----------|---------|--------------|-------------------|
| `JRS_DB_HOST` | PostgreSQL host | `localhost` | Compose default `postgres` |
| `JRS_DB_PORT` | Port | `5432` | `5432` |
| `JRS_DB_NAME` | Database name | `jasper_reports` | `jasper_reports` |
| `JRS_DB_USER` | Database user | `postgres` | Package and Compose write `jasper` |
| `JRS_DB_PASS` | Password | empty | First-boot package install generates one when the host is local |
| `JRS_DB_SCHEMA` | Schema | `public` | `public` |
| `JRS_DB_SSLMODE` | `sslmode` | `disable` | `disable` |
| `JRS_DB_CONNECT_TIMEOUT` | Connect timeout | `5s` | unset uses `5s` |
| `JRS_DATABASE_URL` | Full DSN | empty | When set, used instead of the discrete fields. Alias `JRS_DB_URL`. |

Do not log the DSN. It contains the password.

## Jasper / Tomcat

Read by `jrs-server`.

| Variable | Purpose | Default |
|----------|---------|---------|
| `JRS_JASPER_INTERNAL_REPORT_URL` | Internal Jasper execute URL | `http://127.0.0.1:9099/report` only when unset |
| `JRS_JASPER_INTERNAL_IMAGE_URL` | Internal Jasper image URL | `http://127.0.0.1:9099/acugis_report_image` only when unset |
| `JRS_JASPER_TIMEOUT` | HTTP client timeout | `600s` |
| `JRS_JRXML_DIR` | JRXML and `.jasper` directory | `$CATALINA_BASE/acugis/reports` when that directory exists, otherwise `{app}/docker/report-server/reports` |
| `JRS_JASPER_CONTEXT_PATH` | Tomcat `context.xml` written by datasource admin | `$CATALINA_BASE/conf/context.xml` when present, otherwise `{app}/docker/report-server/context.xml` |
| `JRS_JASPER_REPORTS_CONTAINER_DIR` | Path Tomcat uses for map images | The absolute `JRS_JRXML_DIR` when unset |
| `JRS_JASPER_SCHEDULE_DIR` | Schedule files | `{app}/schedules` |
| `JRS_JASPER_EMAIL_TMPL_DIR` | Email templates | `{app}/email_tmpl` |
| `JRS_REPORTS_OWNER` | Owner for JRXML and map-cache writes | `tomcat` |
| `JRS_REPORTS_GROUP` | Group for those writes | `tomcat` |
| `JRS_MYSQL_JDBC_DRIVER` | MySQL driver class written into Jasper context | empty |
| `CATALINA_BASE` | Tomcat base used for path detection | unset |
| `CATALINA_HOME` | Fallback when `CATALINA_BASE` is unset | unset |

The Debian package and Docker Compose set the Jasper report URL to port `8080`, not `9099`. The package uses `http://127.0.0.1:8080/report`. The Compose `report-server` service uses that same URL. The Compose `report-scheduler` service uses `http://report-server:8080/report`. The image URL uses the same host and port with path `/acugis_report_image`.

The package also sets `JRS_JRXML_DIR`, `JRS_JASPER_CONTEXT_PATH`, and `JRS_JASPER_REPORTS_CONTAINER_DIR` to `/opt/tomcat/acugis/reports` and `/opt/tomcat/conf/context.xml`, and sets `JRS_REPORTS_OWNER` / `JRS_REPORTS_GROUP` to the Tomcat user. Compose sets the in-container equivalents under `/usr/local/tomcat` and sets the owner and group to `root`.

## Authentication and sessions

| Variable | Purpose | Default | Read by |
|----------|---------|---------|---------|
| `JRS_SESSION_COOKIE` | Session cookie name | `jrs_session` | `jrs-server` |
| `JRS_AUTH_DISABLED` | Skip login and treat the process as a local admin | off unless `1`/`true`/`yes`/`on` | `jrs-server` |
| `JRS_PAT_PREFIXES` | Extra accepted Bearer prefixes, comma-separated | empty, which means `jrs_` only | `jrs-server` |
| `JRS_AUTH_CHECK_TOKEN` | Token for the `jrs-auth-check` helper | empty | `jrs-auth-check` only |

`JRS_AUTH_DISABLED` is a local bypass. Do not set it on a shared or production listener.

## Personal access tokens

Issued tokens use the prefix `jrs_`. `JRS_PAT_PREFIXES` changes which Bearer prefixes the server accepts. It does not change the prefix of tokens the server creates. There is no separate MCP token type.

## Licensing

| Variable | Purpose | Default | Read by |
|----------|---------|---------|---------|
| `ACUGIS_LICENSE_STATE_DIR` | Directory for `installation-id`, `license.lic`, and `report-designer-web.lic` | `/var/lib/acugis-report-server` | `jrs-server` |
| `ACUGIS_LICENSE_SERVER_URL` | HTTPS license-service base URL | empty, which uses `https://license.acugis.com` | `jrs-server` |
| `ACUGIS_DESIGNER_WEB_VERSION` | Product version sent for Report Designer Web license calls | `1.0.0` when unset | `jrs-server` license startup |

Compose sets `ACUGIS_LICENSE_STATE_DIR` to `/opt/acugis-report-server/var/license` on the `report-server` and `report-scheduler` services. A non-HTTPS override is rejected. Details: [Licensing](../administration/licensing.md).

## Embedding

| Variable | Purpose | Default |
|----------|---------|---------|
| `ACUGIS_EMBED_ALLOWED_ORIGINS` | Comma-separated origins merged with Administration → Embedded Analytics. Parsing is unchanged. The Admin UI is the normal way to add origins; this variable stays supported and is read-only there. | empty; development adds common localhost origins |
| `ACUGIS_EMBED_TOKEN_SECRET` | Secret that signs `emb_` tokens | empty; `development` and `test` use a fixed dev fallback |
| `ACUGIS_EMBED_TOKEN_TTL` | Default embed-token lifetime | `15m` |

Aliases: `JRS_EMBED_ALLOWED_ORIGINS`, `JRS_EMBED_TOKEN_SECRET`, `JRS_EMBED_TOKEN_TTL`. The package generates `ACUGIS_EMBED_TOKEN_SECRET` on first boot when neither name is already set. Production must set a real secret. See [Embedding security](../developer/embedding/security.md).

Admin-saved origins are stored in `embed_allowed_origin` and do not require this variable. The effective allow-list is the union of the environment value and those saved origins. Changing the variable still requires a process restart. Adding or removing an origin in Administration → Embedded Analytics does not.

## OpenID Connect

Read by the Professional binary. They override Administration → Authentication when set. They are absent from the Open Source binary's OIDC module. A valid Report Server Professional license is still required before OIDC login runs. This Open Source documentation does not include the OpenID Connect setup page.

| Variable | Purpose | Default when unset |
|----------|---------|--------------------|
| `ACUGIS_LOCAL_AUTH_ENABLED` | Override local username/password | database setting; initial default on |
| `ACUGIS_OIDC_ENABLED` | Override the OIDC enabled flag | database setting; initial default off |
| `ACUGIS_OIDC_ISSUER` | Issuer URL | database |
| `ACUGIS_OIDC_CLIENT_ID` | Client ID | database |
| `ACUGIS_OIDC_CLIENT_SECRET` | Client secret | database; an env secret is not returned by the admin UI |
| `ACUGIS_OIDC_SCOPES` | Space-separated scopes | `openid profile email` |
| `ACUGIS_OIDC_USERNAME_CLAIM` | Username claim | `preferred_username` |
| `ACUGIS_OIDC_EMAIL_CLAIM` | Email claim | `email` |
| `ACUGIS_OIDC_NAME_CLAIM` | Display-name claim | `name` |
| `ACUGIS_OIDC_PROVIDER_NAME` | Login-button label | database; UI falls back to `SSO` |

There is no environment variable for the groups claim, auto-provision, email linking, or IdP logout. Those stay in the admin settings.

## SAML

Read by the Professional binary. They override Administration → Authentication when set. A valid Report Server Professional license is still required. This Open Source documentation does not include the SAML setup page.

| Variable | Purpose |
|----------|---------|
| `ACUGIS_SAML_ENABLED` | Override the SAML enabled flag |
| `ACUGIS_SAML_PROVIDER_NAME` | Login-button label |
| `ACUGIS_SAML_IDP_METADATA_URL` | IdP metadata URL |
| `ACUGIS_SAML_IDP_ENTITY_ID` | IdP entity ID |
| `ACUGIS_SAML_IDP_SSO_URL` | IdP SSO URL |
| `ACUGIS_SAML_SP_ENTITY_ID` | SP entity ID |
| `ACUGIS_SAML_ACS_URL` | Assertion Consumer Service URL |
| `ACUGIS_SAML_NAMEID_ATTRIBUTE` | Subject attribute (default `NameID`) |
| `ACUGIS_SAML_USERNAME_ATTRIBUTE` | Username attribute |
| `ACUGIS_SAML_EMAIL_ATTRIBUTE` | Email attribute (default `email`) |
| `ACUGIS_SAML_DISPLAY_NAME_ATTRIBUTE` | Display-name attribute (default `displayName`) |
| `ACUGIS_SAML_CLOCK_SKEW` | Clock-skew seconds (default `120`) |
| `ACUGIS_SAML_SP_KEY_FILE` | Path to the SP private key PEM |
| `ACUGIS_SAML_SP_CERT_FILE` | Path to the SP certificate PEM |

If a key or certificate path is set and the file cannot be read, that material is not loaded. If one of the two files is set and the other is missing, SAML startup reports the pair as incomplete instead of generating a replacement key.

## Designer Web

Read by `jrs-server`. Both `JRS_DESIGNER_UPSTREAM` and `JRS_DESIGNER_INTERNAL_TOKEN` must be set together, or both left empty. An empty upstream leaves Designer routes inactive. The upstream must be an `http` or `https` URL on `127.0.0.1`, `localhost`, or `::1`, with no userinfo and no path. See [Designer Web](../administration/designer-web.md).

| Variable | Purpose | Default |
|----------|---------|---------|
| `JRS_DESIGNER_UPSTREAM` | Loopback Designer process | empty |
| `JRS_DESIGNER_INTERNAL_TOKEN` | Shared secret sent to that process | empty; no production default |
| `JRS_DESIGNER_PROXY_TIMEOUT` | Proxy timeout | `120s` |
| `JRS_DESIGNER_PROXY_MAX_BODY` | Maximum proxied body | 64 MiB (`64MB` is accepted; `KB`, `MB`, and `GB` suffixes too) |
| `JRS_DESIGNER_ASSETS_DIR` | Designer Web files served at `/designer-assets/` to a signed-in, licensed Designer user | `{app root}/designer-web` |

A Report Server Professional license does not set these variables and does not license Designer Web.

## Auditing

Read by `jrs-server`. Recording follows these flags on both packages. The audit administration UI is a separate license gate. See [Auditing](../administration/auditing.md).

| Variable | Purpose | Default |
|----------|---------|---------|
| `JRS_AUDIT_ENABLED` | Record audit events | `true` |
| `JRS_AUDIT_RETENTION_DAYS` | Retention window | `90` |
| `JRS_AUDIT_LOG_PARAMETERS` | Store sanitized report parameters | `true` |
| `JRS_AUDIT_LOG_SQL` | Store raw SQL | `false` |

Prefer the `JRS_AUDIT_*` names. `AUDIT_ENABLED`, `AUDIT_RETENTION_DAYS`, `AUDIT_LOG_PARAMETERS`, and `AUDIT_LOG_SQL` are accepted aliases of those four variables. They are not a second configuration scheme.

## SMTP / mail

Read by the schedule mailer (`jrs-server` schedule actions and `jrs-run-schedule`), not by `config.Load`.

| Variable | Purpose | Default |
|----------|---------|---------|
| `JRS_SMTP_HOST` | SMTP host | empty, which falls back to local sendmail |
| `JRS_SMTP_PORT` | SMTP port | `587` |
| `JRS_SMTP_USER` | SMTP user | empty |
| `JRS_SMTP_PASS` | SMTP password | empty |
| `JRS_SMTP_FROM` | From address | empty |
| `JRS_SMTP_HELO` | EHLO/HELO name | domain of `JRS_SMTP_FROM` when unset |
| `JRS_SENDMAIL_PATH` | Sendmail binary | the usual system path when unset |

## Scheduler

`JRS_SCHEDULE_RUNTIME` selects the runtime. Unset or any value other than `dispatcher` means `systemd`, which is the native package behavior. Compose sets `dispatcher` on both the server and scheduler containers.

| Variable | Purpose | Default | Read by |
|----------|---------|---------|---------|
| `JRS_SCHEDULE_RUNTIME` | `systemd` or `dispatcher` | `systemd` | scheduler code |
| `JRS_SCHEDULER_POLL_INTERVAL` | Dispatcher poll interval | `15s` | `jrs-schedule-dispatcher` |
| `JRS_SCHEDULER_LISTEN_ADDR` | Dispatcher `GET /health` bind | `:9089` | `jrs-schedule-dispatcher` |
| `JRS_SCHEDULE_RUNNER` | Command the dispatcher executes | a sibling `jrs-run-schedule` when present | dispatcher |
| `JRS_JASPER_SYSTEMD_UNIT_DIR` | Generated unit files | `/opt/acugis-report-server/systemd` | systemd schedule manager |
| `JRS_JASPER_SYSTEMD_SYSTEM_DIR` | systemd unit install directory | `/etc/systemd/system` | systemd schedule manager |
| `JRS_JASPER_SYSTEMCTL` | `systemctl` binary | `/bin/systemctl` | systemd schedule manager |
| `JRS_JASPER_SYSTEMD_USE_SUDO` | Call `systemctl` through sudo | on, unless `0`/`false`/`no`/`off` | systemd schedule manager |
| `JRS_SYSTEMD_ENV_FILE` | Extra env file for schedule units, and an extra dotenv path | `/etc/acugis-report-server/env` | systemd manager and `jrs-server` dotenv loader |
| `TZ` | Process timezone for calendar schedules | the container or host zone | process; Compose default `UTC` |

Native systemd units invoke `{JRS_JASPER_HOME}/scripts/run_schedule.php` through PHP. The dispatcher path uses `JRS_SCHEDULE_RUNNER` and does not use that PHP script. The package writes `JRS_JASPER_SYSTEMD_USE_SUDO=true` and `JRS_JASPER_SYSTEMD_UNIT_DIR` under the install root.

## QGIS

| Variable | Purpose | Default | Read by |
|----------|---------|---------|---------|
| `JRS_QGIS_UPSTREAM` | QGIS Server base URL | empty, which leaves `/qgis/*` without an upstream | `jrs-server` (Professional binary mounts the proxy) |
| `JRS_QGIS_MAP_PREFIX` | Prefix rewritten into QGIS `MAP` paths | `/qgis/projects` | `jrs-server` |
| `JRS_QGIS_CONNECTIONS_PATH` | QGIS connection properties file | `{app}/docker/report-server/qgis-connections.properties` | `jrs-server` |

The package sets the properties path to `/opt/tomcat/conf/qgis-connections.properties`. Compose sets `/usr/local/tomcat/conf/qgis-connections.properties` and defaults the upstream to `http://qgis-server`.

## GeoServer and ArcGIS connection storage

| Variable | Purpose | Code fallback | Package path |
|----------|---------|---------------|--------------|
| `JRS_GEOSERVER_CONNECTIONS_PATH` | GeoServer connection properties | `{app}/docker/report-server/geoserver-connections.properties` | `/opt/tomcat/conf/geoserver-connections.properties` |
| `JRS_ARCGIS_CONNECTIONS_PATH` | ArcGIS connection properties | `{app}/docker/report-server/arcgis-connections.properties` | `/opt/tomcat/conf/arcgis-connections.properties` |

Compose uses `/usr/local/tomcat/conf/` for both files. These files store connection secrets. ArcGIS entitlement is separate from the file path: the Open Source package serves the ArcGIS admin route as not found, and an unlicensed Professional process denies the ArcGIS provider.

## Resources and folders

| Variable | Purpose | Default |
|----------|---------|---------|
| `JRS_RESOURCES_DIR` | Non-image resources | `{app}/resources` |
| `JRS_RESOURCE_THUMBNAILS_DIR` | Image-resource thumbnails | `{app}/thumbnails` |
| `JRS_RESOURCE_PATHS_DIR` | Path-addressable image resources for Jasper | sibling `resource-paths` of the JRXML directory |
| `JRS_FOLDER_ALLOW_RECURSIVE_DELETE` | Allow `DELETE /api/v1/folders/{id}?recursive=1&confirm=1` | off unless `1`/`true`/`yes`/`on` |
| `JRS_RESOURCE_SYNC_VERBOSE` | Extra resource-sync log lines | off |

The package sets `JRS_RESOURCE_PATHS_DIR` to `/opt/tomcat/acugis/resource-paths`.

## QCarta / Central

| Variable | Purpose | Default |
|----------|---------|---------|
| `JRS_PROFILE` | Set to `qcarta` | empty |
| `JRS_CENTRAL_LOGIN_URL` | External login URL | empty, which keeps `/login` |
| `JRS_AUTH_DATABASE_URL` | Separate auth database URL | empty |

Aliases: `JRS_DEPLOYMENT_PROFILE`, `CENTRAL_LOGIN_URL`, `CENTRAL_DATABASE_URL`. When the profile is `qcarta`, or when a central login URL is set, local login and logout redirect to that Central URL. This is a deployment cut. It does not change edition.

## DuckDB home

| Variable | Purpose | Default |
|----------|---------|---------|
| `ACUGIS_DUCKDB_HOME` | DuckDB extension home | alias `DUCKDB_HOME`, then a directory under the user home, cache, or temp |
| `ACUGIS_DUCKDB_HTTPFS` | Optional httpfs extension path for the JDBC session | empty |

Compose sets `ACUGIS_DUCKDB_HOME` to `/opt/acugis-report-server/var/duckdb`.

## Installer-only variables

`scripts/native-setup.sh` and the release `install.sh` read these. `jrs-server` does not use them as feature switches. `JRS_ADMIN_USER` and `JRS_ADMIN_PASSWORD` seed the first administrator. They are written to `/etc/acugis-report-server/env` and the first-boot file `/etc/acugis-report-server/initial-credentials`.

| Variable | Purpose | Default |
|----------|---------|---------|
| `JRS_INSTALL_DIR` | Install root | `/opt/acugis-report-server` |
| `JRS_TOMCAT_HOME` | Tomcat directory | `/opt/tomcat` |
| `JRS_TOMCAT_VERSION` | Tomcat release downloaded when missing | `10.1.57` |
| `JRS_APACHE_CONF` | Apache snippet path | `/etc/apache2/conf-available/acugis-report-server.conf` |
| `JRS_SYSTEMD_ENV_FILE` | Env file path | `/etc/acugis-report-server/env` |
| `JRS_CREDENTIALS_FILE` | First-boot password file | `/etc/acugis-report-server/initial-credentials` |
| `JRS_SERVICE_USER` | `jrs-server` user | `www-data` |
| `JRS_TOMCAT_USER` | Tomcat user | `tomcat` |
| `JRS_ADMIN_USER` | Seeded administrator | `admin` |
| `JRS_ADMIN_PASSWORD` | Seeded administrator password | generated on first local boot when empty |
| `JRS_SKIP_TOMCAT` | Skip Tomcat download and permission fix | `0` |
| `JRS_SKIP_DB` | Skip database setup | `0` |
| `JRS_SKIP_APACHE` | Skip Apache snippet | `0` |
| `JRS_APACHE_OVERWRITE` | Replace an existing Apache snippet | `1` for the tarball; Debian postinst uses `0` |
| `JRS_EDITION_STATE` | Installer edition marker path | `/var/lib/acugis-report-server/edition` |
| `JRS_SUDOERS_FILE` | sudoers drop-in for schedule units | `/etc/sudoers.d/acugis-report-server` |

The installer does not replace an existing env file. It refreshes `JRS_APP_VERSION`, and on upgrade it sets `JRS_JASPER_SYSTEMD_USE_SUDO=true` when that value is missing or false. It does not write `installation-id` or `license.lic`.

## Docker entrypoint-only variables

These are read by `docker/report-server/docker-entrypoint.sh` or by Compose. They are not `jrs-server` configuration. Do not treat them as feature flags inside the Go process.

| Variable | Purpose | Default |
|----------|---------|---------|
| `ACUGIS_DOCKER_ROLE` | `scheduler` selects the dispatcher process | empty (combined server) |
| `ACUGIS_DOCKER_SEED_DEMO` | Seed demo content on first boot | `1` |
| `ACUGIS_DOCKER_IMAGE_ROOT` | Image tree copied into the data volume | `/opt/acugis-image` |
| `ACUGIS_DOCKER_CONF_PERSIST` | Persisted Tomcat conf directory | `/var/acugis/tomcat-conf` |
| `ACUGIS_BUILD_EDITION` | Which schema file the entrypoint applies | `oss` |
| `JRS_HOST_PORT` | Host port published to container `9088` | `9088` (Compose only) |
| `JRS_DATA_HOST_PATH` | Host directory mounted at `/data` | `./data` (Compose only) |

Compose also sets `ACUGIS_REPORTS_DIR`, `ACUGIS_UPLOADS_DIR`, `ACUGIS_FONTS_DIR`, `ACUGIS_DRIVERS_DIR`, `ACUGIS_LICENSES_DIR`, `ACUGIS_TEMP_DIR`, and `ACUGIS_RESOURCE_PATHS_DIR`. `jrs-server` config loading does not read those names. The Go process uses the `JRS_*` path variables above.

`BUILD_EDITION` is `oss` for this source tree. The image compiles with `-tags oss`. It is not read by a running `jrs-server` as a license.

Docker Compose does not generate `JRS_DB_PASS` or `JRS_ADMIN_PASSWORD`. Set both in `.env` before the first `docker compose up`. Startup refuses the example values `change-me-strong-password` and `change-me-admin-password`, and it refuses an empty value. Native package installs still generate a local password on first boot when the variable is empty.

## Client variables

Read by the CLI and SDKs. Not read by `jrs-server`.

| Variable | Purpose |
|----------|---------|
| `ACUGIS_REPORT_URL` | Report Server base URL. Use the public origin. Add a path only when `JRS_BASE_PATH` is not `/`. |
| `ACUGIS_REPORT_TOKEN` | Bearer personal access token (`jrs_…`) |

## Related

- [Configuration](../administration/configuration.md)
- [Licensing](../administration/licensing.md)
- [Designer Web](../administration/designer-web.md)
- [Security](../administration/security.md)
