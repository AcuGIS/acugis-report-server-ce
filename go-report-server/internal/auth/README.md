# Auth package

Validates PHP-compatible `jrs_session` cookies and AcuGIS Cloud-style personal
access tokens (`Authorization: Bearer jrs_…`) against PostgreSQL.

## Session contract (`lib/auth.php`)

| Item | Value |
|------|--------|
| Cookie name | `jrs_session` |
| Cookie contents | Raw random token (base64url-ish from `random_bytes(32)`) |
| DB column | `dashboard_sessions.token_hash` = SHA-256(raw), **lowercase hex** |
| Cookie path | `/` |
| Flags | `HttpOnly`, `SameSite=Lax`, `Secure` when HTTPS / `X-Forwarded-Proto=https` |
| TTL | 604800 seconds (7 days) at creation; Go does **not** refresh expiry |
| Multiple sessions | Supported (no unique constraint on `username`) |
| Revoked sessions | Removed on logout (`DELETE` by token hash); no separate revoked flag |

Lookup conditions (exact PHP predicates):

```sql
WHERE s.token_hash = $1
  AND s.expires_at > NOW()
  AND COALESCE(u.enabled, TRUE) = TRUE
```

Join: `dashboard_sessions` → `dashboard_users` on `username`. Groups come from
`auth_group_members` / `auth_groups` (with legacy `dashboard_users.groups` JSON
fallback in the SELECT).

## Personal access tokens

Same model as AcuGIS Cloud (`shared/auth` PATs / `agp_…`), with Report Server
prefix `jrs_`. See `docs/authentication.md`.

| Item | Value |
|------|--------|
| Header | `Authorization: Bearer jrs_<secret>` |
| Table | `personal_access_tokens` |
| Hash | SHA-256 hex of full plaintext |
| UI | `/profile.php` |
| REST | `/api/v1/auth/tokens`, `/api/v1/auth/me` |

Resolution order: Bearer (if present) → session cookie → anonymous. Invalid
Bearer does **not** fall through to the cookie.

## Login / logout

| Operation | Behavior |
|-----------|----------|
| Login | bcrypt verify `dashboard_users.password_hash`; insert session; set cookie |
| Logout | delete session by hash (best-effort); clear cookie; redirect to `/login.php` |
| UI routes | `GET/POST /login.php`, `GET/POST /logout.php` |
| API routes | `POST /api/v1/auth/login`, `POST /api/v1/auth/logout` |

Browser login is unchanged. SDKs should use Bearer tokens and never require login.

## `JRS_AUTH_DISABLED`

When truthy (`1` / `true` / `yes` / `on`), PHP installs a synthetic administrator:

- `id=0`, `username=local-dev`, `is_admin=true`, `groups=[]`

**Security:** Auth-disabled mode bypasses all session and ACL checks. Use only
for local smoke tests. Never enable in production.

## Middleware

| Helper | Behavior |
|--------|----------|
| `OptionalAuthentication` | Attach user when valid (Bearer or cookie); continue anonymously when missing/invalid. Return **503** if a credential was supplied and the database fails. |
| `RequireAuthentication` | **401** JSON when unauthenticated. Never redirects to a login page. |

## Logging policy

Allowed: request id, user id, username, token **prefix**, allow/deny outcomes.  
Never log: raw cookie, plaintext PAT, full token hash, password hashes,
`Authorization` credentials (use `RedactAuthorization`).
