# AcuGIS Report Server (Go) — API Inventory

This inventory includes Professional routes that are not registered in the Open Source server. Data Explorer, Map Reports, dashboards, OpenID Connect, and SAML are absent from this tree. The served contract is the Open Source OpenAPI document.

Inventory of every HTTP endpoint registered by the Go Report Server (`internal/server/routes.go`). Paths are relative to `JRS_BASE_PATH` (default `/`). This document describes the Go implementation only.

**Source of truth:** `internal/server/routes.go` and the named handlers below.  
**Generated from:** Go codebase inspection (handlers, auth helpers, ACL gates).  
**OpenAPI:** modular spec in [`../openapi/`](../openapi/); served at `GET /api/openapi.yaml` and `GET /api/openapi.json`; Swagger UI at `/swagger`. See [`openapi.md`](./openapi.md).  
**Client SDK:** [`sdk/python`](../../sdk/python) (`acugis-report` CLI + `acugis_report` SDK). Server CLIs: `jrs-server`, `jrs-auth-check`, `jrs-run-schedule`.

---

## Conventions

### Auth column values

| Value | Meaning |
|-------|---------|
| Public | No login required |
| Catalog | Session + service view on `jasper_reports` (`requireCatalogAccess`) |
| ServiceEdit | Catalog + service edit (or platform admin) |
| ServiceAdmin | Catalog + service admin (or platform admin) |
| ReportView | Per-report `view` ACL; anonymous allowed if Public group grants view |
| ReportEdit | ServiceEdit + per-report `edit` |
| ReportAdmin | ServiceEdit + per-report `admin` (delete) |
| PlatformAdmin | Catalog + platform admin (`is_admin` or admin-named group) |
| FolderView / FolderEdit / FolderAdmin | Catalog + folder ACL on the target folder |

Session is cookie (`jrs_session`) **or** Bearer PAT (`Authorization: Bearer jrs_…`).
Both produce the same `auth.User`. When `JRS_AUTH_DISABLED` is truthy, middleware
injects synthetic admin `local-dev` and ACL helpers allow.

Global middleware attaches the session optionally; handlers enforce gates. HTML auth failures return plain-text status via `http.Error` (no login redirect). JSON APIs return `{"error":"..."}` with 401/403/503.

### SDK column values

| Value | Meaning |
|-------|---------|
| Yes | JSON (or clean binary) suitable for CLI/SDK |
| Partial | Works programmatically but form-encoded / awkward |
| No | HTML UI or browser-oriented |
| Binary | File/image/report bytes; usable from CLI |
| CLI | Covered by an existing server CLI (not an HTTP SDK) |

### Response shapes (common)

- Authz error JSON: `{"error":"<message>"}`
- Success flags often: `{"ok":true}` or `{"success":true,"message":"..."}`
- Jasper execute/image errors: `text/plain` (not JSON)

---

## Authentication

| Method | Endpoint | Purpose | Auth | Request | Response | Handler | SDK |
|--------|----------|---------|------|---------|----------|---------|-----|
| GET | `/login.php` | Login HTML form | Public | Query `next` | HTML; redirect if already authed / auth disabled | `handleLoginPage` | No |
| POST | `/login.php` | Create session (form) | Public | Form `username`, `password`, `next` | 303 → `next` or HTML with error | `handleLoginPage` | No |
| GET/POST | `/logout.php` | Clear session | Public | — | 303 → `/login.php` | `handleLogout` | No |
| POST | `/api/v1/auth/login` | Create session (API) | Public | JSON `{username,password}` or form | `{"ok":true}`; Set-Cookie | `handleAPIAuthLogin` | Yes |
| POST | `/api/v1/auth/logout` | Clear session (API) | Public | — | `{"ok":true}` | `handleAPIAuthLogout` | Yes |
| GET | `/api/v1/auth/me` | Current user | Session or Bearer | — | `{id,username,email,groups,is_admin,permissions}` | `handleAPIAuthMe` | Yes |
| GET | `/api/v1/auth/tokens` | List PATs (metadata) | Authenticated | — | `{tokens:[…]}` | `handleAPIAuthTokensList` | Yes |
| POST | `/api/v1/auth/tokens` | Create PAT | Authenticated | JSON name/description/expires | `{token,id,created_at,…}` plaintext once | `handleAPIAuthTokenCreate` | Yes |
| PUT | `/api/v1/auth/tokens/{id}` | Update PAT metadata | Authenticated | JSON name/description/expires | Token metadata | `handleAPIAuthTokenUpdate` | Yes |
| DELETE | `/api/v1/auth/tokens/{id}` | Revoke PAT (`?purge=1` hard delete) | Authenticated | Path `id` | 204 | `handleAPIAuthTokenDelete` | Yes |
| POST | `/api/v1/auth/tokens/{id}/rotate` | Rotate PAT | Authenticated | Path `id` | New plaintext once | `handleAPIAuthTokenRotate` | Yes |
| GET | `/profile.php`, `/profile` | Profile / API Tokens UI | Authenticated | — | HTML | `handleProfilePage` | No |

