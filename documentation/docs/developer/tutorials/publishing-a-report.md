# Publishing a report

Workflow: author JRXML → publish to the catalog → verify → run.

## 1. Author

Design the JRXML in Report Designer with stable parameter names and matching datasource connection names. See [Designer](../../user-guide/designer/overview.md).

## 2. Publish via CLI

```bash
acugis-report reports publish report.jrxml \
  --data-source postgres \
  --name Invoice
```

Or via REST multipart `POST /api/v1/reports` — see [CLI publishing](../cli/publishing.md) and [REST reports](../rest-api/reports.md).

## 3. Set access

Include `access_groups` at publish/update time. Folder ACLs still apply.

## 4. Verify

```bash
acugis-report reports list
acugis-report reports config Invoice
acugis-report reports run Invoice --param InvoiceID=1 --output out.pdf
```

## 5. Automate

See [GitHub Actions](github-actions.md).

## Related

- [User Guide — Publishing](../../user-guide/designer/publishing.md)
- [Running a report](running-a-report.md)
