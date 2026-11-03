# Error codes

Error shapes vary slightly by area. Clients should handle both JSON problem bodies and plain-text gateway failures.

## Common JSON shapes

```json
{ "error": "message" }
```

```json
{
  "error": "validation failed",
  "errors": ["name is required"]
}
```

Success wrappers may use `{"ok":true}` or `{"success":true,"message":"…"}` depending on the endpoint family.

## HTTP status guidance

| Status | Typical meaning |
|--------|-----------------|
| 400 | Validation / bad input |
| 401 | Missing or invalid auth |
| 403 | Authenticated but not permitted |
| 404 | Unknown report key / resource |
| 422 | Explorer engine unsupported for interactive use |
| 503 | Dependency unavailable (for example datasource store) |

## Auth token errors

Invalid, revoked, and expired API Tokens are intentionally indistinguishable to clients.

## Execute / image gateway

`/reports/execute` and image proxy failures are often `text/plain`, not JSON. SDKs and the CLI treat non-success statuses as errors and preserve the body for diagnostics (`--debug` in the CLI).

## Soft-failure patterns

A few operational endpoints historically return HTTP 200 with `ok:false` / `success:false` (for example some sync or schedule run-now paths). Prefer checking the JSON body as well as the status code for those operations — see OpenAPI examples.

## SDK / CLI mapping

The Python SDK maps statuses to typed errors; the CLI maps them to exit codes (auth=2, validation=3, not found=4, permissions=5, server=6, network=7).