**Auth methods:** `jrs_session` cookie **or** `Authorization: Bearer jrs_…`. Bearer takes precedence when present. See [`authentication.md`](./authentication.md) and the full OpenAPI in [`../openapi/`](../openapi/) ([`openapi.md`](./openapi.md)).

**Known errors (API login):** 400 missing credentials / auth disabled; 401 invalid credentials; 503 auth unavailable.

---

## Health and version

| Method | Endpoint | Purpose | Auth | Request | Response | Handler | SDK |
|--------|----------|---------|------|---------|----------|---------|-----|
| GET | `/health` | Liveness | Public | — | `{"status":"ok","service":"acugis-report-server-go"}` | `handleHealth` | Yes |
| GET | `/ready` | Readiness (Postgres, FS, Jasper probe) | Public | — | 200 ready or 503 not_ready + `checks` | `handleReady` | Yes |
| GET | `/api/v1/diagnostics` | Ops diagnostics (redacted config, table presence, flags) | Public | — | JSON diagnostics object | `handleDiagnostics` | Yes |
| GET | `/api/openapi.yaml` | Bundled OpenAPI 3.0 YAML | Public | — | OpenAPI document (`info.version` = `JRS_APP_VERSION`) | `handleOpenAPIYAML` | Yes |
| GET | `/api/openapi.json` | Bundled OpenAPI 3.0 JSON | Public | — | Same as YAML | `handleOpenAPIJSON` | Yes |
| GET | `/swagger` | Swagger UI | Public | — | HTML (loads `/api/openapi.yaml`) | `handleSwaggerUI` | No |
| GET | `/dev/jasper-status` | Jasper gateway connectivity snapshot | Public | — | JSON status | `handleDevJasperStatus` | Yes |

---

## Reports

| Method | Endpoint | Purpose | Auth | Request | Response | Handler | SDK |
|--------|----------|---------|------|---------|----------|---------|-----|
| GET | `/` | Report catalog UI | Catalog | — | HTML | `handleCatalogPage` | No |
| GET | `/reports` | Catalog alias | Catalog | — | HTML | `handleCatalogPage` | No |
| GET | `/reports/index.php` | Catalog PHP-shaped alias | Catalog | — | HTML | `handleCatalogPage` | No |
| GET | `/api/v1/reports` | List visible reports | Catalog + ReportView filter | — | `{items:[{key,name,description,datasource,type,format,has_thumbnail}],count}` | `handleAPIReportsList` | Yes |
| POST | `/api/v1/reports` | Publish/create report | ServiceEdit | Multipart (see below) | 201 `{ok,key,message,notes,run_url,json_url}` | `handleAPIReportCreate` | Yes |
| GET | `/api/v1/reports/{key}` | Full report definition JSON | ReportView | Path `key` | Report JSON (`def.Raw`) | `handleAPIReportGet` | Yes |
| PUT | `/api/v1/reports/{key}` | Update report | ReportEdit | Multipart/form | `{ok,key}` | `handleAPIReportUpdate` | Yes |
| DELETE | `/api/v1/reports/{key}` | Delete report | ReportAdmin | Path `key` | `{ok,key}` | `handleDeleteReport` | Yes |
| GET | `/reports/publish_report.php` | Publish UI | ServiceEdit | — | HTML | `handlePublishPage` | No |
| POST | `/reports/publish_report.php` | Publish (form) | ServiceEdit | Multipart | HTML or JSON if `Accept: application/json` | `handlePublishPOST` | Partial |
| GET | `/reports/edit_report.php` | Edit UI | ReportEdit | Query `report` | HTML | `handleEditPage` | No |
| POST | `/reports/edit_report.php` | Save edit | ReportEdit | Multipart; query `report` | Redirect or JSON `{ok,key}` | `handleEditPOST` | Partial |
| GET | `/reports/delete_report.php` | Delete (legacy) | ReportAdmin | Query `report` | Redirect `/` or JSON | `handleDeleteReport` | Partial |
| GET | `/reports/{key}` | Report viewer page | ReportView | Path `key`; query `datasource` | HTML | `handleViewPage` | No |
| GET | `/reports/view.php` | PHP view → canonical path | — | Query `report`, `datasource` | 302 → `/reports/{key}` | `handleViewPagePHP` | No |
| GET | `/reports/report_runner.php` | URL builder UI | ReportView (per item) | — | HTML | `handleReportRunnerPage` | No |

