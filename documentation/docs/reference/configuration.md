# Configuration reference

Operator explanation: [Administration — Configuration](../administration/configuration.md).

Every variable, including defaults and which process reads it: [Environment variables](environment-variables.md).

## Groups

| Group | Start here |
|-------|------------|
| Application database | `JRS_DB_*`, `JRS_DATABASE_URL` |
| Jasper / Tomcat | `JRS_JASPER_INTERNAL_REPORT_URL` and the path variables. Packaged and Compose installs use port `8080`. Port `9099` is only the unset-variable fallback. |
| Go service | `JRS_LISTEN_ADDR` (`127.0.0.1:9088` unless Compose sets `0.0.0.0:9088`), `JRS_BASE_PATH` (`/`), `JRS_APP_VERSION` |
| Licensing | [Licensing](../administration/licensing.md). `ACUGIS_EDITION` does not grant features. |
| OpenID Connect and SAML | Available in Professional. Not part of this Open Source documentation. |
| Designer Web | [Designer Web](../administration/designer-web.md) |
| Embedding | [Embedding security](../developer/embedding/security.md) |
| SMTP and scheduler | Environment variable reference |
| Client (CLI/SDK) | `ACUGIS_REPORT_URL`, `ACUGIS_REPORT_TOKEN` — not read by `jrs-server` |

## Secrets never to log

- `JRS_DB_PASS` and datasource passwords
- Session cookies and personal access tokens
- `ACUGIS_EMBED_TOKEN_SECRET`
- OIDC client secrets and SAML private keys
- `JRS_DESIGNER_INTERNAL_TOKEN`
- License tokens and activation keys
- SMTP passwords
