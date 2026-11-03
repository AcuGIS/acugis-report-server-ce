# Reports package

Loads and writes PHP report definitions under the shared filesystem layout.

## Behavior

- List: glob `*.report.json`, sort by `name` (case-insensitive)
- Get: `{jsonDir}/{key}.report.json`
- SaveJSON / SaveJRXML / SaveJasper / SaveThumbnail / Delete / CompileJRXML
- Key normalization matches PHP `jrs_report_key` / `jrs_is_valid_report_key`
- Empty `report` field falls back to filename stem
- Paths: JSON under `reports/json`, JRXML/.jasper under `docker/report-server/reports`,
  thumbnails under `assets/reports/{key}.png`

## Catalog vs runtime ACL

- Catalog UI/API: login + service `jasper_reports` view, then per-report view filter
- Publish: service `jasper_reports` edit
- Edit: report `edit` (plus service edit gate on the page)
- Delete: report `admin`
- Viewer / execute / image: report `view` only (Public / anonymous allowed)
- Publish/edit sync access groups via `acl.ResourceWriter.SyncReportAccessGroups`
  (replace semantics: prior group grants are cleared, then selected groups applied)
