# Deployment

Report Server is typically deployed as:

1. **Go service** (`jrs-server`) — catalog UI, REST API, OpenAPI, Swagger, Data Explorer
2. **Tomcat + JasperReports WAR** — report fill / execute gateway
3. **PostgreSQL** — application database

## Topology

```text
Clients (browser / CLI / SDK)
        │
        ▼
   Go Report Server  ──►  Application PostgreSQL
        │
        ▼
   Tomcat Jasper (internal execute / image URLs)
        │
        ▼
   Report JDBC datasources (JNDI)
```

## Checklist

1. Apply database migrations
2. Configure `JRS_*` and Jasper internal URLs
3. Mount JRXML / resources / fonts / licenses
4. Configure JNDI datasources and map connection properties
5. Place TLS / reverse proxy in front of the Go listen address
6. Verify with [health checks](health-checks.md)

## Related

- [Installation](installation.md)
- [Docker](docker.md)
- [Reverse proxy](reverse-proxy.md)
- [Configuration](configuration.md)
