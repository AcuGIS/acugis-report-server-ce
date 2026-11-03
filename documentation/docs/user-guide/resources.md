# Resources

**Resources** are binary assets (images and other files) stored by Report Server and referenced from reports or admin workflows.

## What resources are

| Aspect | Description |
|--------|-------------|
| Content | Uploaded files (logos, images, supporting binaries) |
| Identity | UUID used by the catalog and automation |
| Placement | Optionally associated with a folder |
| Use | Referenced by reports, key-based datasource auth, and admin tooling |

Prefer resource UUIDs in documentation and automation instead of brittle absolute filesystem paths.

## Working with resources in the UI

Upload, replace, download, and delete resources through the catalog/admin surfaces that manage images and files. Thumbnails are available for image resources.

## Resource sync

Operators can sync Jasper resource paths when filesystem resource paths and the catalog need reconciliation. This is an operational task — see [Diagnostics](../administration/diagnostics.md) and OpenAPI operationIds `getResourceSyncHint` / `runResourceSync`.

## Tips

- Keep logos and shared images as resources rather than embedding only in JRXML where reuse matters
- Image downloads are binary — save to a file
- Key-based datasource auth can reference `public_key_resource_uuid` / `private_key_resource_uuid`

## Related

- [Reports](reports.md)
- [Folders](folders.md)
- [Datasources](datasources.md)

---

## For Developers

| Integration path | Go here |
|------------------|---------|
| REST resources API | [REST API](../developer/rest-api/overview.md) · [REST reference](../reference/rest-api.md) |
| CLI | [CLI resources](../developer/cli/resources.md) |
| SDK | [SDK overview](../developer/sdk/overview.md) |
