# ACL package

Database-backed access control matching PHP `lib/auth.php` and
`lib/service_acl.php`. Read path is used for authorization; write helpers
support publish/edit ACL sync and full resource catalog backfill
(`SyncAllReportResources`).

`SyncReportAccessGroups` **replaces** report grants (delete prior rows, then
apply selected groups + Administrators `admin`), matching folder
`ReplaceFolderPermissions`. Unchecking a group on edit removes that group’s
`resource_permissions` row so members lose view/execute immediately.

## Resource types

| Type | Key examples | Used for |
|------|--------------|----------|
| `jasper_report` | report key (`^[a-z0-9_-]+$`) | Execute, image, view UI |
| `explorer_view` | `owner/viewKey` | Saved Data Explorer Views and Dashboards (`is_dashboard`) |
| `service` | `jasper_reports` | Catalog / hub entry |

Permissions: `view`, `edit`, `admin`. Hierarchy inside SQL: exact match, or
`admin`, or `edit` satisfying a `view` request.

Dashboards are not a separate ACL resource: they are the same `explorer_view`
row. Listing filters by `is_dashboard` on the saved JSON.

`SyncExplorerViewAccessGroups` mirrors report Access Groups replace semantics
(empty selection → Public `view` + Administrators `admin`).

## Verified precedence

Confirmed from PHP (not assumed):

1. `JRS_AUTH_DISABLED` → allow all
2. `dashboard_users.is_admin` → allow
3. Admin-named groups (`administrator`, `administrators`, `admin`, `admins`, `dashboard-admin`) → allow
4. Group grant on `resources` / `resource_permissions`, including **implicit Public** membership via `jrs_auth_effective_group_names_for_permission`
5. Folder inheritance for folders / content resources (not `jasper_report` / `explorer_view`)
6. Default deny

There are **no per-user ACL rows** in the schema. Only group grants exist.

Report and View/Dashboard Access Groups are authoritative for view/execute.
Folder ACL (including Home’s default Public `view`) does **not** grant report
or explorer view access.

## Public group

Every permission check adds `Public` when the user is not already a member.
Anonymous callers (user id 0 / nil user) therefore still receive Public grants.
Report execute and image proxies use `jrs_require_report_runtime_permission`
semantics: login is not required; missing permission → **403**.

## Service access

`CanAccessReportService` mirrors `jrs_require_service_access('jasper_reports')`,
including the fallback that allows service `view` when the user can view any
`jasper_report` resource.

## Report execution vs service ACL

`execute_report.php` / `report_image.php` check **only** `jasper_report` +
`view`. They do not require service-level `jasper_reports` access.

## Image authorization

There is **no signed image key**. Authorization is the `_repName` / `report`
query parameter plus report `view` ACL. Invalid report key → **403** (PHP
`report_image.php`). HTML rewrite injects `_repName` into image URLs.
