# Configuration

Report Server is configured primarily through `JRS_*` environment variables (shared with the PHP-era config surface) plus Go listen/base-path settings.

## Application database

| Variable | Purpose | Typical default |
|----------|---------|-----------------|
| `JRS_DB_HOST` | PostgreSQL host | Code fallback and native package: `localhost`. Docker Compose sets `postgres` unless you override it. |
| `JRS_DB_PORT` | Port | `5432` |
| `JRS_DB_NAME` | Database name | `jasper_reports` |
| `JRS_DB_USER` | User | Code fallback `postgres`. The Debian package writes `jasper`. Docker `.env.example` and the Compose PostgreSQL service also use `jasper`. |
| `JRS_DB_PASS` | Password | *(set explicitly in production)* |
| `JRS_DB_SCHEMA` | Schema | `public` |
| `JRS_DB_SSLMODE` | pgx sslmode | `disable` |
| `JRS_DB_CONNECT_TIMEOUT` | Connect timeout | `5s` |

## Jasper / Tomcat integration

| Variable | Purpose |
|----------|---------|
| `JRS_JASPER_INTERNAL_REPORT_URL` | Internal execute URL. See the deployment values below. |
| `JRS_JASPER_INTERNAL_IMAGE_URL` | Internal image URL, paired with the execute URL |
| `JRS_JASPER_HOME` | Install root |
| `JRS_JRXML_DIR` | JRXML / `.jasper` directory |
| `JRS_JASPER_CONTEXT_PATH` | Tomcat `context.xml` path |
| `JRS_JASPER_REPORTS_CONTAINER_DIR` | Path Tomcat sees for map PNGs (`P_MAP_IMAGE`) |
| `JRS_JASPER_SCHEDULE_DIR` | Schedules directory |
| `JRS_JASPER_EMAIL_TMPL_DIR` | Email templates |
| `JRS_JASPER_TIMEOUT` | Jasper HTTP client timeout (default `600s`) |
| `JRS_REPORT_ENDPOINT` | Viewer iframe execute URL |

### Jasper URL by deployment

| Deployment | Report URL |
|------------|------------|
| Variable unset (developer fallback only) | `http://127.0.0.1:9099/report` |
| Debian package and release tarball | `http://127.0.0.1:8080/report` |
| Docker `report-server` | `http://127.0.0.1:8080/report` |
| Docker `report-scheduler` | `http://report-server:8080/report` |

The image URL uses the same host and port with path `/acugis_report_image`. Packaged Tomcat is 10 on port `8080`. Do not set a production package or Compose stack to port `9099`.

## Go service

| Variable | Purpose | Default |
|----------|---------|---------|
| `JRS_LISTEN_ADDR` | HTTP bind address | `127.0.0.1:9088` (code and native package). Docker Compose sets `0.0.0.0:9088` inside the container and publishes host port `9088`. |
| `JRS_BASE_PATH` | URL prefix | `/` |
| `JRS_ENVIRONMENT` | Environment name | Code fallback `development`. The Debian package writes `production`. |
| `JRS_APP_VERSION` | Reported / OpenAPI version | Code fallback `1.0.0` when unset. Packaged builds set the version at compile time, and the installer writes that package version into the env file. An upgrade refreshes it. Installations do not all report `1.0.0`. |
| `JRS_SESSION_COOKIE` | Cookie name | `jrs_session` |
| `JRS_AUTH_DISABLED` | Local auth bypass | unset / false |
| `JRS_AUDIT_ENABLED` | Enable audit trail | `true` |
| `JRS_AUDIT_RETENTION_DAYS` | Days to retain audit events | `90` |
| `JRS_AUDIT_LOG_PARAMETERS` | Store sanitized report parameters in audit details | `true` |
| `JRS_AUDIT_LOG_SQL` | Allow raw SQL in audit details | `false` |

`AUDIT_*` aliases for the same settings are also accepted.

## SMTP (schedules)

| Variable | Purpose |
|----------|---------|
| `JRS_SMTP_HOST` | SMTP host |
| `JRS_SMTP_PORT` | Port (default `587`) |
| `JRS_SMTP_USER` / `JRS_SMTP_PASS` | Credentials |
| `JRS_SMTP_FROM` | From address |
| `JRS_SENDMAIL_PATH` | Override sendmail binary |

## Secrets

Never log or return:

- `JRS_DB_PASS`
- Datasource passwords
- Session tokens / cookies
- SMTP passwords
- Embed-token secret (`ACUGIS_EMBED_TOKEN_SECRET`)
- OIDC client secret and SAML private keys
- `JRS_DESIGNER_INTERNAL_TOKEN`
- License files under the license state directory (`license.lic`, `report-designer-web.lic`)
- Full environment dumps
- API Token plaintext / full hashes

The complete variable list is [Environment variables](../reference/environment-variables.md). License and Designer Web have administration pages in this guide. OpenID Connect and SAML are available in Professional and are not part of this Open Source documentation. `ACUGIS_EDITION` does not enable features.

## Related

- [Licensing](licensing.md)
- [Designer Web](designer-web.md)
- [Auditing](auditing.md)
- [Environment variables reference](../reference/environment-variables.md)
- [Security](security.md)
