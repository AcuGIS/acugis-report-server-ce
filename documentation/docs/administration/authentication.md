# Authentication (administration)

Report Server accepts browser sessions and personal **API Tokens**. Both resolve to the same user and ACL.

## Mechanisms

| Mechanism | Typical use |
|-----------|-------------|
| Session cookie `jrs_session` | Web UI |
| Bearer API Token (`jrs_…`) | CLI, SDKs, CI |
| `JRS_AUTH_DISABLED` | Local-only synthetic admin — never in production |
| OpenID Connect | Browser SSO on a licensed Professional package. Not part of this Open Source documentation. |
| SAML 2.0 | Browser SSO on a licensed Professional package. Not part of this Open Source documentation. |
| Embed tokens (`emb_`) | Short-lived tokens for `/embed/*`. Separate from personal access tokens. |

### Resolution order

1. If `Authorization: Bearer` is present → validate the API Token only (no cookie fallback)
2. Else if `jrs_session` is present → validate the session
3. Else → anonymous

An invalid Bearer token does **not** fall through to the session cookie.

## Operator checklist

- Prefer TLS everywhere outside local development
- Keep `JRS_AUTH_DISABLED` off in shared and production environments
- Rotate and revoke API Tokens routinely
- Store tokens in a secret manager; never commit them
- Never log plaintext tokens or full hashes (prefix only)

## Single sign-on

OIDC and SAML are compiled into the Professional package and stay off until you configure them. They require a valid Report Server Professional license. The Open Source package's **Administration → Authentication** page only documents local login.

Local username and password remains the default. Personal access tokens are unchanged by SSO. OpenID Connect and SAML setup is available in Professional and is not part of this Open Source documentation.

## Related

- [Licensing](licensing.md)
- [API Tokens](api-tokens.md)
- [Security](security.md)
- [Getting started — Authentication](../getting-started/authentication.md)
- [Developer authentication](../developer/authentication.md)