**Publish/create multipart fields:** `jrxml_file` (required), optional `jasper_file`, `thumbnail_file`, `report_name`, `report_description`, `data_source` (required), `report_type` (`jasper`\|`map`), `access_groups` / `access_groups[]`, `params[N][…]`. Max multipart ~64 MiB.

**Known errors:** 400 validation / invalid key; 401/403 authz; 404 missing report; 500/503 store unavailable.

---

## Report execution

| Method | Endpoint | Purpose | Auth | Request | Response | Handler | SDK |
|--------|----------|---------|------|---------|----------|---------|-----|
| GET/POST | `/reports/execute` | Proxy Jasper report render | ReportView | Query `_repName` or `report`; `_repFormat` (default `html`); report params; POST body forwarded | Proxied bytes (HTML/PDF/…); optional `X-JRS-Embedded-Map*` | `handleDevReport` | Binary |
| GET/POST | `/reports/execute_report.php` | PHP-shaped execute alias | ReportView | Same | Same | `handleDevReport` | Binary |
| GET/POST | `/dev/report` | Dev execute alias | ReportView | Same | Same | `handleDevReport` | Binary |
| GET | `/report-image` | Proxy Jasper chart/QR image | ReportView | `_repName`/`report`, `image` | Image bytes | `handleDevReportImage` | Binary |
| GET | `/dev/report-image` | Dev image alias | ReportView | Same | Same | `handleDevReportImage` | Binary |

**Known errors:** 400 bad request; 403 ACL / invalid key; 404 missing report JSON; 503 Jasper/gateway — usually `text/plain`, not JSON. Does **not** require catalog login; Public ACL can allow anonymous execute.

---

## Parameters

| Method | Endpoint | Purpose | Auth | Request | Response | Handler | SDK |
|--------|----------|---------|------|---------|----------|---------|-----|
| GET | `/api/v1/reports/{key}/config` | Viewer config (enriched parameters, map meta) | ReportView | Path `key` | JSON config | `handleAPIReportConfig` | Yes |
| GET | `/reports/get_report_config.php` | Config PHP alias | ReportView | Query `report` | Same JSON | `handleReportConfig` | Yes |
| GET/POST | `/api/v1/reports/{key}/parameters/{name}/lookup` | Cascading parameter options | ReportView | Path `key`,`name`; GET query parents; POST JSON map or `{params\|parameters}` | `{options:[…]}` | `handleAPIParameterLookup` | Yes |
| GET/POST | `/reports/parameter_lookup.php` | Lookup PHP alias | ReportView | Query `report`, `parameter`/`name` | Same | `handleParameterLookupPHP` | Yes |
| GET | `/api/v1/reports/{key}/thumbnail` | Report thumbnail PNG | ReportView | Path `key` | `image/png` | `handleAPIReportThumbnail` | Binary |
| GET | `/reports/report_thumbnail.php` | Thumbnail PHP alias | ReportView | Query `report` | `image/png` | `handleReportThumbnail` | Binary |

---

## Folders

