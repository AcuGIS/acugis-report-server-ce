# Current contracts

Behavioral contracts extracted from the existing PHP application. Items that
could not be confirmed from code are marked **Needs verification**.

## Application mount path

- Config key: `app_mount` in `config.php`
- Default: `/acugis-report-server`
- Helper: `jrs_app_mount()` in `lib/bootstrap.php`
- Apache rewrite (`.htaccess`): `reports/{key}` → `reports/view.php?report={key}`

## Cookie and session

| Item | Value |
|------|--------|
| Cookie name | `jrs_session` (`JRS_AUTH_SESSION_COOKIE`) |
| Cookie contents | Raw session token (base64url-ish random bytes from `random_bytes(32)`) |
| DB column | `dashboard_sessions.token_hash` = SHA-256(raw), **lowercase hex** (`hash('sha256', $raw, false)`) |
| Cookie path | `/` |
| Cookie domain | not set (host-only) |
| Cookie flags | `HttpOnly`, `SameSite=Lax`, `Secure` when HTTPS / `X-Forwarded-Proto=https` |
| TTL | 604800 seconds (7 days) at **creation**; PHP does not refresh `expires_at` on use |
| Multiple sessions | Supported (insert-only; no unique constraint on `username`) |
| Revoked sessions | Logout deletes the row by `token_hash`; no separate revoked/disabled session flag |
| Go writes | Login inserts; logout deletes (same SQL as PHP); validation remains read-only |
| Auth bypass | `JRS_AUTH_DISABLED` truthy → synthetic `local-dev` admin (`id=0`, `is_admin=true`) |
| Bearer PATs | Go: `Authorization: Bearer jrs_…` via `personal_access_tokens` (Cloud-compatible). See `docs/authentication.md`. |

### Session lookup SQL (exact PHP predicates)

```sql
FROM dashboard_sessions s
JOIN dashboard_users u ON u.username = s.username
WHERE s.token_hash = $1
  AND s.expires_at > NOW()
  AND COALESCE(u.enabled, TRUE) = TRUE
```

Disabled users fail the lookup (treated as unauthenticated). Invalid or expired
cookies on report runtime paths become anonymous via
`jrs_auth_try_refresh_current_user` (no redirect). `jrs_require_login` returns
401 JSON for API requests or redirects HTML to login.

Go mirrors this: `OptionalAuthentication` / `RequireAuthentication` in
`internal/auth`. Go never creates, refreshes, or deletes sessions.

Source: `lib/auth.php`.

## Authentication storage

- Users: `dashboard_users` (password_hash via PHP `password_verify`; Go never loads hashes into auth models)
- Sessions: `dashboard_sessions`
- Groups: `auth_groups`, `auth_group_members`
- Same application PostgreSQL database as report metadata (`jrs_app_db()`)

## ACL levels

- Permissions: `view`, `edit`, `admin`
- `edit` satisfies `view`; `admin` satisfies any requested permission
- Resource types:
  - `jasper_report` + report key (same key as report JSON / `_repName`)
  - `service` + key `jasper_reports` (catalog / UI gate)
- Default new-report grants: Administrators=`admin`, Editors=`edit`, Public=`view`
- Platform admin: `dashboard_users.is_admin` or admin-named groups
  (`administrator`, `administrators`, `admin`, `admins`, `dashboard-admin`)
- **No per-user ACL rows** — only group grants in `resource_permissions`

### Verified ACL precedence (`jrs_has_permission` + callers)

1. `JRS_AUTH_DISABLED` → allow all
2. `dashboard_users.is_admin` → allow (checked by callers such as
   `jrs_require_report_runtime_permission`)
3. Admin-named groups → allow
4. Group grant on `resources` / `resource_permissions`, including **implicit
   Public** membership (`jrs_auth_effective_group_names_for_permission`)
5. Default deny

Missing resource row or missing ACL row → deny (EXISTS returns false) → **403**,
not 404. Report execute returns **404** only when report JSON is missing
(`jrs_read_report`), before ACL.

`execute_report.php` / `report_image.php` enforce **jasper_report view only**,
not service-level `jasper_reports` access.

## Public group behavior

- Permission checks always include an implicit `Public` group membership via
  `jrs_auth_effective_group_names_for_permission()`
- Anonymous report runtime checks use user id `0` with Public still applied
  (`jrs_require_report_runtime_permission`)

## Report image authorization

There is **no cryptographic image authorization key**.

| Item | Behavior |
|------|----------|
| Identity param | `_repName` (preferred) or `report` |
| Validation | `jrs_is_valid_report_key` → else **403** empty body |
| Access rule | Same as report runtime: `view` on `jasper_report` |
| HTML rewrite | Execute HTML replaces `/report_image.php?image=` with `../report_image.php?_repName=…&image=` |
| Key reuse | N/A (ACL + report name each request) |

## Database schema location

- Primary schema: `database/schema.sql`
- Migrations: `database/migrations/`, `database/migrate_datasource_db_type.sql`
- Schema name: `JRS_DB_SCHEMA` (default `public`)

