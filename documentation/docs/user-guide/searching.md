# Searching

Use catalog **search** to find reports and related content across the folder tree without browsing every node.

## How search works

Search is available from folder browse / catalog surfaces. Results are capped (API inventory notes a hard cap around 50 results), so refine your query if you do not see an expected item.

## Tips

- Prefer stable report keys and clear display names so search is predictable
- Organize with [folders](folders.md) when a department or project grows large
- Confirm you have permission to see the item — ACL still applies to search results

## Related

- [Folders](folders.md)
- [Reports](reports.md)

---

## For Developers

Folder search endpoint: `GET /api/v1/folders/search?q=` — see [REST reference](../reference/rest-api.md) and [CLI folders](../developer/cli/folders.md).
