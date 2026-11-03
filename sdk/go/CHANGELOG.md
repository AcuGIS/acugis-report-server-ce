# Changelog

## 0.1.0 — 2026-08-06

Initial release of the AcuGIS Report Server Go SDK.

- Bearer-token client with `NewClient` / `NewClientFromEnv`
- Service namespaces: Auth, Reports, Folders, Resources, Datasources, MapReports, EmbeddedMaps, Explorer, Health
- Reserved stubs: Users, Groups, Schedules, Jobs, Recipients
- Typed HTTP errors mapped from status codes
- Report run helpers: `Run`, `RunToFile`, `RunStream`
- Unit tests with `net/http/httptest`
