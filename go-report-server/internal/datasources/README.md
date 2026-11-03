# Datasources package

Jasper datasource catalog for Map Report runtime and admin CRUD.

## Behavior

- Find enabled `jasper_datasource` by name (legacy `pg_store` fallback)
- List names / list enabled rows
- Upsert by unique name, delete by id (PHP `reports/datasources.php`)
- Ensure `db_type` column exists (`ADD COLUMN IF NOT EXISTS`)
- JDBC settings + Tomcat `context.xml` generation (`generate_jasper_context.php`)

## ACL

- Datasources UI: service `jasper_reports` **admin**
- Generate context: service `jasper_reports` **edit**
- Name list API under `/go`: catalog/service view when auth enabled
