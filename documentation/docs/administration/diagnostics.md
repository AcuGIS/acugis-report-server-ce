# Diagnostics

These endpoints are registered on the public router. They do not require a session or a personal access token. The packaged Apache proxy forwards `/` to `127.0.0.1:9088`, so a public virtual host exposes them.

Do not put secrets in tickets that quote a diagnostics body. The payload is redacted, and it still describes how the server is configured.

## Endpoints

| Path | Auth | Response |
|------|------|----------|
| `GET /health` | none | `200` and `{"status":"ok","service":"acugis-report-server-go"}` |
| `GET /ready` | none | `200` with `"status":"ready"` when PostgreSQL answers and required filesystem paths exist. `503` with `"status":"not_ready"` when either of those fails. The body also includes a `jasper` check. That Jasper result does not by itself change the HTTP status. |
| `GET /api/v1/diagnostics` | none | `200` JSON. See below. |
| `GET /api/v1/edition` | none | Product name, `JRS_APP_VERSION`, effective edition, display name, capability flags, and `report_designer` (Designer Web is configured, not whether it is licensed). |
| `GET /dev/jasper-status` | none | Jasper gateway status, including the configured report and image URLs, the last report name, the last error summary, and whether authentication is disabled. Registered in the same router as the other probes, including packaged installs. |

`/health` does not check PostgreSQL or Jasper. `/ready` checks PostgreSQL and required paths, and it reports Jasper without using that report as the status code.

```bash
curl -sS "$ACUGIS_REPORT_URL/health"
curl -sS "$ACUGIS_REPORT_URL/ready"
curl -sS "$ACUGIS_REPORT_URL/api/v1/diagnostics"
curl -sS "$ACUGIS_REPORT_URL/api/v1/edition"
```

`ACUGIS_REPORT_URL` is the client base URL. It is not a server setting. With the packaged default base path, that URL has no `/go` prefix.

The Compose healthcheck calls `/health` inside the container and checks that Tomcat port `8080` accepts a connection. It does not call `/ready`.

## Diagnostics body

`/api/v1/diagnostics` includes:

- service and product name, Go version, app version
- effective edition, display name, and capability flags
- environment, base path, and listen address
- whether PostgreSQL responded, whether required filesystem paths exist, and whether expected tables are present
- whether the Jasper endpoint is configured and reachable (`true`, `false`, or `unknown`)
- whether the auth and ACL repositories are configured, and whether `JRS_AUTH_DISABLED` is on
- a `config` object from the redacted summary

The redacted summary includes hosts, ports, database name and user, Jasper URLs, filesystem paths, audit flags, and whether Designer Web is configured. It does not include the database password, the DSN, the embed-token secret, the Designer internal token, OIDC or SAML secrets, or the license token. A regression test rejects a body that contains the database password.

Table presence and path details are still useful to anyone who can reach the URL. Keep the Go listener on loopback and publish Report Server through the reverse proxy you intend to operate.

## CLI

```bash
acugis-report doctor
```

The CLI calls the health, readiness, diagnostics, and Jasper status URLs for the configured `ACUGIS_REPORT_URL`.

## Related

- [Health checks](health-checks.md)
- [Logging](logging.md)
- [Licensing](licensing.md)
- [Security](security.md)
