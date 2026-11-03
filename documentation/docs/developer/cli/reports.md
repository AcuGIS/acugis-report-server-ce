# CLI reports

## Catalog

```bash
acugis-report reports list
acugis-report reports show Invoice
acugis-report reports config Invoice
acugis-report reports thumbnail Invoice --output thumb.png
```

## Publish and update

```bash
acugis-report reports publish report.jrxml --data-source postgres --name Invoice
acugis-report reports update Invoice --jrxml report.jrxml
acugis-report reports delete Invoice --yes
```

## Run

```bash
acugis-report reports run Invoice \
  --param InvoiceID=123 \
  --output invoice.pdf
```

Multi-value parameters:

```bash
acugis-report reports run Sales \
  --param region=West \
  --param region=North \
  --format xlsx \
  --output sales.xlsx
```

Parameter file:

```bash
acugis-report reports run Invoice --params params.json --output invoice.pdf
```

Supported formats: `html`, `pdf`, `xlsx`, `xls`, `csv`, `docx`, `rtf`, `odt`, `ods`, `pptx`, `xml`.

Binary results are written to a file (or `--stdout`). They are never dumped to the terminal by default.

## Scripting

```bash
acugis-report --output json reports list | jq '.items[].key'
```
