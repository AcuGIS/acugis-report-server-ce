# Examples

Install the SDK into the local Maven repository first:

```bash
cd ..
./mvnw install
```

## Console

```bash
cd console
export ACUGIS_REPORT_URL=https://reports.example.com
export ACUGIS_REPORT_TOKEN=jrs_...
mvn -q exec:java -Dexec.args="Invoice out.pdf"
```

## Spring Boot

```bash
cd spring-boot
export ACUGIS_REPORT_URL=https://reports.example.com
export ACUGIS_REPORT_TOKEN=jrs_...
mvn spring-boot:run
```

Endpoints:

- `GET /api/me`
- `GET /api/reports`
- `GET /api/reports/{key}/pdf?param=value`
