# Database contract

Schema source of truth: `database/schema.sql` plus incremental files under
`database/migrations/` and `database/migrate_datasource_db_type.sql`.

Default schema: `public` (`JRS_DB_SCHEMA`).

The Go foundation originally treated the database as **read-only**. Publish/edit
now perform the same ACL resource writes PHP does (`resources` /
`resource_permissions` via `SyncReportAccessGroups`). Session rows remain
read-only. Schema migrations and seeds stay out of scope.

## Tables by feature

### Authentication

| Table | Responsibility |
|-------|----------------|
| `dashboard_users` | User accounts, password hashes, `is_admin`, enabled flag |
| `dashboard_sessions` | Session token hashes and expiry |
| `personal_access_tokens` | Bearer PATs (`jrs_…`); SHA-256 hash + prefix; revoke/expiry/last_used |
| `auth_groups` | Named groups (Administrators, Editors, Public, custom) |
| `auth_group_members` | User ↔ group membership |

Reads: login, session validation, PAT validation, admin UI, **Go auth repository**.  
Writes: login/logout session create/destroy (**Go + PHP**); PAT create/revoke/update
(**Go**); seed remains PHP; **Go admin UI** writes users/groups/memberships (dev under `/go`).

**Session token contract:** cookie holds the raw token; `token_hash` stores
SHA-256(raw) as lowercase hex. Go `auth.HashSessionToken` matches PHP
`jrs_auth_session_token_hash`. Go login inserts and logout deletes session rows
the same way PHP does.

**PAT contract:** plaintext `jrs_…` returned once at creation; DB stores hash +
12-char `token_prefix`. Migration: `database/migrations/007_personal_access_tokens.sql`.
Auth order: Bearer (if present) → session cookie → anonymous.

### Auditing

| Table | Responsibility |
|-------|----------------|
| `audit_log` | Append-only business/security audit events (who/what/when/status/duration) |

Migration: `database/migrations/010_audit_log.sql`. Go ensures the table at
startup when auditing is enabled. Retention cleanup deletes rows older than
`JRS_AUDIT_RETENTION_DAYS` (default 90). Never store credentials or tokens in
`details`.

### Audit Export (Professional)

| Table | Responsibility |
|-------|----------------|
| `audit_export_destination` | External forwarder destinations (webhook / Splunk HEC / syslog) and health |
| `audit_export_cursor` | Per-destination delivery position into `audit_log` (no event payload copy) |

Migration: `database/migrations/015_audit_export.sql`. Go ensures schema at
startup. Delivery is at-least-once with a 7-day maximum delivery age; local
`audit_log` retention remains independent.

### Embedded Analytics allowed origins

| Table | Responsibility |
|-------|----------------|
| `embed_allowed_origin` | Origins saved in Administration → Embedded Analytics |

Migration: `database/migrations/017_embed_allowed_origins.sql`. Go also calls
`EnsureSchema` at startup. Rows survive restart and package upgrade with the
application database. `ACUGIS_EMBED_ALLOWED_ORIGINS` /
`JRS_EMBED_ALLOWED_ORIGINS` are not written to this table. The request path
uses the union of those environment values and these rows.

### ACL / resources

| Table | Responsibility |
|-------|----------------|
| `resources` | Catalog of `jasper_report`, `service`, and `content_folder` resources |
| `resource_permissions` | Group permissions (`view` / `edit` / `admin`) on resources |

Reads: every gated page/API, **Go ACL repository (Phase 3)**.  
Writes (PHP + Go publish/edit): resource registration and access-group sync
(`SyncReportAccessGroups` replaces prior report grants, then applies the
selected groups — same replace semantics as folder `ReplaceFolderPermissions`).
Delete report files does **not** remove ACL rows (PHP parity). Folder ACL is
written by Go folder CRUD / permissions APIs.

**Folder inheritance:** when a folder or `content_items` row has `acl_inherit=true`
(default), parent folder grants apply to **folders and content resources** if the
item’s own resource grants deny. Set `acl_inherit=false` for a per-item override
(direct grants only). **`jasper_report` view/execute does not inherit folder
ACL** (PHP `jrs_has_permission` parity) — report Access Groups /
`resource_permissions` on the report resource are authoritative. Home’s default
Public folder grant must not make every report runnable by Public users.

### Content folders

