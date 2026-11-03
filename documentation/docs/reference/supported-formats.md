# Supported formats

## Report execute (`/reports/execute`)

| Format | Typical MIME |
|--------|--------------|
| `html` | `text/html` |
| `pdf` | `application/pdf` |
| `xlsx` | Excel Open XML |
| `xls` | Excel legacy |
| `csv` | `text/csv` |
| `docx` | Word Open XML |
| `rtf` | RTF |
| `odt` | OpenDocument text |
| `ods` | OpenDocument spreadsheet |
| `pptx` | PowerPoint Open XML |
| `xml` | XML export |

CLI / SDK accept the same format names (with or without a leading dot in some helpers).

## Explorer export

| Format | Notes |
|--------|-------|
| `csv` | Text |
| `excel` | Spreadsheet |
| `geojson` | Text |
| `gpkg` | GeoPackage (ZIP response) |
| `shapefile` | Shapefile (ZIP response) |

## Other binary outputs

- Report thumbnails (PNG)
- Resource downloads / thumbnails
- Report image gateway output for HTML viewers
