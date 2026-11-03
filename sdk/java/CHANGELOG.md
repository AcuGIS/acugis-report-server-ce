# Changelog

All notable changes to the AcuGIS Report Server Java SDK will be documented in this file.

## [0.1.0] — 2026-08-06

### Added

- Initial official Java SDK (`com.acugis:acugis-report:0.1.0`)
- `ReportServerClient` builder with env support (`ACUGIS_REPORT_URL`, `ACUGIS_REPORT_TOKEN`)
- Services: `auth`, `reports`, `folders`, `resources`, `datasources`, `mapReports`, `embeddedMaps`, `explorer`, `health`
- Reserved surfaces (`users`, `groups`, `schedules`, `jobs`, `recipients`) throw `NotImplementedByServerException`
- Report execution via `/reports/execute` with multi-value parameters and `runToFile`
- Folder ops including duplicate, move content, and favorites
- Resource images, sync, and download-to-`Path`
- Typed exception hierarchy aligned with other AcuGIS SDKs
- Optional GET-only retries, custom headers, TLS verify toggle
- Async helpers: `listAsync`, `runAsync`, `meAsync`
- Console and Spring Boot examples
- JUnit 5 tests against JDK `HttpServer`
