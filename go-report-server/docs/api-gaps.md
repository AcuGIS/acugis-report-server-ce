# AcuGIS Report Server (Go) — API Gaps

Gaps relative to a complete, CLI/SDK-friendly Go API surface. Based on the inventory in [`api-inventory.md`](./api-inventory.md) and handler behavior in `internal/server`. Go implementation only.

---

## Summary

This Open Source server exposes `/api/v1/*` for reports, folders, resources, and datasources. Map Reports and Data Explorer are not in this tree. Gaps concentrate in **administration**, **scheduling**, **map connection management**, **auth for non-browser clients**, and **consistency / discoverability**.

---

## UI actions with no corresponding API endpoint

| UI / capability | Current surface | Gap |
|-----------------|-----------------|-----|
| User create / update / delete / list | `GET/POST /admin/users.php` only | No `/api/v1/users` |
| Group create / update / delete / list / members | `GET/POST /admin/groups.php` only | No `/api/v1/groups` |
| Schedule list / get / create / update / delete | HTML pages + `POST /action/schedule.php` form | No `/api/v1/schedules` CRUD |
| Execution list / history / get | `/executions.php`, `/history.php` HTML | No `/api/v1/executions` |
| Recipient list / get / CRUD | HTML + `POST /action/recipient.php` form | No `/api/v1/recipients` |
| GeoServer connections CRUD | `/reports/geoserver_connections.php` HTML only | No REST |
| ArcGIS connections CRUD | `/reports/arcgis_connections.php` HTML only | No REST |
| Datasource update by id | Create POST upserts by name; DELETE by id | No dedicated `PUT /api/v1/datasources/{id}` |
| Datasource get (full record) | List returns **names only** | No get-by-id / detail JSON (credentials excluded) |
| Report ACL get/set (standalone) | Set only via publish/edit multipart `access_groups` | No `/api/v1/reports/{key}/permissions` |
| Service-level ACL management | Implicit via groups/admin | No service permissions API |
| Dashboard layout CRUD | Only explorer view “widget” flag | No layout, placement, or multi-widget dashboard API |
| Licensing | — | No endpoints (if product needs license check/activate) |
| Report definition download as JRXML file | Definition available as JSON via GET report | No dedicated `/definition` or JRXML download route in Go routes |
| Jasper execute under stable `/api/v1/jasper/*` | `/reports/execute`, `/dev/report` | Dev/legacy paths only; no versioned execute/image API |

---

## Endpoints that only accept browser sessions (cookie)

**Resolved for API clients:** personal access tokens (`Authorization: Bearer jrs_…`)
authenticate the same user object as the session cookie. See
[`authentication.md`](./authentication.md).

Remaining cookie-oriented surfaces (HTML only; no JSON twin):

| Impact | Detail |
|--------|--------|
| HTML pages | Catalog, viewer, publish/edit forms, hubs, workspace, explorer page, admin users/groups, schedule/recipient HTML, GeoServer/ArcGIS, resource sync page |
| Auth disabled | `JRS_AUTH_DISABLED` injects synthetic admin — fine for local dev, not a production machine auth story |

Token management UI: `/profile.php` (uses cookie for the browser; tokens themselves are for SDKs).

---

## Inconsistent response formats

| Pattern A | Pattern B | Where |
|-----------|-----------|-------|
| `{"ok":true}` | `{"success":true,"message":"…"}` | Auth/resources vs schedule/recipient actions |
| `{"error":"…"}` | `{"error","errors":[…]}` | Most APIs vs datasource validation |
| JSON authz errors | Plain-text `http.Error` | APIs vs HTML pages |
| Jasper execute/image | `text/plain` gateway errors | Not JSON like other APIs |
| Explorer delete | `{status:"deleted"}` | Most deletes use `{ok:…}` |
| Resource sync | HTTP 200 with `ok:false` when sync has errors | Unusual vs typical 4xx/5xx |
| Schedule `run_now` | HTTP 200 + `success:false` on failed run | Same soft-failure pattern |
| Report create | Multipart form | Explorer APIs use JSON bodies |
| Datasource create API | `application/x-www-form-urlencoded` | Not JSON despite `/api/v1` path |
| Generate context | JSON **or** `text/plain` depending on Accept/path | Dual content types |
| Embedded map debug | JSON **or** HTML wrapper | Dual content types |

---

## Missing pagination

