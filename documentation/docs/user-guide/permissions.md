# Permissions (user perspective)

Access control decides which reports and folders you can see and run. API Tokens and browser sessions use the **same** user identity and ACL evaluation.

## How authorization works

1. Authenticate (session cookie or Bearer API Token)
2. Resolve your user and group memberships
3. Apply folder permissions (inherit or explicit)
4. Apply report visibility (`access_groups` and catalog rules)
5. Gate service-admin operations (datasource mutations, some admin pages)

## Folders

Folders may inherit ACL from their parent or define explicit permissions. Prefer **group-based** folder ACLs over many per-user exceptions.

## Reports

Report visibility uses `access_groups` set when the report is published or updated. There is currently no separate “report permissions” screen that bypasses publish/update — access groups are part of the report’s catalog metadata.

## What you can do vs admin tasks

| You typically can | Usually admin / elevated |
|-------------------|--------------------------|
| Browse permitted folders | Manage users and groups |
| Run permitted reports | Create/delete datasources |
| Use resources you can access | GeoServer / ArcGIS connection admin |
| Create API Tokens for yourself | System-wide security settings |

User and group administration is available through admin HTML pages for platform admins. See [Administration — Users](../administration/users.md) and [Groups](../administration/groups.md).

## Practical guidance

- Ask for group membership rather than one-off folder grants when possible
- Issue least-privilege API Tokens under least-privilege users (tokens inherit the user’s ACL)
- Revoke tokens when staff leave or automation is retired

## Related

- [Folders](folders.md)
- [Reports](reports.md)
- [Authentication (getting started)](../getting-started/authentication.md)
- [Security (administration)](../administration/security.md)
- [Permissions (administration)](../administration/permissions.md)

---

## For Developers

| Integration path | Go here |
|------------------|---------|
| Auth model | [Developer authentication](../developer/authentication.md) |
| Folder permissions API | [REST reference](../reference/rest-api.md) |
| Admin security | [Security](../administration/security.md) |
