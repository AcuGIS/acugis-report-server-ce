# Parameters & execute (REST)

Report parameters are defined in JRXML and supplied at execute time by the viewer, CLI, SDKs, or REST.

## Execute gateway

```bash
curl -sS -H "Authorization: Bearer $ACUGIS_REPORT_TOKEN" \
  -o invoice.pdf \
  "$ACUGIS_REPORT_URL/reports/execute?_repName=Invoice&_repFormat=pdf&InvoiceID=123"
```

Multi-value parameters may be repeated as multiple query keys with the same name.

Query parameters include `_repName`, `_repFormat`, and report parameters. Supported formats: [Supported formats](../../reference/supported-formats.md).

Legacy aliases such as `/reports/execute_report.php` remain available but are deprecated in OpenAPI — prefer `/reports/execute` and `/api/v1/*`.

## Parameter lookup

Dynamic lists (for cascading inputs) use:

```http
GET  /api/v1/reports/{key}/parameters/{name}/lookup
POST /api/v1/reports/{key}/parameters/{name}/lookup
```

POST accepts a JSON body with a `params` object for dependent values.

```bash
curl -sS -H "Authorization: Bearer $ACUGIS_REPORT_TOKEN" \
  "$ACUGIS_REPORT_URL/api/v1/reports/Invoice/parameters/CustomerID/lookup"
```

## Config

`GET /api/v1/reports/{key}/config` returns viewer/config metadata used by the UI, including parameter descriptors when available.

## Map Report parameters

Map Report extent routes such as `/api/v1/map/{key}/extent` are not implemented by the Open Source server. They require Report Server Professional.

## Tips

- Keep JRXML parameter names stable for API consumers
- Prefer explicit types in JRXML so invalid values fail clearly at fill time
- Gateway errors from execute/image are often `text/plain`, not JSON

## Related

- [Binary responses](binary-responses.md)
- [REST reference](../../reference/rest-api.md)
- [User Guide — Parameters](../../user-guide/parameters.md)
