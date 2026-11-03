# Jasper Gateway

This package is the **only** Go component that communicates with the internal
Tomcat/JasperReports engine.

## Scope (Phase 2)

Implemented:

- `Gateway` service interface (`ExecuteReport`, `FetchReportImage`)
- Streaming execute proxy (PDF/HTML/XLSX/DOCX/CSV/XML/…)
- Streaming image proxy
- Raw query / body parameter forwarding (no interpretation)
- Context-based timeouts (default 600s)
- Structured request logging helpers
- Gateway error translation (no Java stack traces to clients)
- Development status snapshot (`Status()`)

Not implemented here:

- Report publishing / editing
- Datasource management
- Schedules
- Real ACL / PHP session auth (placeholders live in `internal/acl` and `internal/auth`)

## Tomcat endpoints

Same internal URLs as PHP:

- Report execute: `JRS_JASPER_INTERNAL_REPORT_URL` (default `http://127.0.0.1:9099/report`)
- Report images: `JRS_JASPER_INTERNAL_IMAGE_URL` (default `http://127.0.0.1:9099/acugis_report_image`)

## Development HTTP routes

Mounted by `internal/server`:

| Method | Path | Purpose |
|--------|------|---------|
| GET, POST | `/dev/report` | Execute proxy |
| GET | `/dev/report-image` | Image proxy |
| GET | `/dev/jasper-status` | Gateway status |

These routes must not replace production PHP URLs.

## Usage

```go
gw := jasper.NewGateway(cfg.Jasper, jasper.Options{
    ReportJSONDir: cfg.Paths.ReportJSONDir,
})
resp, err := gw.ExecuteReport(ctx, jasper.ExecuteRequest{...})
```

No other package should construct requests to Tomcat directly.
