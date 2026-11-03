# Route map (PHP → future Go)

Rows for Data Explorer, Map Reports, and other Professional routes describe the full product. Those implementations are not in this Open Source tree.

Mount prefix in production PHP: `/acugis-report-server`.

Statuses: **Not started**, **Foundation only**, or **Development Complete / Production Pending**.
Production PHP routes remain authoritative; Go `/dev/*` proxies are development-only.

| Current PHP file | Current URL | Purpose | Auth required | ACL requirement | Reads | Writes | Calls Tomcat | Proposed Go route | Proposed Go package | Migration phase | Status |
|------------------|-------------|---------|---------------|-----------------|-------|--------|--------------|-------------------|---------------------|-----------------|--------|
| `index.php` | `/` or mount root | Redirect / entry | varies | — | — | — | no | TBD | server | 9 | Not started |
| `login.php` | `/login.php` | Login form / session create | no | — | `dashboard_users`, sessions | sessions, cookie | no | `GET/POST /login.php` + `POST /api/v1/auth/login` | auth | 6 | Development Complete / Production Pending |
| `logout.php` | `/logout.php` | Logout | optional | — | sessions | sessions, cookie | no | `GET/POST /logout.php` + `POST /api/v1/auth/logout` | auth | 6 | Development Complete / Production Pending |
| `workspace.php` | `/workspace.php` | Federation workspace shell | yes | service view | — | — | no | `GET /workspace.php` (dev; PHP prod stays) | server | 9 | Development Complete / Production Pending |
| `hubs/reports.php` | `/hubs/reports.php` | Reports hub shell | yes | service view | — | — | no | `GET /hubs/reports` (dev; PHP prod stays) | server | 9 | Development Complete / Production Pending |
| `hubs/data.php` | `/hubs/data.php` | Data hub shell | yes | service view | — | — | no | `GET /hubs/data` (dev; PHP prod stays) | server | 9 | Development Complete / Production Pending |
| — (Go-first) | `/hubs/data-explorer` | Data Explorer hub | yes | service view | — | — | no | `GET /hubs/data-explorer` | explorer | 10 | Development Complete / Production Pending |
| — (Go-first) | `/explorer` | Data Explorer UI | yes | service view | DS, views JSON | views JSON | no | `GET /explorer` + `/api/v1/explorer/*` | explorer | 10 | Development Complete / Production Pending |
| `hubs/scheduling.php` | `/hubs/scheduling.php` | Scheduling hub | yes | platform admin | — | — | no | `GET /hubs/scheduling` (dev; PHP prod stays) | schedules | 7 | Development Complete / Production Pending |
| `hubs/administration.php` | `/hubs/administration.php` | Admin hub | yes | platform admin | — | — | no | `GET /hubs/administration` (dev; PHP prod stays) | acl | 9 | Development Complete / Production Pending |
| `reports/index.php` | `/reports/index.php` | Report catalog UI | yes | service view | JSON, resources | — | no | `GET /` + `GET /api/v1/reports` (dev; PHP prod stays) | reports | 3 | Development Complete / Production Pending |
| — (Go-first) | `/api/v1/folders/*` | Content folders CRUD, browse, tree, search, move, permissions | yes | folder ACL (+ inheritance) | `content_folders`, `content_items` | folders, favorites | no | `GET/POST/PUT/DELETE /api/v1/folders…` + `POST /api/v1/content/move` + `/api/v1/favorites` | folders | 11 | Development Complete / Production Pending |
| — (Go-first) | `/api/v1/resources/*` | Folder Resources CRUD + download (UUID storage) | yes | folder ACL (inherit) | `jasper_content_resource`, `content_items` | `resources/{uuid}{ext}` | no | `GET/POST /api/v1/resources` + `GET/PUT/DELETE /api/v1/resources/{uuid}` + `/download` + `/content` + `/thumbnail` | contentresource | 11 | Development Complete / Production Pending |
| — (Go-first) | `/api/v1/resources/images/*` | Image Resource upload/list/metadata/replace/download/delete | yes | folder ACL (inherit) | `jasper_content_resource` | `resources/{uuid}{ext}`, `thumbnails/{uuid}.png` | no | `GET/POST /api/v1/resources/images` + `GET/PUT/DELETE /api/v1/resources/images/{uuid}` + `/download` | contentresource | 11 | Development Complete / Production Pending |
| `reports/_index.php` | `/reports/_index.php` | Alternate catalog | yes | service view | JSON | — | no | `GET /api/v1/reports` | reports | 3 | Not started |
| `reports/view.php` | `/reports/view.php` / rewrite `/reports/{key}` | View report | runtime | report view | JSON | — | indirect | `GET /reports/{key}` (dev; PHP prod stays) | server | 9 | Development Complete / Production Pending |
| `reports/execute_report.php` | `/reports/execute_report.php` | Jasper execute proxy | runtime | report view | JSON, ACL | — | yes `/report` | `GET/POST /dev/report` + `/reports/execute` + `/reports/execute_report.php` (prod later: `/api/v1/jasper/execute`) | jasper | 2–3 | Development Complete / Production Pending |
| `report_image.php` | `/report_image.php` | Jasper image proxy | runtime | report view | ACL | — | yes `/acugis_report_image` | `GET /dev/report-image` (prod later: `/api/v1/jasper/image`) | jasper | 2–3 | Development Complete / Production Pending |
| `reports/get_report_config.php` | `/reports/get_report_config.php` | Report config JSON | yes/runtime | report view | JSON, DS | — | no | `GET /reports/get_report_config.php` + `GET /api/v1/reports/{key}/config` | reports | 3 | Development Complete / Production Pending |
| `reports/serve_report_definition.php` | `/reports/serve_report_definition.php` | Serve definition | yes | report view | JSON | — | no | `GET /api/v1/reports/{key}/definition` | reports | 3 | Not started |
| `reports/load_reports.php` | `/reports/load_reports.php` | Load report list | yes | service view | JSON | — | no | `GET /api/v1/reports` | reports | 3 | Not started |
| `reports/publish_report.php` | `/reports/publish_report.php` | Publish report | yes | service edit | DS, groups | JSON, JRXML, resources | no | `GET/POST /reports/publish_report.php` + `POST /api/v1/reports` (dev; PHP prod stays) | reports | 5–6 | Development Complete / Production Pending |
| `reports/edit_report.php` | `/reports/edit_report.php` | Edit report | yes | report edit | JSON | JSON, JRXML, ACL | no | `GET/POST /reports/edit_report.php` + `PUT /api/v1/reports/{key}` (dev; PHP prod stays) | reports | 5–6 | Development Complete / Production Pending |
| `reports/delete_report.php` | `/reports/delete_report.php` | Delete report | yes | report admin | — | JSON, files, resources | no | `GET /reports/delete_report.php` + `DELETE /api/v1/reports/{key}` (dev; PHP prod stays) | reports | 5–6 | Development Complete / Production Pending |
| `reports/datasources.php` | `/reports/datasources.php` | Datasource UI/CRUD | yes | service admin | `jasper_datasource` | `jasper_datasource` | no | `GET/POST /reports/datasources.php` + `/api/v1/datasources` (dev; PHP prod stays) | datasources | 4–5 | Development Complete / Production Pending |
| `reports/geoserver_connections.php` | `/reports/geoserver_connections.php` | GeoServer connection UI/CRUD + properties export | yes | service admin | `jasper_geoserver` | `geoserver-connections.properties` | no | `GET/POST /reports/geoserver_connections.php` (dev under `/go`) | geoserver | 4 | Development Complete / Production Pending |
| `reports/arcgis_connections.php` | `/reports/arcgis_connections.php` | ArcGIS connection UI/CRUD + properties export | yes | service admin | `jasper_arcgis` | `arcgis-connections.properties` | no | `GET/POST /reports/arcgis_connections.php` (dev under `/go`) | arcgis | 4 | Development Complete / Production Pending |
| `reports/generate_jasper_context.php` | `/reports/generate_jasper_context.php` | Write Tomcat context.xml | yes | service edit | DS | `context.xml` | no | `GET/POST /reports/generate_jasper_context.php` + `POST /api/v1/datasources/context` (dev; PHP prod stays) | datasources | 4–5 | Development Complete / Production Pending |
| `reports/report_thumbnail.php` | `/reports/report_thumbnail.php` | Serve thumbnail | varies | report view | PNG | — | no | `GET /reports/report_thumbnail.php` + `GET /api/v1/reports/{key}/thumbnail` | reports | 3 | Development Complete / Production Pending |
| `reports/report_runner.php` | `/reports/report_runner.php` | URL builder UI | yes | report view | JSON | — | no | `GET /reports/report_runner.php` | server | 9 | Development Complete / Production Pending |
| `reports/frontend.php` | `/reports/frontend.php` | Frontend helper | varies | — | — | — | no | TBD | reports | 9 | Not started |
| `reports/render_reports.php` | `/reports/render_reports.php` | Render helper | varies | — | — | — | no | TBD | reports | 9 | Not started |
| `reports/resource_sync.php` | `/reports/resource_sync.php` | Manual resource sync UI | yes | platform admin | JSON, resources | resources | no | `GET/POST /reports/resource_sync.php` + `POST /api/v1/resource-sync` | reports | 5 | Development Complete / Production Pending |
| `reports/map_columns.php` | `/reports/map_columns.php` | Map columns | runtime | report edit | DS | — | no | `GET /api/v1/map/{key}/columns` + `/reports/map_columns.php` | mapreport | 8 | Development Complete / Production Pending |
| `reports/map_extent.php` | `/reports/map_extent.php` | Map extent | runtime | report view | DS | — | no | `GET /api/v1/map/{key}/extent` + `/reports/map_extent.php` | mapreport | 8 | Development Complete / Production Pending |
| `reports/map_features.php` | `/reports/map_features.php` | Map features | runtime | report view | DS | — | no | `GET /api/v1/map/{key}/features` + `/reports/map_features.php` | mapreport | 8 | Development Complete / Production Pending |
| `api/reports/index.php` | `/api/reports/` | Reports API | yes | service/report ACL | JSON | — | no | `GET /api/v1/reports` | reports | 3 | Not started |
| `api/datasources/index.php` | `/api/datasources/` | Datasources API | yes | ACL | DS | — | no | `GET /api/v1/datasources` (dev; PHP prod stays) | datasources | 4–5 | Development Complete / Production Pending |
| `api/discovery/index.php` | `/api/discovery/` | Discovery API | yes | report view filter | JSON, resources | — | no | `GET /api/v1/discovery` | reports | 3 | Not started |
| `api/resource-sync/index.php` | `/api/resource-sync/` | Resource sync API | yes | platform admin | JSON | resources | no | `GET/POST /api/v1/resource-sync` (+ PHP-shaped aliases) | reports | 5 | Development Complete / Production Pending |
| `schedules.php` | `/schedules.php` | Schedules list UI | yes | platform admin | `jasper_schedule` | — | no | `GET /schedules.php` | schedules | 7 | Development Complete / Production Pending |
| `edit_schedule.php` | `/edit_schedule.php` | Edit schedule UI | yes | platform admin | schedule | schedule, env, systemd | no | `GET /edit_schedule.php` | schedules | 7 | Development Complete / Production Pending |
| `action/schedule.php` | `/action/schedule.php` | Schedule JSON actions | yes | platform admin | schedule | schedule, systemd | maybe via runner | `POST /action/schedule.php` | schedules | 7 | Development Complete / Production Pending |
| `executions.php` | `/executions.php` | Execution history UI | yes | platform admin | executions | — | no | `GET /executions.php` | schedules | 7 | Development Complete / Production Pending |
| `history.php` | `/history.php` | History UI | yes | platform admin | executions | — | no | `GET /history.php` | schedules | 7 | Development Complete / Production Pending |
| `recipients.php` | `/recipients.php` | Recipients UI | yes | platform admin | recipients | — | no | `GET /recipients.php` | schedules | 7 | Development Complete / Production Pending |
| `edit_recipient.php` | `/edit_recipient.php` | Edit recipient | yes | platform admin | recipients | recipients | no | `GET /edit_recipient.php` | schedules | 7 | Development Complete / Production Pending |
| `action/recipient.php` | `/action/recipient.php` | Recipient actions | yes | platform admin | recipients | recipients | no | `POST /action/recipient.php` | schedules | 7 | Development Complete / Production Pending |
| `admin/users.php` | `/admin/users.php` | Users admin | yes | platform admin | users, groups | users, groups | no | `GET/POST /admin/users.php` | admin | 9 | Development Complete / Production Pending |
| `admin/groups.php` | `/admin/groups.php` | Groups admin | yes | platform admin | groups | groups, ACL | no | `GET/POST /admin/groups.php` | admin | 9 | Development Complete / Production Pending |
| `scripts/run_schedule.php` | CLI / systemd | Scheduled run | N/A (CLI) | — | schedule, Jasper | output, email, executions | yes | Go runner (`ScheduleRun`) + `cmd/jrs-run-schedule` (timers still call PHP until cutover) | schedules | 7 | Development Complete / Production Pending |
| `debug/session.php` | `/debug/session.php` | Session probe | debug flag | — | sessions | — | no | TBD | auth | 6 | Not started |
| *(Go only)* | — | Process health | no | — | — | — | no | `GET /health` | server | 1 | Foundation only |
| *(Go only)* | — | Readiness | no | — | DB, FS | — | no | `GET /ready` | server | 1 | Foundation only |
| *(Go only)* | — | Diagnostics | no | — | DB meta, FS | — | no | `GET /api/v1/diagnostics` | server | 1 | Foundation only |
| *(Go only)* | — | Dev Jasper execute | optional session | report view ACL | JSON exists | — | yes `/report` | `GET/POST /dev/report` | jasper | 2–3 | Development Complete / Production Pending |
| *(Go only)* | — | Dev Jasper image | optional session | report view ACL | — | — | yes `/acugis_report_image` | `GET /dev/report-image` | jasper | 2–3 | Development Complete / Production Pending |
| *(Go only)* | — | Dev Jasper status | no | — | gateway meta | — | TCP probe | `GET /dev/jasper-status` | jasper | 2–3 | Development Complete |

Notes:

- “runtime” auth means report proxy may allow Public ACL without a full login redirect (`jrs_require_report_runtime_permission`).
- Proposed Go routes are planning targets; they must not replace PHP URLs in Apache during early phases.
- Phase 3 Go routes enforce PHP-compatible session validation and ACL on `/dev/*`. Production traffic stays on PHP until cutover approval.
- Image access uses `_repName`/`report` + report `view` ACL; there is no signed image key in PHP.