| Method | Endpoint | Purpose | Auth | Request | Response | Handler | SDK |
|--------|----------|---------|------|---------|----------|---------|-----|
| GET | `/api/v1/folders/tree` | ACL-filtered folder tree | Catalog + FolderView | — | `TreeNode` tree | `handleAPIFoldersTree` | Yes |
| GET | `/api/v1/folders/browse` | Browse folder contents | Catalog + FolderView | Query `folder_id` (0 = root/Home) | `{folder,breadcrumbs,folders,items}` | `handleAPIFoldersBrowse` | Yes |
| GET | `/api/v1/folders/search` | Search folders/items | Catalog | Query `q` (limit 50) | `{items,count}` | `handleAPIFoldersSearch` | Yes |
| POST | `/api/v1/folders` | Create folder | Catalog + FolderEdit on parent | JSON `{parent_id,name,description,icon,sort_order,acl_inherit?}` | 201 `Folder` | `handleAPIFolderCreate` | Yes |
| GET | `/api/v1/folders/{id}` | Get folder | FolderView | Path `id` | `Folder` | `handleAPIFolderGet` | Yes |
| PUT | `/api/v1/folders/{id}` | Update folder | FolderEdit | JSON patch fields | `Folder` | `handleAPIFolderUpdate` | Yes |
| DELETE | `/api/v1/folders/{id}` | Delete folder | FolderAdmin | Query `recursive`, `confirm` | `{ok:true}` | `handleAPIFolderDelete` | Yes |
| POST | `/api/v1/folders/{id}/move` | Move folder | FolderEdit src+dest | JSON `{parent_id,sort_order?}` | `Folder` | `handleAPIFolderMove` | Yes |
| POST | `/api/v1/folders/{id}/duplicate` | Duplicate folder | FolderEdit on parent | — | 201 `Folder` | `handleAPIFolderDuplicate` | Yes |
| GET | `/api/v1/folders/{id}/permissions` | List group grants | FolderView | Path `id` | `{group_names:[…]}` | `handleAPIFolderPermissions` | Yes |
| PUT | `/api/v1/folders/{id}/permissions` | Replace grants | FolderAdmin | JSON `{group_names,permission}` | `{ok:true}` | `handleAPIFolderPermissions` | Yes |
| POST | `/api/v1/content/move` | Place/move content in folder | FolderEdit (+ ReportEdit for reports) | JSON `{folder_id,content_type,content_key,…}` | `Item` | `handleAPIContentMove` | Yes |
| GET | `/api/v1/favorites` | List favorites | Catalog | — | `{items:[…]}` (empty if no real user) | `handleAPIFavorites` | Yes |
| POST | `/api/v1/favorites` | Set/unset favorite | Catalog + real user | JSON `{content_type,content_key,favorite?}` | `{ok,favorite}` | `handleAPIFavorites` | Yes |

**Known errors:** 400 bad request / confirm required; 403 ACL; 404; 409 conflict/cycle/not empty; 503 folders unavailable.

---

## Resources (attachments / content files)

| Method | Endpoint | Purpose | Auth | Request | Response | Handler | SDK |
|--------|----------|---------|------|---------|----------|---------|-----|
| GET | `/api/v1/resources` | List resources | Catalog + FolderView | Query `folder_id`, `resource_type`, `q` | `{items:[Resource]}` | `handleAPIResourcesList` | Yes |
| POST | `/api/v1/resources` | Create resource | FolderEdit | JSON or multipart (`file` / `content`) | 201 `Resource` | `handleAPIResourceCreate` | Yes |
| GET | `/api/v1/resources/{uuid}` | Get metadata | FolderView | Path `uuid` | `Resource` | `handleAPIResourceGet` | Yes |
| PUT | `/api/v1/resources/{uuid}` | Update metadata / move | FolderEdit | JSON `{display_name?,resource_type?,folder_id?}` | `Resource` | `handleAPIResourceUpdate` | Yes |
| PUT | `/api/v1/resources/{uuid}/content` | Replace file bytes | FolderEdit | Multipart `file` | `Resource` | `handleAPIResourceReplaceContent` | Yes |
| DELETE | `/api/v1/resources/{uuid}` | Delete | FolderEdit | Path `uuid` | `{ok,uuid}` | `handleAPIResourceDelete` | Yes |
| GET | `/api/v1/resources/{uuid}/download` | Download | FolderView | Query `inline` / `disposition` | Binary | `handleAPIResourceDownload` | Binary |
| GET | `/api/v1/resources/{uuid}/thumbnail` | Image thumbnail | FolderView | Path `uuid` | PNG or original | `handleAPIResourceThumbnail` | Binary |
| GET | `/api/v1/resources/images` | List images | FolderView | Query `folder_id`, `q` | `{items,supported_formats}` | `handleAPIImageResourcesList` | Yes |
| POST | `/api/v1/resources/images` | Upload image | FolderEdit | Multipart max 32 MiB `file` | 201 `Resource` | `handleAPIImageResourceCreate` | Yes |
| GET | `/api/v1/resources/images/{uuid}` | Image metadata | FolderView | Path `uuid` | `Resource` | `handleAPIImageResourceGet` | Yes |
| PUT | `/api/v1/resources/images/{uuid}` | Replace image | FolderEdit | Multipart `file` | `Resource` | `handleAPIImageResourceReplace` | Yes |
| DELETE | `/api/v1/resources/images/{uuid}` | Delete image | FolderEdit | Path `uuid` | `{ok,uuid}` | `handleAPIImageResourceDelete` | Yes |
| GET | `/api/v1/resources/images/{uuid}/download` | Download image | FolderView | inline/disposition | Binary | `handleAPIImageResourceDownload` | Binary |

