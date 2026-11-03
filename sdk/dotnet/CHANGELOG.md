# Changelog

All notable changes to the AcuGIS.Report .NET SDK are documented in this file.

## 0.1.0 — 2026-08-06

### Added

- Initial official .NET 8 SDK (`AcuGIS.Report`) for AcuGIS Report Server
- `ReportServerClient` with Auth, Reports, Folders, Resources, Datasources, MapReports, EmbeddedMaps, Explorer, Health
- Reserved Users / Groups / Schedules / Jobs / Recipients surfaces (`NotImplementedByServerException`)
- Typed exceptions with preserved server response bodies
- `IHttpClientFactory` / DI via `AddAcuGISReport`
- Environment configuration (`ACUGIS_REPORT_URL`, `ACUGIS_REPORT_TOKEN`)
- Report execution helpers (`RunAsync`, `RunStreamAsync`, `RunToFileAsync`) with multi-value parameters
- Multipart report publish/update and resource uploads
- Optional GET-only retries, custom headers, timeout, and TLS verify toggle
