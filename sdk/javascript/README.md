# @acugis/report-embed

Official **JavaScript/TypeScript Embedding SDK** for [AcuGIS Report Server](https://acugis.com).

This is the high-level Embedded Analytics SDK (`embedReport`, parameters, events, exports) — not a generated REST client.

## Requirement: embed tokens

Embedding **always** requires a short-lived **embed token** (`emb_…`).

1. Your backend authenticates with a session or Personal Access Token (`jrs_…`).
2. Your backend calls `POST /api/v1/embed/token` to mint an `emb_` token scoped to content.
3. Your frontend passes only the `emb_` token into the SDK.

Personal access tokens and session cookies are **not** accepted by the browser SDK.

```http
POST /api/v1/embed/token
Authorization: Bearer jrs_…
Content-Type: application/json

{
  "content_type": "report",
  "content_id": "Invoice",
  "origin": "https://app.example.com"
}
```

```json
{
  "token": "emb_…",
  "expires_at": "2026-08-11T12:30:00Z",
  "content_type": "report",
  "content_id": "Invoice"
}
```

## Quick start (CDN)

```html
<div id="report"></div>
<script src="https://reports.example.com/sdk/acugis-report.min.js"></script>
<script>
AcuGIS.embedReport({
  server: "https://reports.example.com",
  target: "#report",
  reportId: "REPORT_KEY",
  token: embedTokenFromYourBackend
});
</script>
```

## npm / ES modules

```bash
npm install @acugis/report-embed
```

```javascript
import { AcuGIS } from "@acugis/report-embed";

const report = AcuGIS.embedReport({
  server: "https://reports.example.com",
  target: "#report",
  reportId: "Invoice",
  auth: { type: "token", token: embedToken },
  parameters: { p_country: "USA" },
});

await report.ready();
report.setParameters({ p_country: "Canada" });
report.export("pdf");
```

## API

- `AcuGIS.embedReport` / `embedDashboard` / `embedView` / `embedMapReport`
- `AcuGIS.embed({ type, id, … })`
- Instance: `ready`, `on`/`off`, `setParameters`, `getParameters`, `refresh`, `resize`, `setHeight`, `setWidth`, `export`, `zoomIn`/`zoomOut`/`setZoom`, `destroy`

## Build

```bash
npm install
npm test
npm run build
```

Outputs: `dist/acugis-report.js`, `acugis-report.min.js`, `acugis-report.esm.js`, `index.d.ts`.

## Security

- Never put `jrs_` PATs in the browser or iframe URLs. Mint `emb_` tokens on the host backend.
- Host-owned values (`account_id`) are bound parameters on the embed token. Do not try to change them with `setParameters()`. Mint a new token. Interactive parameters (`year`, `property_id`) still use `parameters` / `setParameters()`.
- Configure `ACUGIS_EMBED_ALLOWED_ORIGINS` on Report Server.
- Only `/embed/*` routes are frameable; the normal UI keeps `X-Frame-Options: SAMEORIGIN`.
- `postMessage` validates origin, source window, protocol `source`, and version.
