# Running reports from the CLI

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

## Related

- [CLI reports](../cli/reports.md)
- [CLI automation](../cli/automation.md)
- [Running a report](running-a-report.md)
