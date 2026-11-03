# Output formats

When you run a report, you choose how JasperReports exports the filled document.

## Common report formats

| Format | Typical use |
|--------|-------------|
| `pdf` | Printable / archival documents |
| `html` | Browser viewing |
| `xlsx` / `xls` | Spreadsheet analysis |
| `csv` | Raw tabular export |
| `docx` / `rtf` / `odt` | Word-processing workflows |
| `ods` | OpenDocument spreadsheet |
| `pptx` | Presentation export |
| `xml` | Structured XML export |

The authoritative MIME / format table is in [Supported formats](../reference/supported-formats.md).

## Data Explorer exports

Data Explorer export is available in Professional. It is not part of this Open Source server, and this guide does not document those pages.

## Related

- [Report execution](report-execution.md)
- [Supported formats (reference)](../reference/supported-formats.md)

---

## For Developers

Format names are shared across the execute gateway, CLI (`--format`), and SDKs. See [REST binary responses](../developer/rest-api/binary-responses.md) and [CLI reports](../developer/cli/reports.md).