## Report JSON location

- `{install_root}/reports/json/{report}.report.json`
- Helper: `jrs_json_dir()` / `jrs_report_json_path()`

## JRXML location

- `{install_root}/docker/report-server/reports/{report}.jrxml`
- Compiled sibling: `{report}.jasper`
- Docker volume maps this directory into Tomcat

## Thumbnail and assets

- `{install_root}/assets/reports/{report}.png`

## Tomcat / Jasper internal URLs

From `config.php` (defaults verified in code):

| Purpose | Env | Default |
|---------|-----|---------|
| Report execution | `JRS_JASPER_INTERNAL_REPORT_URL` | `http://127.0.0.1:9099/report` |
| Report images | `JRS_JASPER_INTERNAL_IMAGE_URL` | `http://127.0.0.1:9099/acugis_report_image` |
| Context.xml | `JRS_JASPER_CONTEXT_PATH` | `{install_root}/docker/report-server/context.xml` |

Docker Compose publishes host `9099` → container Tomcat `8080`.

No dedicated harmless Jasper health endpoint was found in the PHP application.
Go readiness therefore reports Jasper connectivity as **not checked / unknown**.

## Schedule execution

- UI / actions: `schedules.php`, `edit_schedule.php`, `action/schedule.php`
- Runner: `scripts/run_schedule.php`
- Per-schedule env: `{schedule_dir}/{id}_env.sh`
- Output: `{schedule_dir}/output/{id}/`
- Timers: systemd units under `JRS_JASPER_SYSTEMD_UNIT_DIR` (default
  `/opt/acugis-report-server/systemd`)
- Schedule dir default: `{JRS_JASPER_HOME or install_root}/schedules`

## Email configuration

- SMTP: `JRS_SMTP_HOST`, `JRS_SMTP_PORT`, `JRS_SMTP_USER`, `JRS_SMTP_PASS`, `JRS_SMTP_FROM`
- Templates: `{JRS_JASPER_EMAIL_TMPL_DIR or install_root/email_tmpl}/`
- Not stored as DB rows (comment in `database/schema.sql`)

## Datasource context generation

- PHP regenerates Tomcat JNDI resources into `context.xml` via
  `reports/generate_jasper_context.php` / related helpers in `lib/bootstrap.php`
  (`jrs_jdbc_settings_for_datasource`)

## Map Report endpoints

| File | Typical URL under mount |
|------|-------------------------|
| `reports/map_columns.php` | `/acugis-report-server/reports/map_columns.php` |
| `reports/map_extent.php` | `/acugis-report-server/reports/map_extent.php` |
| `reports/map_features.php` | `/acugis-report-server/reports/map_features.php` |

## Environment variables (observed)

`JRS_DB_HOST`, `JRS_DB_PORT`, `JRS_DB_NAME`, `JRS_DB_USER`, `JRS_DB_PASS`,
`JRS_DB_SCHEMA`, `JRS_LOGIN_URL`, `JRS_REPORT_ENDPOINT`,
`JRS_JASPER_CONTEXT_PATH`, `JRS_JASPER_INTERNAL_REPORT_URL`,
`JRS_JASPER_INTERNAL_IMAGE_URL`, `JRS_JASPER_HOME`, `JRS_JASPER_SCHEDULE_DIR`,
`JRS_JASPER_EMAIL_TMPL_DIR`, `JRS_JASPER_SYSTEMCTL`, `JRS_JASPER_SYSTEMD_USE_SUDO`
(Needs verification: whether env override is wired in PHP — config.php hardcodes
`jasper_systemd_use_sudo` to `true`), `JRS_JASPER_SYSTEMD_UNIT_DIR`,
`JRS_JASPER_SYSTEMD_SYSTEM_DIR`, `JRS_SMTP_*`, `JRS_AUTH_DISABLED`,
`JRS_AUTH_DEBUG`, `JRS_AUTH_LOGIN_TRACE`, `JRS_ADMIN_USER`, `JRS_ADMIN_PASSWORD`,
`JRS_RESOURCE_SYNC_VERBOSE`, `JRS_INSTALL_DIR`.

Go-only additions for the development server (documented in
`docs/configuration.md`): `JRS_LISTEN_ADDR`, `JRS_BASE_PATH`, `JRS_ENVIRONMENT`,
`JRS_APP_VERSION`, `JRS_DB_SSLMODE`, `JRS_DB_CONNECT_TIMEOUT`,
`JRS_JASPER_TIMEOUT`, `JRS_SESSION_COOKIE`, `JRS_AUTH_CHECK_TOKEN` (local
`jrs-auth-check` only).

## Remaining PHP ↔ Go differences (Phase 3)

- Production execute/image URLs still served by PHP
- Go does not create sessions, refresh expiry, or logout
- Go development APIs never redirect to login (JSON 401 only for
  `RequireAuthentication` routes; report runtime uses 403 like PHP)
- HTML image URL rewrite targets `/dev/report-image` on Go instead of
  `../report_image.php`
- Live session interop can be probed with `cmd/jrs-auth-check` (stdin / env token)
