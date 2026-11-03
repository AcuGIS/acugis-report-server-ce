# AcuGIS Report Server — Java SDK

Official Java client for [AcuGIS Report Server](https://acugis.com/).

| | |
|---|---|
| **Maven** | `com.acugis:acugis-report:0.1.0` |
| **Package** | `com.acugis.report` |
| **Java** | 17+ |
| **HTTP** | `java.net.http.HttpClient` |
| **JSON** | Jackson databind |

## Requirements

- **Java 17 or newer** (this machine may only have Java 11 — install a JDK 17+ toolchain to build)
- **Maven 3.9+**, or use the included Maven Wrapper (`mvnw` / `mvnw.cmd`)

```bash
# Windows
.\mvnw.cmd test

# Unix
./mvnw test
```

## Install

### Maven

```xml
<dependency>
  <groupId>com.acugis</groupId>
  <artifactId>acugis-report</artifactId>
  <version>0.1.0</version>
</dependency>
```

### Gradle

```kotlin
implementation("com.acugis:acugis-report:0.1.0")
```

Until published to Maven Central, install locally from this directory:

```bash
./mvnw install
```

## Quick start

```java
import com.acugis.report.ReportServerClient;
import java.nio.file.Path;
import java.time.Duration;
import java.util.Map;

ReportServerClient client = ReportServerClient.builder()
    .baseUrl("https://reports.example.com")
    .token(token)                          // Bearer PAT (jrs_…)
    .timeout(Duration.ofSeconds(60))
    .verifyTls(true)
    .header("X-Custom", "1")
    .build();

client.reports().list();
client.reports().runToFile("Invoice", Map.of("InvoiceID", 123), Path.of("invoice.pdf"));
```

### Environment variables

| Variable | Purpose |
|---|---|
| `ACUGIS_REPORT_URL` | Base URL (scheme + host + optional path prefix) |
| `ACUGIS_REPORT_TOKEN` | Personal access token |

```java
ReportServerClient client = ReportServerClient.fromEnvironment();
// or
ReportServerClient client = ReportServerClient.builder().fromEnvironment().build();
```

## Architecture

```
internal HttpTransport (java.net.http)
        ↓
ReportServerClient.builder()…build()
        ↓
client.reports() / folders() / resources() / …
```

| Service | Methods cover |
|---|---|
| `auth()` | login, logout, me, tokens CRUD/rotate |
| `reports()` | list, get, publish, update, delete, config, thumbnail, parameter lookup, run / runToFile |
| `folders()` | tree, browse, search, CRUD, move, duplicate, permissions, favorites, moveContent |
| `resources()` | list/upload/update/download (to `Path`), images, resource sync |
| `datasources()` | list, create (form upsert), delete, generateContext |
| `mapReports()` | features, extent, columns, run |
| `embeddedMaps()` | debug |
| `explorer()` | datasources, tables, metadata, query, export, views, dashboard, view permissions, map helpers, handoff, … |
| `health()` | health, ready, diagnostics, jasperStatus, doctor |

Reserved (throw `NotImplementedByServerException`):

- `users()`, `groups()`, `schedules()`, `jobs()`, `recipients()`

## Report execution

Renders via `GET|POST /reports/execute` with `_repName` and `_repFormat`.

```java
// Multi-value params are repeated in the query string
client.reports().run("Invoice", Map.of(
    "InvoiceID", 123,
    "Tags", List.of("a", "b")
), "pdf");

client.reports().runToFile("Invoice", Map.of("InvoiceID", 123), Path.of("invoice.pdf"));
```

Supported formats: `html`, `pdf`, `xlsx`, `xls`, `csv`, `docx`, `rtf`, `odt`, `ods`, `pptx`, `xml`.

## Async

```java
client.auth().meAsync().thenAccept(user -> System.out.println(user.getUsername()));
client.reports().listAsync();
client.reports().runAsync("Invoice", Map.of(), "pdf");
```

Cancel by calling `CompletableFuture.cancel(true)` (interrupts the HTTP exchange) or by configuring a short `timeout(...)`.

## Errors

| Exception | Typical status |
|---|---|
| `AuthenticationException` | 401 |
| `PermissionDeniedException` | 403 |
| `NotFoundException` | 404 |
| `ValidationException` | 400 / 422 |
| `ConflictException` | 409 |
| `ServerException` | 5xx |
| `NetworkException` | transport failure |
| `TimeoutException` | request timeout |
| `InvalidResponseException` | unparseable body |
| `NotImplementedByServerException` | reserved API |

## Options

| Builder method | Default | Notes |
|---|---|---|
| `timeout(Duration)` | 60s | Applied per request |
| `verifyTls(boolean)` | `true` | `false` disables cert validation (dev only) |
| `maxGetRetries(int)` | `0` | Retries GET on IO / 5xx only |
| `header(name, value)` | — | Sent on every request |
| `userAgent(String)` | `AcuGIS-Report-Java/0.1.0` | |

## Examples

- [`examples/console`](examples/console) — env vars, me, list, run to file
- [`examples/spring-boot`](examples/spring-boot) — bean + REST endpoints (`/api/me`, `/api/reports`, `/api/reports/{key}/pdf`)

```bash
# After ./mvnw install in sdk/java
cd examples/console
mvn -q exec:java -Dexec.args="Invoice out.pdf"

cd ../spring-boot
mvn spring-boot:run
```

## OpenAPI

This SDK is hand-written against [`../openapi.bundle.yaml`](../openapi.bundle.yaml). It does not invent endpoints.

## Maven Central

`pom.xml` includes name, description, URL, license, SCM, and developers placeholders for Central publishing. Add GPG signing / `nexus-staging-maven-plugin` (or the Sonatype Central Portal plugin) when ready to release.

## License

MIT — see [LICENSE](LICENSE).
