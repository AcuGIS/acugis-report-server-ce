# Permissions (administration)

Access control combines user/group membership, folder ACLs, and report `access_groups`. API Tokens and browser sessions use the same `auth.User` and ACL evaluation.

## How authorization works

1. Authenticate (session cookie or Bearer API Token)
2. Resolve the user and group memberships
3. Apply folder permissions (inherit or explicit)
4. Apply report visibility (`access_groups` and catalog rules)
5. Gate service-admin operations (datasource mutations, some admin HTML pages)

## Folder permissions

Folders may inherit ACL from their parent (`acl_inherit` on create/update) or define explicit permissions via the folder permissions API / UI.

Prefer group-based folder ACLs over per-user exceptions.

## Report access groups

Set at publish or update via multipart field `access_groups`. There is currently **no** dedicated `GET/PUT /api/v1/reports/{key}/permissions` endpoint — use publish/update to change group access.

## Service-admin operations

Examples that require elevated rights:

- Create / delete datasources
- Generate Jasper context
- Certain administration pages (schedules, recipients, GeoServer/ArcGIS connections)

## Practical guidance

- Prefer group-based folder ACLs
- Issue least-privilege API Tokens by creating them under least-privilege users
- Revoke tokens when staff leave or automation is retired

## Related

- [Users](users.md)
- [Groups](groups.md)
- [Security](security.md)
- [User Guide — Permissions](../user-guide/permissions.md)
