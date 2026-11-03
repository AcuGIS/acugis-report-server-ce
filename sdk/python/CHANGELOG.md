# Changelog

All notable changes to the AcuGIS Report Python SDK are documented in this file.

## 0.1.0 — 2026-08-06

### Added

- Initial official Python SDK and CLI (`acugis-report`) for AcuGIS Report Server
- `ReportServer` client with service namespaces: `auth`, `reports`, `folders`, `resources`, `datasources`, `map_reports`, `embedded_maps`, `explorer`, `health`
- Reserved `users` / `groups` / `schedules` / `jobs` / `recipients` surfaces (`NotImplementedByServerError`)
- Backward-compatible flat methods alongside namespaced APIs
- Report execution helpers (`run_report`, `run_to_file`) with multi-value parameters
- Typed exceptions with preserved server response bodies
- CLI (`acugis-report`) wrapping the same client
