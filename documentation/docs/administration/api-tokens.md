# API Tokens

Personal **API Tokens** authenticate automation (CLI, SDKs, CI) as a specific user. Tokens inherit that user’s ACL.

## Lifecycle

| Action | Where |
|--------|-------|
| Create / rename / expire / rotate / revoke | **Profile → API Tokens** (`/profile.php`) |
| Same operations via REST | `/api/v1/auth/tokens` (see Developer Guide) |

The plaintext `jrs_…` value is shown **once** at create/rotate. The UI otherwise shows only a short `token_prefix`.

## Token properties

- Prefix `jrs_` (32 random bytes, base64url secret; SHA-256 hash stored)
- Optional expiration (`never`, `30d`, `90d`, `1y`, or custom date)
- Currently inherit the full user ACL (scopes column reserved)

## Operational guidance

- Create tokens under least-privilege users
- Prefer short expirations for CI where practical; rotate on schedule
- Revoke when staff leave or automation is retired
- Never put long-lived tokens in query strings or frontend bundles

## Related

- [Authentication](authentication.md)
- [Security](security.md)
- [Developer authentication](../developer/authentication.md)
