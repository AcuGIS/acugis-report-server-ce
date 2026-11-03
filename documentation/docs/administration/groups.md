# Groups

Group administration is available through the admin HTML UI for platform admins:

- `/admin/groups.php`

REST `/api/v1/groups` is **not** implemented yet. The CLI reserves a `groups` command group for when those APIs ship.

## Guidance

- Map groups to departments or roles that match folder ACLs and report `access_groups`
- Prefer groups over per-user ACL exceptions
- Keep group names stable — they appear in publish-time `access_groups` metadata

## Related

- [Users](users.md)
- [Permissions](permissions.md)
- [API compatibility](../developer/rest-api/compatibility.md)