| Endpoint | Behavior | Gap |
|----------|----------|-----|
| `GET /api/v1/reports` | Returns all visible reports | No `limit`/`offset`/`cursor` |
| `GET /api/v1/folders/search` | Hard cap 50 | No paging beyond limit |
| `GET /api/v1/resources` | Filtered list, no page params | No pagination |
| `GET /api/v1/explorer/views` | All owner views | No pagination |
| `GET /api/v1/explorer/dashboard-widgets` | All widgets | No pagination |
| History UI | Last 200 executions | No API; no cursor for older rows |
| Explorer query | Supports `limit`/`offset`/`page`/`page_size` | Good — exception |

---

## Missing filtering

| Endpoint | Available | Missing |
|----------|-----------|---------|
| `GET /api/v1/reports` | ACL visibility only | Filter by type, datasource, folder, q |
| `GET /api/v1/datasources` | — | Filter by `db_type`, enabled; returns names only |
| `GET /api/v1/resources` | `folder_id`, `resource_type`, `q` | Sort, date range |
| Folder browse | Single folder | Filter item types in query (client-side today) |
| Schedules / executions | N/A (no list API) | Filter by report, status, date |
| Map features | Report param query string | Documented filter schema varies per report |

---

## Missing or weak stable identifiers

| Resource | Identifier today | Issue |
|----------|------------------|-------|
| Report | String `key` (filename-derived) | Stable if key immutable; rename/republish semantics are key-centric, not UUID |
| Folder | Numeric `id` | Stable DB id — OK |
| Content resource | UUID | Stable — OK |
| Datasource | Numeric `id` for delete; **name** for create/list | List API does not return `id`; clients cannot delete from list response alone without another round-trip through HTML/admin knowledge |
| Saved explorer view | String `key` | OK if documented as stable |
| Schedule | Numeric `id` | Only via form/HTML — no list API to discover ids |
| User / group | Numeric `id` | HTML-only; no API discovery |
| Jasper image | `image=` query + report name | No signed/stable image token (by design; ACL each request) |
| Favorite items | `content_type` + `content_key` | Composite — OK if both always present |

---

## Endpoints that return HTML instead of JSON

Unsuitable as primary SDK surfaces (HTML or redirect):

- `/login.php`, `/logout.php` (redirect)
- `/`, `/reports`, `/reports/index.php` (catalog)
- `/reports/{key}`, `/reports/view.php` (viewer / redirect)
- `/reports/publish_report.php`, `/reports/edit_report.php` (unless `Accept: application/json` on POST)
- `/reports/delete_report.php` (redirect unless Accept/DELETE)
- `/reports/datasources.php`, geoserver/arcgis pages
- `/reports/resource_sync.php`, `/reports/report_runner.php`
- `/explorer`, `/workspace.php`, `/hubs/{hub}`
- `/admin/users.php`, `/admin/groups.php`
- `/schedules.php`, `/edit_schedule.php`, `/executions.php`, `/history.php`
- `/recipients.php`, `/edit_recipient.php`
- Embedded map debug without `format=json` / Accept JSON

Prefer the `/api/v1/*` twins where they exist.

---

## Endpoints unsuitable for CLI or SDK use

| Category | Examples | Why |
|----------|----------|-----|
| HTML shells | Catalog, hubs, workspace, explorer page | Browser UI |
| Form-only admin | Users, groups, GeoServer, ArcGIS | No JSON contract; scrape-hostile |
| Soft JSON form actions | `/action/schedule.php`, `/action/recipient.php` | Form fields, inconsistent success semantics |
| Dual-mode pages | Publish/edit/delete PHP paths | Need Accept hacks; prefer REST |
| Dev-named routes | `/dev/report`, `/dev/report-image` | Naming implies non-production; aliases exist under `/reports/*` |
| Public diagnostics | `/api/v1/diagnostics` | Useful for ops; may leak environment detail — gate for production if exposed |
| Cookie-only auth | All protected routes | No token auth for automation |

**Usable today for CLI/SDK (with cookie jar):** auth login/logout, reports CRUD/config/lookup, execute/image (binary), folders, resources, explorer APIs, map APIs, datasource list/create/delete/context, resource-sync POST, health/ready.

---

## Operations that require multiple undocumented calls

