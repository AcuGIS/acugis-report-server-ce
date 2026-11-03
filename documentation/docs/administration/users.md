# Users

User administration is available through the admin HTML UI for platform admins:

- `/admin/users.php`

REST `/api/v1/users` is **not** implemented yet. The CLI reserves a `users` command group for when those APIs ship.

## Guidance

- Create dedicated automation users for CI tokens instead of sharing interactive admin accounts
- Prefer group membership for catalog access rather than many per-user folder ACL exceptions
- Disable or remove accounts promptly when staff leave; revoke their API Tokens

## Related

- [Groups](groups.md)
- [Permissions](permissions.md)
- [API Tokens](api-tokens.md)
- [API compatibility](../developer/rest-api/compatibility.md)
