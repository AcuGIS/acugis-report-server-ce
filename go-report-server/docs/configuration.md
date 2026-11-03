# Configuration

The Go service reuses the PHP application’s `JRS_*` environment variables from
`config.php` and `lib/auth.php`, plus a small set of Go-only development keys.

## Shared with PHP

| Variable | Purpose | PHP default |
|----------|---------|-------------|
| `JRS_DB_HOST` | PostgreSQL host | `localhost` |
| `JRS_DB_PORT` | PostgreSQL port | `5432` |
| `JRS_DB_NAME` | Database name | `jasper_reports` |
| `JRS_DB_USER` | Database user | `postgres` |
| `JRS_DB_PASS` | Database password | *(present in config.php fallback — do not rely on repo defaults in production)* |
| `JRS_DB_SCHEMA` | Schema for app tables | `public` |
| `JRS_JASPER_INTERNAL_REPORT_URL` | Internal execute URL | `http://127.0.0.1:9099/report` |
| `JRS_JASPER_INTERNAL_IMAGE_URL` | Internal image URL | `http://127.0.0.1:9099/acugis_report_image` |
| `JRS_JASPER_HOME` | Install root | PHP service directory when empty |
| `JRS_JRXML_DIR` | JRXML / `.jasper` directory (uploads + map) | `$CATALINA_BASE/acugis/reports` if present, else `{root}/docker/report-server/reports` |
| `JRS_JASPER_CONTEXT_PATH` | Tomcat context.xml path | `$CATALINA_BASE/conf/context.xml` if present, else `{root}/docker/report-server/context.xml` |
| `JRS_JASPER_REPORTS_CONTAINER_DIR` | Path Tomcat sees for map PNGs (`P_MAP_IMAGE`) | defaults to absolute `JRS_JRXML_DIR` (set only for Docker host≠container mounts) |
| `JRS_JASPER_SCHEDULE_DIR` | Schedules directory | `{root}/schedules` |
| `JRS_JASPER_EMAIL_TMPL_DIR` | Email templates | `{root}/email_tmpl` |
| `JRS_AUTH_DISABLED` | Bypass auth (PHP) | unset / false |
| `JRS_SMTP_HOST` | SMTP host for schedule email | empty → PHP `mail()` / Go sendmail |
| `JRS_SMTP_PORT` | SMTP port | `587` |
| `JRS_SMTP_USER` | SMTP username | empty |
| `JRS_SMTP_PASS` | SMTP password | empty |
| `JRS_SMTP_FROM` | From address | empty → user or `jasper-reports@localhost` |
| `JRS_SMTP_HELO` | EHLO/HELO name | domain of `JRS_SMTP_FROM` (not `localhost`) |
| `JRS_SENDMAIL_PATH` | Override sendmail binary (Go) | `/usr/sbin/sendmail` etc. |

## Go development additions

| Variable | Purpose | Default |
|----------|---------|---------|
| `JRS_LISTEN_ADDR` | HTTP bind address | `127.0.0.1:9088` |
| `JRS_BASE_PATH` | URL prefix for routes | `/` |
| `JRS_ENVIRONMENT` | Environment name | `development` |
| `JRS_APP_VERSION` | Reported app version | `1.0.0` |
| `ACUGIS_EDITION` / `JRS_EDITION` | Diagnostic label only. Does not grant Professional | unset |
| `ACUGIS_LICENSE_STATE_DIR` | Installation ID and ACL1 token directory | `/var/lib/acugis-report-server` |
| `ACUGIS_LICENSE_SERVER_URL` | Optional HTTPS license server base URL | `https://license.acugis.com` |
| `JRS_DB_SSLMODE` | pgx sslmode | `disable` |
| `JRS_DB_CONNECT_TIMEOUT` | Connect timeout (`5s` or seconds) | `5s` |
| `JRS_JASPER_TIMEOUT` | Jasper HTTP client timeout | `600s` |
| `JRS_SESSION_COOKIE` | Cookie name (documented for future auth) | `jrs_session` |
| `JRS_REPORT_ENDPOINT` | Viewer iframe execute URL (PHP `report_endpoint`) | `execute_report.php` |
| `JRS_AUDIT_ENABLED` / `AUDIT_ENABLED` | Enable audit trail | `true` |
| `JRS_AUDIT_RETENTION_DAYS` / `AUDIT_RETENTION_DAYS` | Days to retain audit events | `90` |
| `JRS_AUDIT_LOG_PARAMETERS` / `AUDIT_LOG_PARAMETERS` | Store sanitized report parameters in audit details | `true` |
| `JRS_AUDIT_LOG_SQL` / `AUDIT_LOG_SQL` | Allow raw SQL in audit details | `false` |

## Secrets

Never log or return:

- `JRS_DB_PASS`
- Datasource passwords from `jasper_datasource`
- Session tokens / cookies
- SMTP passwords
- Full environment dumps
- Audit details must not contain passwords, bearer tokens, or private keys

See `.env.example` for a safe local template.
Product documentation: Administration → Auditing.