**Resource JSON keys:** `id`, `uuid`, `folder_id`, `display_name`, `resource_type`, `original_filename`, `extension`, `storage_filename`, `mime_type`, `sha256`, `size_bytes`, `width?`, `height?`, `has_alpha?`, `created_by?`, `created_at`, `updated_at`, `folder_path?`, `resource_path?`.

### Resource sync

| Method | Endpoint | Purpose | Auth | Request | Response | Handler | SDK |
|--------|----------|---------|------|---------|----------|---------|-----|
| GET | `/api/v1/resource-sync` | Sync API hint | PlatformAdmin | — | `{ok,service,hint}` | `handleAPIResourceSync` | Yes |
| POST | `/api/v1/resource-sync` | Sync report JSON into ACL resources | PlatformAdmin | — | `{ok,registered,already_exists,errors,skipped,…}` | `handleAPIResourceSync` | Yes |
| GET/POST | `/api/resource-sync/` | Alias | PlatformAdmin | — | Same | `handleAPIResourceSync` | Yes |
| GET/POST | `/api/resource-sync/index.php` | Alias | PlatformAdmin | — | Same | `handleAPIResourceSync` | Yes |
| GET/POST | `/reports/resource_sync.php` | Sync admin UI | PlatformAdmin | POST form `sync_reports=1` | HTML | `handleResourceSyncPage` | No |

---

## Datasources

| Method | Endpoint | Purpose | Auth | Request | Response | Handler | SDK |
|--------|----------|---------|------|---------|----------|---------|-----|
| GET | `/api/v1/datasources` | List datasource names | Catalog | — | `[{"name":"…"},…]` | `handleAPIDatasourcesList` | Yes |
| POST | `/api/v1/datasources` | Create/upsert (or delete via form action) | ServiceAdmin | Form: `name`, `db_type`, `host`, `port`, `dbname`, `username`, `password`, `schema_name`, `auth_type`, key resource UUIDs; or `action=delete`+`id` | `{ok,message}` or `{error,errors}` | `handleAPIDatasourceCreate` | Partial |
| DELETE | `/api/v1/datasources/{id}` | Delete by id | ServiceAdmin | Path or query `id` | `{ok:true}` | `handleAPIDatasourceDelete` | Yes |
| POST | `/api/v1/datasources/context` | Write Tomcat `context.xml` | ServiceEdit | — | `{ok,path,notes}` | `handleGenerateJasperContext` | Yes |
| GET/POST | `/reports/generate_jasper_context.php` | Context PHP alias | ServiceEdit | — | JSON or `text/plain` | `handleGenerateJasperContext` | Partial |
| GET/POST | `/reports/datasources.php` | Datasource admin UI | ServiceAdmin | Same form as API | HTML (JSON if Accept) | `handleDatasourcesPage` | No |
| GET/POST | `/reports/geoserver_connections.php` | GeoServer connections UI | ServiceAdmin | Form upsert / `action=delete\|regenerate` | HTML | `handleGeoServerConnectionsPage` | No |
| GET/POST | `/reports/qgis_connections.php` | QGIS Server connections UI | ServiceAdmin | Form upsert / `action=delete\|regenerate` | HTML | `handleQGISConnectionsPage` | No |
| GET/HEAD | `/qgis/` | Reverse proxy to optional QGIS Server | Public (Designer GetMap) | WMS query; `MAP` rewritten to filename | Upstream WMS | `handleQGISProxy` | No |
| GET/POST | `/reports/arcgis_connections.php` | ArcGIS connections UI | ServiceAdmin | Same pattern | HTML | `handleArcGISConnectionsPage` | No |

**Note:** List API returns names only (no host/credentials). No dedicated PUT update route — create POST upserts by name.

---

## Map Reports

| Method | Endpoint | Purpose | Auth | Request | Response | Handler | SDK |
|--------|----------|---------|------|---------|----------|---------|-----|
| GET | `/api/v1/map/{key}/features` | GeoJSON features for map report | ReportView | Path `key`; query = filter params | GeoJSON FeatureCollection (+ meta) | `handleMapFeatures` | Yes |
| GET | `/reports/map_features.php` | Features PHP alias | ReportView | Query `report` + filters | Same | `handleMapFeaturesPHP` | Yes |
| GET | `/api/v1/map/{key}/extent` | Build spatial filter params | ReportView | `srid` + (`geometry` or `xmin,ymin,xmax,ymax`) | `{p_geometry,p_srid}` | `handleMapExtent` | Yes |
| GET | `/reports/map_extent.php` | Extent PHP alias | ReportView | Query `report` + same | Same | `handleMapExtentPHP` | Yes |
| GET | `/api/v1/map/{key}/columns` | Column/geometry metadata for edit | ReportEdit | Path `key` | `ColumnsResult` JSON | `handleMapColumns` | Yes |
| GET | `/reports/map_columns.php` | Columns PHP alias | ReportEdit | Query `report` | Same | `handleMapColumnsPHP` | Yes |

