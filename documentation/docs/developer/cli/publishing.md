# Publishing reports (CLI)

```bash
acugis-report reports publish report.jrxml \
  --data-source postgres \
  --name Invoice
```

Update an existing key:

```bash
acugis-report reports update Invoice --jrxml report.jrxml
```

```bash
acugis-report reports delete Invoice --yes
```

## After publish

```bash
acugis-report reports list
acugis-report reports config Invoice
acugis-report reports run Invoice --param InvoiceID=1 --output out.pdf
```

## Related

- [CLI reports](reports.md)
- [Tutorial — Publishing a report](../tutorials/publishing-a-report.md)
- [User Guide — Publishing](../../user-guide/designer/publishing.md)
