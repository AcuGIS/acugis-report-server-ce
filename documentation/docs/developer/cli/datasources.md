# CLI datasources

```bash
acugis-report datasources list
acugis-report datasources create -n postgres --db-type postgres --host db --dbname reports -u jrs -p secret
acugis-report datasources delete 3 --yes
acugis-report datasources context
```

## Limitations (server REST)

- List returns names only
- No detail GET
- No dedicated update endpoint (create upserts by name)
- No GeoServer / ArcGIS connection CRUD via REST

`show` / `update` CLI subcommands activate when the corresponding REST endpoints exist.

See [Server datasources](../../user-guide/datasources.md).
