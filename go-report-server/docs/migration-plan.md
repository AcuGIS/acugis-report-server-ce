# Migration plan

This document describes the ordered, non-destructive migration from the existing
PHP AcuGIS Report Server to the parallel Go implementation in `go-report-server/`.

**The PHP application remains the production implementation until a separately
approved cutover.** PHP retirement is not part of any foundation or feature-port
task unless explicitly approved.

## Phases

### 1. Foundation and contracts

- Parallel Go module and development server
- Shared `JRS_*` configuration loading
- Read-only PostgreSQL connectivity
- Filesystem path diagnostics
- Jasper HTTP client foundation (no execution)
- Health / ready / diagnostics endpoints
- Contract documentation

Status: **Complete**

### 2. Jasper execute and image proxy

Status: **Development Complete / Production Pending**

- Port `reports/execute_report.php` and `report_image.php` behind
  **development-only** Go routes (`/dev/report`, `/dev/report-image`)
- Reusable Jasper Gateway is the only Go component talking to Tomcat
- Streaming responses, parameter forwarding, timeouts, error translation
- Preserve existing PHP endpoints and Apache routing unchanged
- Production cutover for these routes remains pending

### 3. Authentication and ACL interoperability

Status:

- Authentication interoperability: **Development Complete / Production Pending**
- ACL interoperability: **Development Complete / Production Pending**

- Validate `jrs_session` the same way PHP does (SHA-256 of raw cookie vs
  `dashboard_sessions.token_hash`)
- Service and report ACL parity (`view` / `edit` / `admin`, Public group)
- Development `/dev/report` and `/dev/report-image` enforce real ACL (no
  permissive placeholders)
- `cmd/jrs-auth-check` for live session interop probes
- Allow Go and PHP to coexist on the same sessions and permissions while
  keeping all production routes on PHP
- Login / logout session create+destroy under `/go` (shared cookie/DB with PHP)
### 4. Report catalog read APIs *(next recommended)*

- Read-only equivalents of discovery / reports list / report definition serving
- Load existing `*.report.json` without rewriting files
- Reuse Phase 3 ACL contracts for catalog filtering

### 5. Datasource management

Status: **Development Complete / Production Pending**

- Datasource list / create / update / delete parity with PHP (`/reports/datasources.php`)
- Read/connect for Map Reports — **done** (used by map viewer)
- `context.xml` generation matching `reports/generate_jasper_context.php` (dev under `/go`; PHP prod unchanged)
### 6. Publishing and editing

Status: **Development Complete / Production Pending**

- Publish / edit / delete report flows (PHP-shaped UI under `/go` + REST)
- JRXML / .jasper / thumbnail upload and optional `jasperstarter` compile
- Resource registration / access-group sync parity (`jrs_sync_report_access_groups`)
- Production PHP URLs unchanged until cutover approval
### 7. Scheduling and email

Status: **Development Complete / Production Pending**

- Schedule CRUD UI (`GET /schedules.php`, `/edit_schedule.php`, `POST /action/schedule.php`)
- Executions / history / recipients UI and actions
- systemd timer units, Go runner, SMTP delivery (wired via `internal/schedules`)
- Must not install production timers until approved

### 8. Map Reports

Status: **Development Complete / Production Pending**

- `map_features` / `map_extent` and map definition handling for the interactive viewer
- `map_columns` (edit UI field pickers) under `/go`

### 9. Admin and remaining UI

- Hubs chrome + portal catalog UI: **dev slice complete** (`GET /hubs/{hub}`, `GET /` reusing PHP `shared/ui` + `portal.css`; PHP production URLs unchanged)
- Report viewer `view.php` parity: **dev slice complete** (`GET /reports/{key}`, PHP-shaped config/map/execute aliases; reuses `assets/js/map-style.js`)
- Publish / edit / delete: **dev slice complete** (material shell pages + multipart APIs; reuses `shared/ui` shell assets)
- Users / groups admin: **dev slice complete** (`GET/POST /admin/users.php`, `/admin/groups.php`; bcrypt passwords; platform-admin gate)
- Report URL builder: **dev slice complete** (`GET /reports/report_runner.php`; ACL-filtered report list; reuses `get_report_config.php`)
- Login / logout: **dev slice complete** (`GET/POST /login.php`, `/logout.php`; session create/destroy; shell logout stays on Go mount)
- Workspace chrome: **dev slice complete** (`GET /workspace.php`)
- Resource sync + thumbnails: **dev slice complete**
- Remaining discovery aliases / misc leftovers

### 10. Cutover and PHP retirement

- Route selected traffic to Go only after contract and integration tests pass
- PHP retirement requires a separate, explicit approval
- systemd timers may switch from `scripts/run_schedule.php` to `jrs-run-schedule` only after approval
## Non-negotiable rules

- Do not modify, rename, move, replace, or delete existing PHP/Java/Tomcat/Apache
  assets while building Go features
- Do not send production traffic to Go until a feature is contract-tested
- Do not invent schema changes; reuse `database/schema.sql` and existing data
