# Java SDK

The Java client uses lowercase service accessors (`reports()`, `auth()`) with the same operation names as other SDKs. Repository path: [`sdk/java/`](https://github.com/AcuGIS/acugis-report-server-ce/tree/main/sdk/java).

## Installation

Build/install from `sdk/java` with Maven (`./mvnw install`). See the SDK README for artifact coordinates when published.

## Authentication

Pass a Bearer API Token into `ReportServerClient.builder().token(...)`. Token lifecycle helpers live under `client.auth()`. See [Developer authentication](../authentication.md).

## Creating a client

```java
import java.nio.file.Files;
import java.nio.file.Path;
import java.util.Map;

var client = ReportServerClient.builder()
    .url(System.getenv("ACUGIS_REPORT_URL"))
    .token(System.getenv("ACUGIS_REPORT_TOKEN"))
    .build();

var me = client.auth().me();
System.out.println(me.username() + " " + me.groups());
```

### Service namespaces

| Accessor | Examples |
|----------|----------|
| `client.auth()` | `me()`, token APIs |
| `client.reports()` | `list()`, `get(key)`, `run(...)`, `publish(...)` |
| `client.folders()` | `tree()`, `browse(id)` |
| `client.resources()` | `list()`, `upload(...)`, `download(uuid)` |
| `client.datasources()` | `list()`, `create(...)`, `delete(id)`, `context()` |
| `client.map()` / `client.mapReports()` | `features`, `extent`, `run` |
| `client.explorer()` | `query`, `statistics`, `export` |

## Listing reports

```java
var reports = client.reports().list();
reports.items().forEach(r ->
    System.out.println(r.key() + " " + r.name()));
```

## Running reports

```java
var result = client.reports().run(
    "Invoice",
    "pdf",
    Map.of("InvoiceID", 123));

Files.write(Path.of("invoice.pdf"), result.content());
System.out.println(result.contentType());
```

## Uploading reports

Use `client.reports().publish(...)` for multipart JRXML create. Update via the Reports service update operation.

## Downloading reports

`run(...)` returns binary `content()`. Resource downloads use `client.resources().download(uuid)`. Save bytes to disk — see [Binary responses](../rest-api/binary-responses.md).

## Error handling

Map `ReportServerException` subclasses (authentication, validation, not found, permissions, server, network) in application code.

## Environment variables

```bash
export ACUGIS_REPORT_URL=https://reports.example.com
export ACUGIS_REPORT_TOKEN=jrs_xxxxxxxxx
```

## Common patterns

- Construct the client from `ACUGIS_REPORT_URL` and `ACUGIS_REPORT_TOKEN`
- List reports, then `run` with a format and parameter map
- Write binary `result.content()` to disk
- Register the client as a Spring bean for web apps

## OpenAPI generation

```bash
curl -o openapi.yaml "$ACUGIS_REPORT_URL/api/openapi.yaml"
openapi-generator-cli generate -i openapi.yaml -g java -o ./sdk-java
```

## Related

- [Spring Boot tutorial](../tutorials/spring-boot-integration.md)
- [Binary responses](../rest-api/binary-responses.md)
