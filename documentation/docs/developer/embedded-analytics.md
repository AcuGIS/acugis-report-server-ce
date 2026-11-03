# Embedded analytics

Patterns for embedding Report Server output in another application.

For the official browser **JavaScript Embedding SDK** (`@acugis/report-embed`), see **[JavaScript Embedding](embedding/overview.md)**. That path **requires short-lived embed tokens** (`emb_…`) minted via `POST /api/v1/embed/token`.

Host-owned report parameters, such as a customer `account_id`, are **bound parameters** on that token. The host backend mints them with a PAT. The browser cannot override them. Interactive parameters such as `year` still go through the SDK. See [Bound Parameters](embedding/parameters.md#bound-parameters). The PAT must never be shipped to browser JavaScript.

## Pattern A — Server-side proxy (recommended for binaries)

Your backend holds `ACUGIS_REPORT_TOKEN`, runs `Reports.Run`, and streams PDF/HTML to the browser. Frontends never see the API Token.

## Pattern A — Server-side proxy (recommended)

Your backend holds `ACUGIS_REPORT_TOKEN`, runs `Reports.Run`, and streams PDF/HTML to the browser. Frontends never see the API Token.

See language tutorials:

- [Python FastAPI](tutorials/python-integration.md)
- [ASP.NET](tutorials/aspnet-integration.md)
- [Spring Boot](tutorials/spring-boot-integration.md)
- [Go HTTP](tutorials/go-http-integration.md)

## Pattern B — iframe to the viewer

If users already have Report Server sessions (SSO / shared login domain), embed:

```html
<iframe
  title="Invoice"
  src="https://reports.example.com/reports/Invoice"
  style="width:100%;height:80vh;border:0;">
</iframe>
```

Map Reports use the same viewer path with interactive map chrome when the report type is Map Report. That type requires a licensed Professional package. The packaged base path is `/`. Include `JRS_BASE_PATH` in the iframe URL when it is set.

## Pattern C — Direct execute URL (trusted networks only)

```text
{ACUGIS_REPORT_URL}/reports/execute?_repName=Invoice&_repFormat=pdf&InvoiceID=123
```

Requires a session cookie or a gateway that injects `Authorization: Bearer …`. Do not put long-lived tokens in browser-visible URLs.

## Embedded Maps inside JRXML

Maps authored as **Embedded Map** elements render during fill — no separate iframe. Debug with:

```bash
acugis-report embedded-map debug Invoice --compile --output json
```

## Checklist

1. Use the [compatibility matrix](../reference/compatibility-matrix.md) when the report needs Map Reports or other Professional features
2. Publish JRXML with stable parameter names
3. Issue a least-privilege API Token
4. Proxy binaries from your app tier
5. Set `Content-Type` and disposition headers appropriately
