# API authentication

## Bearer API Token

```bash
curl -sS -H "Authorization: Bearer $ACUGIS_REPORT_TOKEN" \
  "$ACUGIS_REPORT_URL/api/v1/auth/me"
```

Create tokens via UI or:

```http
POST /api/v1/auth/tokens
```

Plaintext is returned **once**. Invalid/revoked/expired tokens share the same error path.

## Session cookie

```bash
curl -sS -c cookies.txt -X POST \
  -H "Content-Type: application/json" \
  -d '{"username":"alice","password":"…"}' \
  "$ACUGIS_REPORT_URL/api/v1/auth/login"

curl -sS -b cookies.txt \
  "$ACUGIS_REPORT_URL/api/v1/reports"
```

## Swagger Authorize

1. Open `/swagger`
2. Click **Authorize**
3. Paste a PAT under **BearerAuth**, or login via `POST /api/v1/auth/login` so **SessionCookie** is set
4. Use **Try it out**

## Resolution rules

- Bearer present → PAT only (no cookie fallback)
- Else cookie → session
- Else anonymous

See [Developer authentication](../authentication.md) for token lifecycle details. OpenID Connect and SAML, available in Professional, create that session cookie in the browser. They are not additional `Authorization` schemes, and this Open Source documentation does not include those setup pages.
