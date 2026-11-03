# Reverse proxy

Place TLS termination and routing in front of the Go listen address (`JRS_LISTEN_ADDR`).

## Base path

If the public URL is `https://host/go/`, set:

```bash
JRS_BASE_PATH=/go
```

Clients must use the same prefix:

```bash
export ACUGIS_REPORT_URL=https://host/go
```

Swagger and OpenAPI become `/go/swagger` and `/go/api/openapi.yaml`.

## Guidance

- Forward `Authorization` headers unchanged (required for Bearer API Tokens)
- Preserve cookies for browser sessions (`jrs_session` by default)
- Preserve the public host. The packaged Apache snippet sets `ProxyPreserveHost On` and `ProxyAddHeaders On`, so the Go service sees `X-Forwarded-Host` and the original `Host`. License activation compares the browser `Origin` to that public host.
- Forward `X-Forwarded-Proto` (`https` when TLS terminates at the proxy)
- Do not strip WebSocket upgrades if you add features that need them later
- Restrict direct access to Tomcat Jasper internal ports — only the Go/app tier should reach them
- Prefer HTTPS at the proxy; keep `JRS_AUTH_DISABLED` off behind any shared proxy

Apache snippet shipped in the package (`config/apache-acugis-report-server.conf`):

```apache
ProxyPreserveHost On
ProxyAddHeaders On
RequestHeader set X-Forwarded-Proto "https" env=HTTPS
ProxyPass / http://127.0.0.1:9088/
ProxyPassReverse / http://127.0.0.1:9088/
```

## QGIS Server (`/qgis/`)

When the Docker Compose `qgis` profile is enabled, Report Server reverse-proxies
`/qgis/` to the internal QGIS Server HTTP frontend. FastCGI is never exposed.

Apache example (TLS already terminated):

```apache
ProxyPass /qgis/ http://127.0.0.1:9088/qgis/
ProxyPassReverse /qgis/ http://127.0.0.1:9088/qgis/
```

Designer uses the public URL (`https://reports.example.com/qgis/`). Report Server
Docker fill should use the same connection **name** with an internal URL such as
`http://qgis-server`. Native OS installs do not install QGIS Server; point the
connection at an existing QGIS Server if you already run one.

`MAP` query values are rewritten to a filename under `JRS_QGIS_MAP_PREFIX`
(default `/qgis/projects`). Arbitrary filesystem paths are rejected.

## Related

- [Deployment](deployment.md)
- [Security](security.md)
- [Configuration](configuration.md)
