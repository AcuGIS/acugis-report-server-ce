# Embedding API reference

## `AcuGIS.embedReport(options)` / `embedDashboard` / `embedView` / `embedMapReport`

Returns an `Embed` instance.

### Options

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `server` | string | yes | Report Server base URL |
| `target` | string \| HTMLElement | yes | Mount point |
| `reportId` / `dashboardId` / `viewId` / `mapReportId` | string | yes* | Content key |
| `token` or `auth` | string / object | **yes** | `emb_` embed token |
| `parameters` | object | no | Interactive parameters. They do not override bound parameters signed into the embed token. |
| `width` / `height` | string \| number | no | iframe size |
| `autoResize` | boolean | no | |
| `viewer` | object | no | toolbar, parameters, export, zoom, close |
| `datasource` | string | no | Report datasource override |

### `AcuGIS.embed({ type, id, … })`

`type`: `report` \| `dashboard` \| `view` \| `map-report`.

### Server routes

| Route | Purpose |
|-------|---------|
| `POST /api/v1/embed/token` | Mint `emb_` token (session/PAT). Optional `parameters` are bound parameters. |
| `GET /embed/report/{key}` | Embedded report (requires `et`) |
| `GET /embed/map-report/{key}` | Embedded map report |
| `GET /embed/dashboard/{owner}/{key}` | Embedded dashboard |
| `GET /embed/view/{owner}/{key}` | Embedded view |
| `/sdk/acugis-report(.min).js` | Browser SDK |
| `/sdk/v1/…` | Versioned SDK assets |
