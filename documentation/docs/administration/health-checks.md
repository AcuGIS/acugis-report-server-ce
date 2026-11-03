# Health checks

## Probes

These routes do not require a session or an API token. See [Diagnostics](diagnostics.md) for the fields they return and for the authentication note.

| Path | Meaning |
|------|---------|
| `GET /health` | Process is answering. Does not check PostgreSQL or Jasper. |
| `GET /ready` | PostgreSQL ping and required filesystem paths. `503` when either fails. The body includes a Jasper check that does not by itself change the HTTP status. |
| `GET /api/v1/diagnostics` | Redacted configuration and dependency summary. |
| `GET /api/v1/edition` | Effective edition and capability flags. |
| `GET /dev/jasper-status` | Unauthenticated Jasper gateway status. The body includes the configured report and image URLs, the last report name, the last error summary, and whether authentication is disabled. Available on packaged servers, not only in a development build. |

```bash
curl -sS "$ACUGIS_REPORT_URL/health"
curl -sS "$ACUGIS_REPORT_URL/ready"
curl -sS "$ACUGIS_REPORT_URL/api/v1/diagnostics"
```

The Docker Compose healthcheck uses `/health` plus a TCP check of Tomcat port `8080` inside the container. It does not use `/ready`.

The scheduler container, when `JRS_SCHEDULE_RUNTIME=dispatcher`, serves its own `GET /health` on `JRS_SCHEDULER_LISTEN_ADDR` (default `:9089`). That is the dispatcher process, not `jrs-server`.

## CLI

```bash
acugis-report doctor
```

## Related

- [Diagnostics](diagnostics.md)
- [Logging](logging.md)
- [Troubleshooting](troubleshooting.md)
