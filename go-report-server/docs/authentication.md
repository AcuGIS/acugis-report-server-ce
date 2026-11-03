# Authentication

## Mechanisms

| Mechanism | Supported | Notes |
|-----------|-----------|-------|
| Session cookie `jrs_session` | Yes | Browser login (unchanged) |
| Bearer personal access token | Yes | `Authorization: Bearer jrs_…` for SDK / CLI / CI |
| OIDC SSO | Professional only | Not registered by the Open Source server |
| SAML 2.0 SSO | Professional only | Not registered by the Open Source server |
| `JRS_AUTH_DISABLED` | Yes | Synthetic `local-dev` admin (local only) |

Resolution order in `auth.Service.AuthenticateRequest`:

1. If `Authorization: Bearer` is present → validate PAT only (no cookie fallback)
2. Else if `jrs_session` cookie is present → validate session
3. Else → anonymous

Both methods attach the same `auth.User` to the request context. ACL and handlers do not distinguish how the user authenticated.

## Browser session (unchanged)

| Item | Value |
|------|--------|
| Cookie | `jrs_session` |
| Login | `POST /api/v1/auth/login` or `POST /login.php` |
| Logout | `POST /api/v1/auth/logout` or `/logout.php` |
| Storage | SHA-256 hex in `dashboard_sessions` |

## Personal access tokens

Same architecture as AcuGIS Cloud (`agp_…`), with Report Server prefix `jrs_`.

| Item | Detail |
|------|--------|
| Prefix | `jrs_` |
| Entropy | 32 random bytes, base64url secret portion |
| Hash | SHA-256 hex of full plaintext (never store plaintext) |
| Display | First 12 characters (`token_prefix`) for UI only |
| Scopes | Column reserved; tokens currently inherit full user ACL |
| Table | `personal_access_tokens` |
| Last used | Throttled updates (~5 minutes) of `last_used_at` / `last_used_ip` |

### Token REST API

| Method | Path | Purpose |
|--------|------|---------|
| `GET` | `/api/v1/auth/me` | Current user (session or bearer) |
| `GET` | `/api/v1/auth/tokens` | List metadata (no secrets) |
| `POST` | `/api/v1/auth/tokens` | Create; returns plaintext **once** |
| `PUT` | `/api/v1/auth/tokens/{id}` | Rename / description / expiration |
| `DELETE` | `/api/v1/auth/tokens/{id}` | Soft revoke |
| `DELETE` | `/api/v1/auth/tokens/{id}?purge=1` | Hard delete |
| `POST` | `/api/v1/auth/tokens/{id}/rotate` | Revoke old, issue new (plaintext once) |

UI: `/profile.php` (tabs: Profile, Password, API Tokens).

### Create body

```json
{
  "name": "CI Server",
  "description": "optional",
  "expires_in": "never|30d|90d|1y|custom",
  "expires_at": "2027-01-01"
}
```

### Create response (only place plaintext is returned)

```json
{
  "token": "jrs_…",
  "id": 1,
  "created_at": "…",
  "name": "CI Server",
  "message": "This token will only be shown once."
}
```

## Client examples

### cURL (Bearer)

```bash
curl -sS -H "Authorization: Bearer jrs_xxxxx" \
  https://reports.example.com/go/api/v1/auth/me
```

### cURL (browser session)

```bash
curl -sS -c cookies.txt -X POST \
  -H "Content-Type: application/json" \
  -d '{"username":"alice","password":"…"}' \
  https://reports.example.com/go/api/v1/auth/login

curl -sS -b cookies.txt \
  https://reports.example.com/go/api/v1/reports
```

### Python SDK (planned)

```python
client = ReportServer(
    url="https://reports.example.com/go",
    token="jrs_xxxxxxxxx",
)
# No cookies / session handling required.
```

### CLI / environment (planned)

```bash
export ACUGIS_REPORT_TOKEN=jrs_xxxxxxxxx
# or: acugis-report auth login --token jrs_xxxxxxxxx
```

## Migration

Apply:

```text
database/migrations/007_personal_access_tokens.sql
```

Also included in `database/schema.sql` for fresh installs.

## Security

- Never log plaintext tokens or full hashes (prefix only).
- Never return plaintext after creation.
- Invalid / revoked / expired tokens all look the same (`ErrTokenInvalid`).
- Bearer present but invalid does **not** fall through to the session cookie.
- Prefer TLS; store PATs in a secret manager; revoke unused tokens.

## OpenAPI

Full contract: [`../openapi/`](../openapi/) — served at `/api/openapi.yaml`, `/api/openapi.json`, Swagger UI at `/swagger`.

See [`openapi.md`](./openapi.md). The historical auth-only stub [`openapi-auth.yaml`](./openapi-auth.yaml) is superseded.
