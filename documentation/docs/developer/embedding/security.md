# Embedding security

1. **Embed tokens required** — never pass `jrs_` PATs to the browser SDK.
2. **ACL still applies** — token minting and `/embed/*` both enforce Report Server permissions.
3. **Allowed origins** — Administration → Embedded Analytics is the normal place to name the applications that may embed this server. `ACUGIS_EMBED_ALLOWED_ORIGINS` (alias `JRS_EMBED_ALLOWED_ORIGINS`) remains supported. The effective allow-list is the union of that environment value and the origins saved in Admin. Only those origins may frame `/embed/*` (`Content-Security-Policy: frame-ancestors`). An empty effective list stays `frame-ancestors 'none'`.
4. **Normal UI stays protected** — non-embed routes keep `X-Frame-Options: SAMEORIGIN`.
5. **postMessage** — host and viewer validate origin, source window, protocol `source: "acugis-report"`, and `version: 1`. Production code never uses `postMessage(..., "*")`.
6. **No secrets in URLs long-term** — `et=` bootstraps the embed; the viewer strips it after setting an HttpOnly embed cookie.
7. **Bound parameters** — host-owned values such as `account_id` belong in `POST /api/v1/embed/token` `parameters`, minted by the host backend with a PAT. Report Server overwrites the browser value on execute and on report data endpoints. Hiding the control is not the security boundary. See [Parameters](parameters.md#bound-parameters).

## Server configuration

Native installs (Debian package and `install.sh`) write the signing secret to `/etc/acugis-report-server/env`:

- A fresh installation generates a 32-byte hex secret with `openssl rand -hex 32` (or Python `secrets.token_hex`) on that machine. It is not hard-coded and is not the same on every installation.
- The env file is mode `640`, owner `root`, group `www-data`, which is the same file systemd reads as `EnvironmentFile`. The Go server and PHP helpers also fall back to that path. The secret is not written to `initial-credentials`, is not copied to `/opt/acugis-report-server/.env`, and is not printed by the installer. An older copy in that `.env` is removed on upgrade.
- Package upgrades do not replace the env file and do not rotate a secret that is already set.
- If `ACUGIS_EMBED_TOKEN_SECRET` or `JRS_EMBED_TOKEN_SECRET` is already in the env file, that value is kept. If neither key is set, an administrator value supplied in the environment at configure time is written. Otherwise a new secret is generated.
- Allowed origins are managed in Administration → Embedded Analytics. Origins added there are stored in PostgreSQL (`embed_allowed_origin`), take effect without a restart, and are kept across package upgrades with the rest of the application database.
- `ACUGIS_EMBED_ALLOWED_ORIGINS` remains supported and is merged with the Admin list. Its parsing is unchanged, including a configured `*`. Environment origins are read-only in the Admin UI (shown as system configured). The variable is not generated for you.

```bash
ACUGIS_EMBED_ALLOWED_ORIGINS=https://app.example.com,https://portal.example.com
ACUGIS_EMBED_TOKEN_SECRET=long-random-secret
ACUGIS_EMBED_TOKEN_TTL=15m
```

The Admin page accepts exact origins only (`https://app.example.com`, `http://localhost:3000`). It rejects paths, query strings, fragments, credentials, and wildcards. Those Admin rules do not change how an existing environment value is interpreted.

Docker Compose still expects `ACUGIS_EMBED_TOKEN_SECRET` in the compose `.env` file. Development and test processes use a fixed fallback secret when the variable is unset. Production does not.

Development defaults add common localhost origins when the environment allow-list is empty. Those defaults are part of the read-only system list.