**Known errors:** 400 not a map report / missing params; 422 layer unresolved / not spatial; 500 datasource errors.

---

## Embedded maps

| Method | Endpoint | Purpose | Auth | Request | Response | Handler | SDK |
|--------|----------|---------|------|---------|----------|---------|-----|
| GET | `/api/v1/embedded-map/debug` | Diagnose embedded map path/JRXML/compile | ReportEdit | Query `report` (required); `format=json`; `compile=1` | JSON or HTML-wrapped JSON | `handleDebugEmbeddedMap` | Yes |
| GET | `/reports/debug_embedded_map.php` | Debug PHP alias | ReportEdit | Same | Same | `handleDebugEmbeddedMap` | Yes |

---

## Data Explorer

| Method | Endpoint | Purpose | Auth | Request | Response | Handler | SDK |
|--------|----------|---------|------|---------|----------|---------|-----|
| GET | `/explorer` | Data Explorer UI | Catalog | — | HTML | `handleExplorerPage` | No |
| GET | `/explorer/` | Alias | Catalog | — | HTML | `handleExplorerPage` | No |
| GET | `/explorer/index.php` | PHP-shaped alias | Catalog | — | HTML | `handleExplorerPage` | No |
| GET | `/api/v1/explorer/datasources` | Explorer-capable datasources | Catalog | — | `{datasources:[…]}` | `handleExplorerAPIDatasources` | Yes |
| GET | `/api/v1/explorer/tables` | List tables | Catalog | Query `datasource` | `{engine,tables}` | `handleExplorerAPITables` | Yes |
| GET | `/api/v1/explorer/metadata` | Table column metadata | Catalog | `datasource`, `schema`, `table` | `TableMeta` | `handleExplorerAPIMetadata` | Yes |
| POST | `/api/v1/explorer/query` | Run exploratory query | Catalog | JSON `QueryRequest` (max 1 MiB) | `QueryResult` | `handleExplorerAPIQuery` | Yes |
| POST | `/api/v1/explorer/report-query` | Query via report definition | Catalog + ReportView | JSON `{report_key,datasource?,parameters,max_rows}` | `QueryResult` | `handleExplorerAPIReportQuery` | Yes |
| POST | `/api/v1/explorer/summary` | Summary stats | Catalog | `SummaryRequest` | `SummaryStats` | `handleExplorerAPISummary` | Yes |
| POST | `/api/v1/explorer/statistics` | Statistics | Catalog | `StatisticsRequest` | `StatisticsResult` | `handleExplorerAPIStatistics` | Yes |
| POST | `/api/v1/explorer/spatial-analysis` | Spatial analysis | Catalog | `SpatialAnalysisRequest` | `SpatialAnalysisResult` | `handleExplorerAPISpatialAnalysis` | Yes |
| POST | `/api/v1/explorer/spatial-analysis/validate` | Validate spatial request | Catalog | Same | Validation result | `handleExplorerAPISpatialValidate` | Yes |
| GET | `/api/v1/explorer/spatial-analysis/capabilities` | Spatial capabilities | Catalog | Query `engine`, `source_type` | Capability map | `handleExplorerAPISpatialCapabilities` | Yes |
| POST | `/api/v1/explorer/features` | GeoJSON features | Catalog | `FeaturesRequest` | FeatureCollection | `handleExplorerAPIFeatures` | Yes |
| GET | `/api/v1/explorer/map/sources` | Map connection sources | Catalog | — | `{sources,spatial,connections}` | `handleExplorerAPIMapSources` | Yes |
| POST | `/api/v1/explorer/map/discover` | Discover map table | Catalog | `{connection,schema,table}` | `MapDiscoverResult` | `handleExplorerAPIMapDiscover` | Yes |
| GET | `/api/v1/explorer/map/tables` | Map tables | Catalog | `connection` or `datasource` | `{tables,engine}` | `handleExplorerAPIMapTables` | Yes |
| POST | `/api/v1/explorer/export` | Export query result | Catalog | `QueryRequest`; query `format`=csv\|excel\|geojson\|gpkg\|shapefile | File download | `handleExplorerAPIExport` | Binary |
| POST | `/api/v1/explorer/handoff` | Handoff payload for publish | Catalog | `QueryRequest` | `{handoff,publish_url,note}` | `handleExplorerAPIHandoff` | Yes |
| GET | `/api/v1/explorer/views` | List saved views | Catalog | — | `{views:[SavedView]}` | `handleExplorerAPIViewsList` | Yes |
| POST | `/api/v1/explorer/views` | Save view | Catalog | `SavedView` JSON | Saved view | `handleExplorerAPIViewSave` | Yes |
| GET | `/api/v1/explorer/views/{key}` | Get saved view | Catalog | Path `key` | `SavedView` | `handleExplorerAPIViewGet` | Yes |
| DELETE | `/api/v1/explorer/views/{key}` | Delete view | Catalog | Path `key` | `{status:"deleted"}` | `handleExplorerAPIViewDelete` | Yes |
| POST | `/api/v1/explorer/views/{key}/dashboard` | Flag view as dashboard widget | Catalog | — | `{status,widget:{…}}` | `handleExplorerAPIViewDashboard` | Yes |
| GET | `/api/v1/explorer/dashboard-widgets` | List dashboard widgets | Catalog | — | `{widgets:[SavedView]}` | `handleExplorerAPIDashboardWidgets` | Yes |

