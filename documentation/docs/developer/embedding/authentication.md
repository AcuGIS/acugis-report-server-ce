# Embedding authentication

## Required: embed tokens

Every embed requires an `emb_` token from `POST /api/v1/embed/token`.

| Credential | Where used |
|------------|------------|
| Session / PAT (`jrs_`) | **Backend only** — mint embed tokens |
| Embed token (`emb_`) | **Browser SDK** and `/embed/*` routes |

### Mint

```http
POST /api/v1/embed/token
Authorization: Bearer jrs_…
Content-Type: application/json

{
  "content_type": "report",
  "content_id": "Invoice",
  "ttl_seconds": 900,
  "origin": "https://app.example.com",
  "parameters": {
    "account_id": "smith-residential"
  }
}
```

`parameters` are optional **bound parameters**. When present, Report Server treats those values as authoritative for later requests that authenticate with this embed token. Omit the field for the previous behavior. The PAT used to mint the token stays on the host backend. See [Bound Parameters](parameters.md#bound-parameters).

`content_type` is one of: `report`, `dashboard`, `view`, `map-report`.

The server checks ACL for the caller before issuing the token. Tokens are short-lived (default 15m) and scoped to that content.

### Use in the SDK

```javascript
AcuGIS.embedReport({
  server: "https://reports.example.com",
  target: "#report",
  reportId: "Invoice",
  auth: { type: "token", token: embToken },
});
```

Convenience form: `token: embToken`.

Session-only auth and raw PATs are rejected by the SDK with `AUTH_REQUIRED`.
