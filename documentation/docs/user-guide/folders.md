# Folders

**Folders** organize reports, saved views, dashboards, and resources in a tree. The catalog browse UI and automation APIs share the same hierarchy.

## What folders are for

- Group related reports, Data Explorer saved views, and dashboards (by department, product, client, or project)
- Apply permissions once at a folder and inherit downward
- Keep resources (images and files) near the content that uses them
- Browse and search within a structured catalog instead of a flat list

## Root and browse

Folder `0` is the root browse target (Home). From the catalog you navigate the tree, open a folder’s contents, and move items between folders when you have rights to do so.

Unfiled reports and your unfiled saved views appear under Home until you move them into a folder.

## Creating and maintaining folders

Users and admins with appropriate rights can:

- Create folders (name, optional description, icon, sort order)
- Rename / update metadata
- Move folders under a new parent
- Duplicate folders
- Delete folders (may require confirmation when content exists)
- Move content between folders

From **New** in the catalog you can create a Report, Saved View, Dashboard, or Resource into the current folder.

Folders may inherit ACL from their parent (`acl_inherit`) or define explicit permissions.

## Content types

| Catalog type | Folder `content_type` | Key |
|--------------|----------------------|-----|
| Report | `jasper_report` | report key |
| Saved View / Dashboard | `explorer_view` | `owner/viewKey` |
| Resource | `content_resource` | resource UUID |

Dashboards are saved views with the dashboard flag; they share the same folder membership type.

## Searching

Catalog search finds content across the folder tree. Results are capped (around 50 matches). See [Searching](searching.md).

## Permissions

Folder permissions control who can browse, manage, and act on content under that node. Prefer group-based ACLs over many per-user exceptions. Details: [Permissions](permissions.md).

## Related

- [Reports](reports.md)
- Saved views and dashboards are available in Professional and are not part of this Open Source guide.
- [Resources](resources.md)
- [Permissions](permissions.md)
- [Searching](searching.md)

---

## For Developers

| Integration path | Go here |
|------------------|---------|
| REST folders API | [REST API](../developer/rest-api/overview.md) · [REST reference](../reference/rest-api.md) |
| CLI | [CLI folders](../developer/cli/folders.md) |
| SDK | [SDK overview](../developer/sdk/overview.md) |