**Explorer errors:** 400/403/404/500; 422 `unsupported_datasource`; 503 explorer unavailable.

**`QueryRequest` core fields:** `datasource`, `schema`, `table`, `columns`, `filters`, `group_by`, `aggregates`, `sort`, `limit`/`offset`/`page`/`page_size`, `source_mode`, `report_key`, `report_cache_key`.

---

## Dashboards

| Method | Endpoint | Purpose | Auth | Request | Response | Handler | SDK |
|--------|----------|---------|------|---------|----------|---------|-----|
| GET | `/api/v1/explorer/dashboard-widgets` | List flagged explorer views | Catalog | — | `{widgets:[…]}` | `handleExplorerAPIDashboardWidgets` | Yes |
| POST | `/api/v1/explorer/views/{key}/dashboard` | Mark view as widget | Catalog | Path `key` | Widget ref JSON | `handleExplorerAPIViewDashboard` | Yes |

No separate dashboard layout/CRUD API beyond explorer view flags.

---

## Users

| Method | Endpoint | Purpose | Auth | Request | Response | Handler | SDK |
|--------|----------|---------|------|---------|----------|---------|-----|
| GET | `/admin/users.php` | Users admin UI | PlatformAdmin | Query `id` for edit | HTML | `handleAdminUsersPage` | No |
| POST | `/admin/users.php` | Create/update/delete user | PlatformAdmin | Form: `id`, `username`, `email`, `password`, `is_admin`, `enabled`, `groups`; or `action=delete`+`id` | HTML | `handleAdminUsersPOST` | No |

**Known constraints:** cannot delete self; duplicate username rejected. No `/api/v1/users` REST surface.

---

## Groups

| Method | Endpoint | Purpose | Auth | Request | Response | Handler | SDK |
|--------|----------|---------|------|---------|----------|---------|-----|
| GET | `/admin/groups.php` | Groups admin UI | PlatformAdmin | Query `id` | HTML | `handleAdminGroupsPage` | No |
| POST | `/admin/groups.php` | Create/update/delete group | PlatformAdmin | Form: `name`, `description`, `members`; or `action=delete`+`id` | HTML | `handleAdminGroupsPOST` | No |

**Known constraints:** protected groups `Administrators` / `Public` cannot be deleted. No `/api/v1/groups` REST surface.

---

## Permissions

Folder permissions are REST:

| Method | Endpoint | Purpose | Auth | Handler | SDK |
|--------|----------|---------|------|---------|-----|
| GET | `/api/v1/folders/{id}/permissions` | List folder group grants | FolderView | `handleAPIFolderPermissions` | Yes |
| PUT | `/api/v1/folders/{id}/permissions` | Replace folder grants | FolderAdmin | `handleAPIFolderPermissions` | Yes |

Report ACL is set during publish/edit via `access_groups` form fields (multipart), not a dedicated permissions API. Resource permissions inherit from folder ACL. No general `/api/v1/permissions` or per-report ACL REST endpoint.

---

## Schedules

| Method | Endpoint | Purpose | Auth | Request | Response | Handler | SDK |
|--------|----------|---------|------|---------|----------|---------|-----|
| GET | `/schedules.php` | Schedules list UI | PlatformAdmin | — | HTML | `handleSchedulesPage` | No |
| GET | `/edit_schedule.php` | Create/edit schedule UI | PlatformAdmin | Query `id` (0 = add) | HTML | `handleEditSchedulePage` | No |
| POST | `/action/schedule.php` | Schedule CRUD + run-now | PlatformAdmin | Form flags `create`\|`update`\|`delete`\|`run_now` + schedule fields | JSON `{success,message,id?,execution_id?,…}` | `handleScheduleAction` | Partial |

