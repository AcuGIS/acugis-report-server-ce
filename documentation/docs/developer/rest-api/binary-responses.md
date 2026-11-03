# Binary responses

Several endpoints return files instead of JSON.

## Report execute

| Path | Formats |
|------|---------|
| `/reports/execute` | `html`, `pdf`, `xlsx`, `xls`, `csv`, `docx`, `rtf`, `odt`, `ods`, `pptx`, `xml` |

Parameters: `_repName`, `_repFormat`, plus report parameters.

```bash
curl -sS -H "Authorization: Bearer $ACUGIS_REPORT_TOKEN" \
  -o invoice.pdf \
  "$ACUGIS_REPORT_URL/reports/execute?_repName=Invoice&_repFormat=pdf&InvoiceID=123"
```

## Explorer export (Professional)

Data Explorer export is not implemented by the Open Source server. It requires Report Server Professional.

`POST /api/v1/explorer/export?format=`

| Format | Notes |
|--------|-------|
| `csv` | Text |
| `excel` | Spreadsheet |
| `geojson` | Text |
| `gpkg` | ZIP |
| `shapefile` | ZIP |

## Images and thumbnails

| Path | Notes |
|------|-------|
| `/api/v1/reports/{key}/thumbnail` | PNG |
| `/api/v1/resources/{uuid}/download` | Original bytes |
| `/api/v1/resources/{uuid}/thumbnail` | Preview |
| Report image gateway | Used by HTML report viewers |

## Client guidance

- Always write binary bodies to a file or stream
- Read `Content-Type` when present
- Treat `text/plain` error bodies from the execute gateway as failures even when you expected a PDF
- CLI defaults to file output and avoids dumping binaries to the terminal
