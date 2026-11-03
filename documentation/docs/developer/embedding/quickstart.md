# Quick Start

## 1. Mint an embed token (backend)

```bash
curl -s -X POST "$ACUGIS_REPORT_URL/api/v1/embed/token" \
  -H "Authorization: Bearer $ACUGIS_REPORT_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"content_type":"report","content_id":"Invoice","origin":"https://app.example.com"}'
```

## 2. Embed the report (frontend)

```html
<div id="report"></div>

<script src="https://reports.example.com/sdk/acugis-report.min.js"></script>

<script>
AcuGIS.embedReport({
  server: "https://reports.example.com",
  target: "#report",
  reportId: "Invoice",
  token: EMBED_TOKEN_FROM_YOUR_BACKEND
});
</script>
```

That is the entire Quick Start. Authentication details, parameters, events, and dashboards are covered in the following pages.