| Table | Responsibility |
|-------|----------------|
| `content_folders` | Nested folders (`parent_id` self-FK); root row is **Home** |
| `content_items` | Polymorphic membership (`content_type` + `content_key` → `folder_id`) for reports, dashboards, templates, **content resources**, etc. |
| `content_favorites` | Per-user favorites across any content type |
| `jasper_content_resource` | First-class folder Resources (keys, certs, **images**, files). Identity is **UUID**; bytes live under `{appRoot}/resources/{uuid}{ext}`. Image thumbnails under `{appRoot}/thumbnails/{uuid}.png`. Image metadata: `width`, `height`, `has_alpha`. `content_items` uses `content_type=content_resource`, `content_key=<uuid>`. |

Reports remain filesystem JSON; folder placement is metadata in `content_items`.
Unfiled reports appear under Home in the catalog. Seed/migration:
`database/migrations/004_content_folders.sql`. Resources migration:
`database/migrations/005_content_resources.sql` and image columns in
`database/migrations/006_image_resources.sql`. Recursive delete of non-empty
folders requires `JRS_FOLDER_ALLOW_RECURSIVE_DELETE=1` plus `confirm=1`.

Resources inherit folder ACL (no per-resource ACL row). List/download require
folder **view**; create/update/delete require folder **edit**.

**Precedence (verified):** auth disabled → `is_admin` → admin-named groups →
group grants including implicit Public → **folder inheritance** → default deny.
No per-user ACL rows.

### Datasources

| Table | Responsibility |
|-------|----------------|
| `jasper_datasource` | Named JDBC connection settings (host, port, db, user, password, schema, `db_type`, `auth_type`, optional `public_key_resource_uuid` / `private_key_resource_uuid`, enabled) |
| `jasper_geoserver` | Named GeoServer connections for Designer-published maps (url, optional user/password, enabled). Lookup key is **name** (= Designer Connection Manager name / JRXML `connectionName`, not Designer UUID). |
| `jasper_arcgis` | Named ArcGIS Feature Server connections for Designer-published maps (url, optional user/password, enabled). Lookup key is **name** (= Designer Connection Manager name / JRXML `connectionName`, not Designer UUID). |
| `pg_store` | Legacy fallback if `jasper_datasource` missing (Needs verification in deployed DBs) |

Reads: report param SQL, context generation, UI.  
Writes (PHP + Go datasources admin): upsert/delete on `jasper_datasource`.
Writes (PHP/Go GeoServer admin): upsert/delete on `jasper_geoserver` and regenerate `geoserver-connections.properties` for Java `GeoServerAuth`.
Writes (PHP/Go ArcGIS admin): upsert/delete on `jasper_arcgis` and regenerate `arcgis-connections.properties` for Java `ArcGISAuth`.
Tomcat `context.xml` generation is available on Go under `/go` (PHP prod URL unchanged).

### Access groups (legacy)

| Table | Responsibility |
|-------|----------------|
| `jasper_access_group` | Legacy access group names; PHP prefers `auth_groups` when present |

### Scheduling

| Table | Responsibility |
|-------|----------------|
| `jasper_schedule` | Schedule definition, report, format, email fields, repeat fields |
| `jasper_schedule_execution` | Execution history / status / output path |
| `jasper_recipient` | Email recipient address book |

Reads: scheduling UI, runner.  
Writes (PHP + Go under `/go`): schedule/recipient CRUD, runner status updates, env/unit sync.

### Dataset Intelligence

| Table | Responsibility |
|-------|----------------|
| `di_dataset` | Stable dataset identity (`connection_id` + database + schema + object) and business metadata |
| `di_dataset_usage` | Links datasets to reports/views/dashboards (Phase 2+) |
| `di_dataset_profile` | Stored profile runs (`is_latest`, health status/reasons, row/column counts) |
| `di_profile_check` | Per-check status/result for a profile (timeouts do not fail the whole profile) |
| `di_field_profile` | Per-field null/distinct statistics for a profile |
| `di_spatial_profile` | PostGIS validity/extent/SRID summary for a profile |
| `di_field_metadata` | Optional field display name/description/unit/tags (not stored in JRXML) |

Migration: `database/migrations/009_dataset_intelligence.sql`.  
Go also calls `EnsureSchema` at startup so tables exist without a manual migrate step.  
Profiling uses ephemeral customer-DB connections (same pattern as Data Explorer); credentials are never returned by Dataset Intelligence APIs.

## Go diagnostics expected tables

The `/api/v1/diagnostics` endpoint may report presence (boolean only) for:

- `jasper_datasource`
- `jasper_access_group`
- `jasper_schedule`
- `jasper_recipient`
- `jasper_schedule_execution`
- `dashboard_users`
- `dashboard_sessions`
- `auth_groups`
- `auth_group_members`
- `resources`
- `resource_permissions`
- `content_folders`
- `content_items`
- `content_favorites`

It must never return row contents or credentials.
