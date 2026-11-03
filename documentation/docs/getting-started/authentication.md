# Authentication

Report Server accepts browser sessions and personal **API Tokens**. Both resolve to the same user and ACL; handlers do not distinguish how you authenticated.

## Mechanisms

| Mechanism | Header / cookie | Typical use |
|-----------|-----------------|-------------|
| Session cookie `jrs_session` | Cookie set by login | Web UI |
| Bearer API Token | `Authorization: Bearer jrs_…` | CLI, SDKs, CI |
| `JRS_AUTH_DISABLED` | — | Local-only synthetic admin |
| OpenID Connect | Browser redirect `/auth/oidc/login` | Licensed Professional package only |
| SAML 2.0 | Browser redirect `/auth/saml/login` | Licensed Professional package only |
| Embed token `emb_` | Embed routes | Short-lived embed, not a personal access token |

### Resolution order

1. If `Authorization: Bearer` is present → validate the API Token only (no cookie fallback)
2. Else if `jrs_session` is present → validate the session
3. Else → anonymous

An invalid Bearer token does **not** fall through to the session cookie.

## Browser session

| Item | Value |
|------|--------|
| Cookie | `jrs_session` |
| Login | `POST /api/v1/auth/login` or `POST /login.php` |
| Logout | `POST /api/v1/auth/logout` or `/logout.php` |
| Storage | SHA-256 hex in `dashboard_sessions` |

```bash
curl -sS -c cookies.txt -X POST \
  -H "Content-Type: application/json" \
  -d '{"username":"alice","password":"…"}' \
  "$ACUGIS_REPORT_URL/api/v1/auth/login"

curl -sS -b cookies.txt \
  "$ACUGIS_REPORT_URL/api/v1/reports"
```

## API Tokens

Tokens use the `jrs_` prefix (32 random bytes, base64url secret; SHA-256 hash stored). The UI shows only a short `token_prefix`. Tokens currently inherit the full user ACL (scopes column reserved).

### Manage in the UI

**Profile → API Tokens** (`/profile.php`): create, rename, expire, rotate, revoke.

### Token REST API

| Method | Path | Purpose |
|--------|------|---------|
| `GET` | `/api/v1/auth/me` | Current user |
| `GET` | `/api/v1/auth/tokens` | List metadata (no secrets) |
| `POST` | `/api/v1/auth/tokens` | Create; plaintext returned **once** |
| `PUT` | `/api/v1/auth/tokens/{id}` | Rename / description / expiration |
| `DELETE` | `/api/v1/auth/tokens/{id}` | Soft revoke |
| `DELETE` | `/api/v1/auth/tokens/{id}?purge=1` | Hard delete |
| `POST` | `/api/v1/auth/tokens/{id}/rotate` | Revoke old, issue new |

Create body:

```json
{
  "name": "CI Server",
  "description": "optional",
  "expires_in": "never|30d|90d|1y|custom",
  "expires_at": "2027-01-01"
}
```

### Client usage

```bash
export ACUGIS_REPORT_URL=https://reports.example.com
export ACUGIS_REPORT_TOKEN=jrs_xxxxxxxxx

curl -sS -H "Authorization: Bearer $ACUGIS_REPORT_TOKEN" \
  "$ACUGIS_REPORT_URL/api/v1/auth/me"
```

CLI:

```bash
acugis-report login          # username/password → creates a PAT
acugis-report login --token  # paste an existing token
acugis-report whoami
acugis-report logout
```

Passwords are never stored by the CLI — only the Bearer token.

## Single sign-on

OIDC and SAML are browser login on the Professional package after a Report Server Professional license is valid. They are not in the Open Source package, and `ACUGIS_EDITION` does not turn them on. API clients keep using personal access tokens. This Open Source documentation does not include those setup pages.

## Security practices

- Prefer TLS in every environment that is not local-only
- Store tokens in a secret manager; never commit them
- Revoke unused or leaked tokens immediately
- Never log plaintext tokens or full hashes (prefix only)
- Keep `JRS_AUTH_DISABLED` off outside local development

## Related

- [API authentication](../developer/rest-api/authentication.md)
- [CLI authentication](../developer/cli/authentication.md)
- [Security administration](../administration/security.md)
