# Building a Spring Boot integration

Serve Invoice PDFs from a Spring Boot controller using `client.reports().run`.

## Configuration

```properties
# application.properties (values from env in real deployments)
acugis.report.url=${ACUGIS_REPORT_URL}
acugis.report.token=${ACUGIS_REPORT_TOKEN}
```

## Controller

```java
@RestController
@RequiredArgsConstructor
public class InvoiceController {
    private final ReportServerClient client;

    @GetMapping(value = "/invoices/{invoiceId}.pdf", produces = MediaType.APPLICATION_PDF_VALUE)
    public ResponseEntity<byte[]> invoice(@PathVariable int invoiceId) {
        var result = client.reports().run(
            "Invoice",
            "pdf",
            Map.of("InvoiceID", invoiceId));

        return ResponseEntity.ok()
            .header(HttpHeaders.CONTENT_DISPOSITION,
                "inline; filename=\"invoice-" + invoiceId + ".pdf\"")
            .contentType(MediaType.APPLICATION_PDF)
            .body(result.content());
    }
}
```

## Bean

```java
@Bean
ReportServerClient reportServerClient(
    @Value("${acugis.report.url}") String url,
    @Value("${acugis.report.token}") String token) {
    return ReportServerClient.builder().url(url).token(token).build();
}
```

## Notes

- Externalize the API Token via your platform secret mechanism
- Map SDK exceptions to `502 Bad Gateway` or problem details for upstream failures
- For HTML embeds, see [Embedded reporting](../embedded-analytics.md)
