# Troubleshooting

## Cannot authenticate

| Symptom | Check |
|---------|-------|
| 401 on all API calls | Token prefix `jrs_`, not revoked/expired; `Authorization` header spelling |
| Browser logged in but CLI fails | CLI needs Bearer token — session cookie is not used unless you implement it |
| Bearer ignored for cookie user | Invalid Bearer does **not** fall back to cookie |
| Local smoke tests | `JRS_AUTH_DISABLED` only for local; never in production |

## Report execute fails

| Symptom | Check |
|---------|-------|
| Empty/error PDF | Datasource name, JNDI/`context.xml`, JRXML query |
| `text/plain` error body | Read the gateway message; often Jasper/internal URL misconfig |
| Timeout | `JRS_JASPER_TIMEOUT`, Jasper URL reachability from Go |
| Missing font/image | Fonts dir, resource UUIDs, reports directory mounts |

## Embedded Map / Map Report issues

| Symptom | Check |
|---------|-------|
| Blank map image | Embedded Map debug; spatial datasource; renderer jars in WAR |
| **Token Required** | ArcGIS connection name/credentials + properties mounted into Tomcat `conf/` + restart |
| Map Report publish error | Spatial datasource requirement (commonly PostGIS) |
| Features/extent empty | ACL, key, SRID/geometry parameters |

## Data Explorer

| Symptom | Check |
|---------|-------|
| HTTP 422 | Engine not PostgreSQL/MySQL for interactive Explorer |
| Spatial analysis unsupported | Needs PostGIS today |
| Missing tables | Datasource enabled; metadata permissions |

## Datasource REST surprises

| Symptom | Expected |
|---------|----------|
| List has no hosts/passwords | Names only by design |
| No GET-by-id | Not implemented |
| GeoServer/ArcGIS not in REST | HTML admin only |

## OpenAPI / Swagger not found

Confirm base path:

```text
{origin}{JRS_BASE_PATH}/swagger
{origin}{JRS_BASE_PATH}/api/openapi.yaml
```

## Still stuck

1. `acugis-report doctor`
2. `/api/v1/diagnostics`
3. Server logs (redact secrets)
4. Reproduce with curl against `/api/v1/auth/me` and `/reports/execute`
