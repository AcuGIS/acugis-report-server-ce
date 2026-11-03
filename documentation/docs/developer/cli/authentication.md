# CLI authentication

## Interactive login

```bash
acugis-report login
```

Prompts for server URL, username, and password. The CLI authenticates, creates a personal API Token (`jrs_…`), and stores the token locally. **Passwords are never stored.**

## Token login

```bash
acugis-report login --token
```

Paste an existing API Token from Profile → API Tokens.

## Environment variables (CI)

```bash
export ACUGIS_REPORT_URL=https://reports.example.com
export ACUGIS_REPORT_TOKEN=jrs_xxxxxxxxx
```

## Who am I?

```bash
acugis-report whoami
acugis-report --output json whoami
```

Shows username, groups, authentication type, server, token name, and expiration.

## Logout

```bash
acugis-report logout
```

## Doctor

```bash
acugis-report doctor
```

Aggregates `/health`, `/ready`, `/api/v1/diagnostics`, and Jasper status.

## Related

- [Getting started authentication](../../getting-started/authentication.md)
- [Profiles](profiles.md)
