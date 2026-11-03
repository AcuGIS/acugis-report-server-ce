# AcuGIS Report Server — Go (parallel migration)

> **This directory is a parallel migration implementation.**
>
> The existing PHP application remains the production implementation.
> Do not route production traffic to this Go service until individual
> features have passed contract and integration testing.

## Purpose

`go-report-server/` is a **non-destructive**, side-by-side Go foundation for
migrating AcuGIS Report Server off PHP. The PHP tree, Tomcat/Jasper WAR,
Apache configuration, Docker setup, schemas, and report assets are left
untouched.

## Current milestone

**Replace PHP with Go, slice by slice.** Live in Go:

- Portal catalog UI (`GET /`) — same Materialize/`portal.css` look as `reports/index.php`
- Hubs chrome (`GET /hubs/{reports|data|scheduling|administration}`) — reuses `shared/ui` shell
- Shared PHP assets (`/shared/ui/*`, `/reports/portal.css`, `/assets/reports/*`, `/assets/js/*`)
- Report viewer (`GET /reports/{key}`) — `reports/view.php` parity (same CSS/JS; `map-style.js`)
- Catalog API (`GET /api/v1/reports`, `GET /api/v1/reports/{key}`, `…/config`)
- Data Explorer is not in this Open Source tree
- Map Report routes are not in this Open Source tree
- Execute / image (`GET/POST /reports/execute`, `/reports/execute_report.php`, `GET /report-image`)
- Admin users / groups (`GET/POST /admin/users.php`, `/admin/groups.php`) — platform-admin gate; bcrypt passwords
- OpenID Connect and SAML are not in this Open Source tree. `ACUGIS_EDITION` does not enable them.
- Report URL builder (`GET /reports/report_runner.php`) — ACL-filtered report list
- Schedules UI (`GET /schedules.php`, `/edit_schedule.php`, `/executions.php`, `/history.php`, `/recipients.php`) — platform-admin gate; JSON actions via `/action/schedule.php` and `/action/recipient.php`
- PHP-compatible session + ACL (login / logout create and destroy `jrs_session`)

Logout and login stay on the Go mount when `JRS_BASE_PATH=/go`.

## Run locally

```bash
cd go-report-server
cp .env.example .env   # edit credentials
# For a quick smoke test without login:
# JRS_AUTH_DISABLED=1

make run
# or
go run ./cmd/jrs-server
```

Default listen address: `127.0.0.1:9088` (localhost only).

If the public URL is under a prefix (e.g. `https://host/go/`), set:

```bash
JRS_BASE_PATH=/go
```

so catalog/hub links stay on `/go/...` instead of the PHP `/acugis-report-server` mount.

Open:

- http://127.0.0.1:9088/login.php — sign in (creates `jrs_session`)
- http://127.0.0.1:9088/ — portal catalog (PHP look/feel)
- http://127.0.0.1:9088/swagger — OpenAPI Swagger UI
- http://127.0.0.1:9088/api/openapi.yaml — OpenAPI 3.0 contract
- http://127.0.0.1:9088/workspace.php — federation workspace chrome
- http://127.0.0.1:9088/hubs/reports — federation hubs chrome
- http://127.0.0.1:9088/reports/nycdata — viewer (example key)
- http://127.0.0.1:9088/reports/report_runner.php — report URL builder
- http://127.0.0.1:9088/explorer — Data Explorer
- http://127.0.0.1:9088/hubs/data-explorer — Data Explorer hub
- http://127.0.0.1:9088/admin/users.php — user admin (platform admin)

With real auth, use Go `/login.php` (or PHP) so the `jrs_session` cookie exists, or set `JRS_AUTH_DISABLED=1` for local smoke tests only.## Session check utility

Do **not** pass tokens as CLI arguments:

```bash
# stdin
printf '%s' "$TOKEN" | go run ./cmd/jrs-auth-check

# or protected env (local testing only)
JRS_AUTH_CHECK_TOKEN="$TOKEN" go run ./cmd/jrs-auth-check
```

Reports only: `valid`, `invalid`, `expired`, `disabled_user`, `database_error`,
or `auth_disabled` — never the raw token or hash.

## Test commands

```bash
cd go-report-server
make test
make vet
make fmt
go test -race ./...
go build ./...
```

## Current implementation status

| Area | Status |
|------|--------|
| Config (`JRS_*`) | Implemented |
| PostgreSQL pool (read-only) | Implemented |
| Filesystem diagnostics | Implemented |
| Jasper Gateway (execute / image) | Implemented (dev `/dev/*` only) |
| `/health`, `/ready`, `/diagnostics` | Implemented |
| Auth middleware | PHP session interoperability |
| ACL service | Database-backed |
| Report catalog UI | Implemented (portal look; PHP assets) |
| Hubs chrome | Implemented (`/hubs/{hub}`; PHP `shared/ui`) |
| Report viewer UI | Implemented (`view.php` parity; PHP assets + OpenLayers) |
| Jasper execute / image | Implemented (`/reports/execute`, `/report-image`) |
| Map Report APIs | Not in this Open Source tree (`/api/v1/map/{key}/*` requires Professional) |
| Datasources CRUD / context.xml | Implemented (dev under `/go`) |
| Admin users / groups UI | Implemented (`/admin/users.php`, `/admin/groups.php`) |
| Report URL builder | Implemented (`/reports/report_runner.php`) |
| Scheduling / email | Implemented (dev under `/go`; systemd timers soft-skip on non-Linux) |
| Publish / edit UI | Implemented (dev under `/go`) |
| Login / logout | Implemented (`/login.php`, `/logout.php`; shared `jrs_session`) |
| Workspace chrome | Implemented (`/workspace.php`) |
| Map columns | Not in this Open Source tree |
| Resource sync | Implemented (`/reports/resource_sync.php`, `/api/v1/resource-sync`) |
| Report thumbnails | Implemented (`/reports/report_thumbnail.php`) |
| Data Explorer | Not in this Open Source tree |
| OpenAPI / Swagger UI | Implemented (`openapi/`, `/api/openapi.yaml`, `/api/openapi.json`, `/swagger`) |

## Explicitly unimplemented

- Production Apache cutover (PHP production routes unchanged)
- Switching installed systemd timers from `scripts/run_schedule.php` to `jrs-run-schedule` (CLI binary is ready; cutover needs approval)
- Apache / Docker package changes
- Discovery API aliases and a few misc leftovers in `docs/route-map.md`

## Schedule CLI

```bash
make build-cli
# or
go run ./cmd/jrs-run-schedule <schedule_id>
```

Same contract as `php scripts/run_schedule.php <schedule_id>`. Installed timers keep calling PHP until an explicit cutover.
## Documentation

- `docs/openapi.md` — OpenAPI contract, Swagger UI, SDK/Postman generation
- `docs/migration-plan.md`
- `docs/current-contracts.md`
- `docs/route-map.md`
- `docs/database-contract.md`
- `docs/filesystem-contract.md`
- `docs/configuration.md`
- `docs/api-inventory.md`
- `docs/authentication.md`
- `internal/auth/README.md`
- `internal/acl/README.md`
- `internal/jasper/README.md`
