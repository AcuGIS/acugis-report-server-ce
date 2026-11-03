# Security

## Authentication

- Prefer TLS everywhere outside local development
- Use API Tokens for automation; rotate and revoke routinely
- Keep `JRS_AUTH_DISABLED` off in shared and production environments
- Invalid Bearer tokens must not fall back to cookies (server enforces this)

## Authorization

- Prefer group-based folder ACLs
- Scope automation users with least privilege
- Treat service-admin operations (datasource mutations, context generation, admin HTML) as privileged

## Secrets handling

| Secret | Guidance |
|--------|----------|
| `JRS_DB_PASS` | Secret store / env injection. The package file is `/etc/acugis-report-server/env`. |
| Datasource passwords | Admin UI / form API only; never returned by list |
| API Tokens | Shown once; store in CI secrets |
| `ACUGIS_EMBED_TOKEN_SECRET` | Signs `emb_` tokens. The installer generates one on first boot. Required in production. |
| OIDC client secret | Administration → Authentication, or `ACUGIS_OIDC_CLIENT_SECRET`. Not returned after it is set from the environment. |
| SAML SP private key | Database or `ACUGIS_SAML_SP_KEY_FILE`. Never returned by the admin UI. |
| `JRS_DESIGNER_INTERNAL_TOKEN` | Shared secret for the Designer Web proxy. No production default. |
| License files | `license.lic` and `report-designer-web.lic` in the license state directory. The activation key is not stored. |
| SMTP passwords | Env only |
| GeoServer / QGIS / ArcGIS connection passwords | Properties files under Tomcat `conf/` with restricted permissions |

`ACUGIS_EDITION` is not a secret and not a license.

## Network

- The Go default listen address is `127.0.0.1:9088`. The Debian package writes that value. Docker Compose sets `0.0.0.0:9088` inside the container and publishes host port `9088`.
- The native installer downloads stock Tomcat 10 and does not change `server.xml` to bind the HTTP connector to loopback. Stock Tomcat listens on port `8080` on all interfaces. Restrict that port at the host firewall when the machine is reachable. Packaged Apache proxies public traffic to `127.0.0.1:9088`, not to `8080`.
- Docker Compose does not publish container port `8080` to the host. Inside the `report-server` container, Go uses `http://127.0.0.1:8080/report`. Other containers on the Compose network can reach Tomcat at `http://report-server:8080/report`. The scheduler uses that service URL. Port `8080` is not on the host, and it is reachable on the Compose network.
- Report Server does not terminate TLS. Use the reverse proxy for HTTPS and send `X-Forwarded-Proto` and `X-Forwarded-Host`. OIDC and SAML absolute URLs are built from those headers when they are present. The process uses the values as received and does not check that a trusted proxy set them.
- `/health`, `/ready`, `/api/v1/diagnostics`, `/api/v1/edition`, and `/dev/jasper-status` do not require a session. See [Diagnostics](diagnostics.md).
- Swagger is on the same public router. Protected operations still require a token or session.

## Headers for clients

```http
Authorization: Bearer jrs_…
```

Do not put long-lived tokens in query strings or frontend bundles.

## Related

- [Authentication](authentication.md)
- [Licensing](licensing.md)
- [Designer Web](designer-web.md)
- [API Tokens](api-tokens.md)
- [Permissions](permissions.md)
- [Users](users.md)
- [Groups](groups.md)
