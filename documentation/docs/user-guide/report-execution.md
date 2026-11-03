# Report execution

**Report execution** fills a published Jasper definition with parameters and returns binary output in the format you choose.

## Running from the viewer

1. Open the catalog and select a report (or go to `/reports/{key}`).
2. Supply [parameters](parameters.md).
3. Choose an [output format](output-formats.md).
4. Run and download or view the result.

Map Reports, available in Professional, add an interactive map step before fill. They are not part of this Open Source server.

## What happens on the server

1. Authenticate (session or API Token — same ACL).
2. Authorize the report for your user/groups.
3. Resolve parameters (including Map Report-derived values when applicable).
4. JasperReports fills against the configured datasource / JNDI context.
5. Output is streamed as PDF, HTML, Excel, and so on.

## Formats

Common formats include `pdf`, `html`, `xlsx`, `xls`, `csv`, `docx`, and others. Full list: [Output formats](output-formats.md) and [Supported formats](../reference/supported-formats.md).

## Troubleshooting execution

If output is empty or fails:

- Confirm datasource name and connectivity
- Confirm parameter names and types
- Check fonts/images/resources referenced by the JRXML
- Ask an administrator to review Jasper internal URL and timeout settings

See [Troubleshooting](../administration/troubleshooting.md).

## Related

- [Reports](reports.md)
- [Parameters](parameters.md)
- [Output formats](output-formats.md)

---

## For Developers

| Integration path | Go here |
|------------------|---------|
| Execute gateway | [REST API](../developer/rest-api/overview.md) · [Binary responses](../developer/rest-api/binary-responses.md) |
| CLI `reports run` | [CLI reports](../developer/cli/reports.md) · [Running reports from the CLI](../developer/tutorials/running-reports-cli.md) |
| SDK `Reports.Run` | [SDK overview](../developer/sdk/overview.md) |