**Schedule form fields (create/update):** `schedule_mode` + mode fields, `report_name`/`name`, `datasource_name`, `format`, `filename`, `email`/`noemail`, `email_subject`, `email_body`, `email_template`, `url_opt_params`, `enabled`, `id` (update).

**CLI:** `jrs-run-schedule <schedule_id>` runs a schedule headlessly (not HTTP).

---

## Jobs (executions / history)

| Method | Endpoint | Purpose | Auth | Request | Response | Handler | SDK |
|--------|----------|---------|------|---------|----------|---------|-----|
| GET | `/executions.php` | Per-schedule last status UI | PlatformAdmin | — | HTML | `handleExecutionsPage` | No |
| GET | `/history.php` | Execution history UI | PlatformAdmin | Query `schedule_id` | HTML (last 200) | `handleHistoryPage` | No |
| POST | `/action/schedule.php` (`run_now`) | Trigger immediate run | PlatformAdmin | Form `run_now`+`id` | JSON `{success,message,execution_id,…}` | `handleScheduleRunNow` | Partial |

No `/api/v1/schedules` or `/api/v1/executions` list/get REST endpoints. Failed `run_now` may still return HTTP 200 with `success:false`.

---

## Recipients (schedule address book)

| Method | Endpoint | Purpose | Auth | Request | Response | Handler | SDK |
|--------|----------|---------|------|---------|----------|---------|-----|
| GET | `/recipients.php` | Recipients list UI | PlatformAdmin | — | HTML | `handleRecipientsPage` | No |
| GET | `/edit_recipient.php` | Add/edit recipient UI | PlatformAdmin | Query `id` | HTML | `handleEditRecipientPage` | No |
| POST | `/action/recipient.php` | Save/delete recipient | PlatformAdmin | Form `id?`, `name`, `email`, `notes` or `delete`+`id` | JSON `{success,message}` | `handleRecipientAction` | Partial |

---

## Administration (hubs / workspace)

| Method | Endpoint | Purpose | Auth | Request | Response | Handler | SDK |
|--------|----------|---------|------|---------|----------|---------|-----|
| GET | `/workspace.php` | Federation workspace shell | Catalog | — | HTML | `handleWorkspacePage` | No |
| GET | `/hubs/{hub}` | Hub shell (`reports`, `data`, `data-explorer`, `scheduling`, `administration`; `.php` suffix stripped) | Catalog; `scheduling`/`administration` also PlatformAdmin | Path `hub` | HTML | `handleHubPage` | No |

Also see Users, Groups, Schedules, Resource sync (platform admin).

---

## Licensing

No licensing HTTP endpoints are registered in the Go server.

---

## Static assets (no auth)

Mounted by `mountStaticAssets` (not application APIs):

| Path prefix | Source |
|-------------|--------|
| `/shared/ui/*` | `{AppRoot}/shared/ui` |
| `/reports/portal.css` | `{AppRoot}/reports/portal.css` |
| `/assets/reports/*` | Thumbnail dir / `{AppRoot}/assets/reports` |
| `/assets/js/*` | `{AppRoot}/assets/js` |

---

## Route registration notes

- All application routes are registered in `Server.routes()` via a local `mount` function on a Chi router.
- When `JRS_BASE_PATH` ≠ `/`, routes mount under that prefix; static UI is also exposed at root for assets.
- Unknown paths return JSON `404` `{"error":"not found","service":"acugis-report-server-go"}`.
- Many PHP-shaped aliases coexist with `/api/v1/*` for UI compatibility; prefer `/api/v1/*` for new clients.

---

## Endpoint counts (approximate)

| Area | Routes (incl. aliases & method variants) |
|------|------------------------------------------|
| Auth | 5 |
| Health / diagnostics | 4 |
| Reports + catalog UI | ~16 |
| Report execution / images | 5 |
| Parameters / thumbnails | 6 |
| Folders / favorites / content | 14 |
| Resources / images / sync | ~20 |
| Datasources / GeoServer / ArcGIS / context | 8 |
| Map Reports | 6 |
| Embedded map debug | 2 |
| Data Explorer + dashboard | ~25 |
| Admin users/groups | 4 |
| Schedules / jobs / recipients | 9 |
| Hubs / workspace | 2 |
| **Licensing** | **0** |

Exact method+path registrations: see `internal/server/routes.go` (`mount` closure).
