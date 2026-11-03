# Running a report

End-to-end execute workflow with an API Token.

## 1. Authenticate

```bash
export ACUGIS_REPORT_URL=https://reports.example.com
export ACUGIS_REPORT_TOKEN=jrs_xxxxxxxxx
acugis-report whoami
```

## 2. List and inspect

```bash
acugis-report reports list
acugis-report reports config Invoice
```

## 3. Run (CLI)

```bash
acugis-report reports run Invoice \
  --param InvoiceID=123 \
  --output invoice.pdf
```

## 4. Run (REST)

```bash
curl -sS -H "Authorization: Bearer $ACUGIS_REPORT_TOKEN" \
  -o invoice.pdf \
  "$ACUGIS_REPORT_URL/reports/execute?_repName=Invoice&_repFormat=pdf&InvoiceID=123"
```

## 5. Run (SDK)

See the Invoice workflow on [SDK overview](../sdk/overview.md).

## Related

- [CLI reports](../cli/reports.md)
- [Parameters & execute](../rest-api/parameters.md)
- [Running reports from the CLI](running-reports-cli.md)