| Goal | Typical call chain | Gap |
|------|--------------------|-----|
| Publish then run report | `POST /api/v1/reports` → `GET/POST /reports/execute` | Execute not under `/api/v1`; format/param contract is Jasper-shaped |
| Create datasource and enable Jasper JNDI | `POST /api/v1/datasources` → `POST /api/v1/datasources/context` | Context not automatic; second call easy to miss |
| Folder + upload image + attach to report | folders create → images POST → edit report multipart | No single “attach resource” report API |
| Explorer → publish report | `POST …/handoff` → open publish UI with handoff | Handoff returns `publish_url` / note; does not create JRXML |
| Schedule a report | Discover report key → HTML edit schedule or form POST action | No schedule list API; systemd sync side effects undocumented in OpenAPI |
| Delete datasource by name | List names → somehow obtain id → DELETE | List omits `id` |
| Admin: create user in group | HTML users POST with `groups` | No API; group name vs id coupling only in HTML form |
| Favorites UX | browse/search → POST favorites | Documented, but favorites GET may return sparse fields |

---

## Missing OpenAPI definitions

| Item | Status |
|------|--------|
| OpenAPI / Swagger / JSON Schema for `/api/v1` | **Done:** modular [`openapi/`](../openapi/); served at `/api/openapi.yaml` + `/api/openapi.json`; Swagger UI at `/swagger` — see [`openapi.md`](./openapi.md) |
| Generated client SDK | Partial: Python SDK/CLI exists; other languages generate from OpenAPI |
| Per-endpoint request/response schemas checked into docs | OpenAPI schemas + [`api-inventory.md`](./api-inventory.md) |
| Error code catalog | Informal (401/403/404/422/503); no stable `error_code` field on all endpoints |

---

## Auth / security gaps for API clients

| Gap | Detail |
|-----|--------|
| ~~No token auth~~ | **Added:** Bearer PATs (`jrs_…`) + `/api/v1/auth/tokens` + `/api/v1/auth/me` |
| HTML pages do not redirect to login | `writeAuthzHTML` returns plain 401/403 text — browsers see error page, not login form |
| Login API returns no user profile | `{"ok":true}` only — use `GET /api/v1/auth/me` after login or with Bearer |
| Diagnostics is public | Unauthenticated ops metadata |

---

## Area-by-area gap checklist

### Authentication
- [x] Bearer / API token
- [x] `GET /api/v1/auth/me`
- [ ] Login response includes user + expiry (optional; `/me` covers identity)
- [x] OpenAPI for auth endpoints (full API in [`openapi/`](../openapi/); legacy stub [`openapi-auth.yaml`](./openapi-auth.yaml))

### Reports
- [ ] List filters + pagination
- [ ] Dedicated permissions API
- [ ] Stable UUID optional alongside key (if required by product)
- [ ] Versioned `/api/v1/jasper/execute` and `/image`

### Report execution
- [ ] JSON error envelope consistent with other APIs
- [ ] Documented parameter + format contract as OpenAPI

### Folders / permissions
- Folder REST is relatively complete
- [ ] Report/service ACL REST parity with folder permissions

### Datasources
- [ ] JSON create/update body
- [ ] Detail GET including `id` (no secrets)
- [ ] REST for GeoServer / ArcGIS connections

### Users / groups
- [ ] Full REST CRUD

### Schedules / jobs / recipients
- [ ] Full REST CRUD + list/filter/pagination
- [ ] Replace form `action/*.php` with `/api/v1/schedules` and `/api/v1/recipients`

### Data Explorer / dashboards
- Explorer query APIs are strong
- [ ] Real dashboard resource API beyond widget flags
- [ ] View list pagination

### Embedded maps / Map Reports
- Map feature/extent/columns APIs are usable
- [ ] Non-debug embedded-map status API if needed for production tooling

### Licensing
- [ ] Endpoints if product requires license management

### Health
- Health/ready present
- [ ] Consider auth on diagnostics in production

### Documentation
- [x] OpenAPI 3 definition covering all `/api/v1/*` endpoints ([`openapi/`](../openapi/), [`openapi.md`](./openapi.md))
- [ ] Official CLI/SDK package wrapping cookie or future token auth — Python present; other SDKs via OpenAPI generators

---

## Related docs

- [`api-inventory.md`](./api-inventory.md) — full endpoint inventory
- [`route-map.md`](./route-map.md) — historical PHP → Go route planning
- [`current-contracts.md`](./current-contracts.md) — session, ACL, and Jasper contracts
