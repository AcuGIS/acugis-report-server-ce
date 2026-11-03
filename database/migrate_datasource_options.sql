-- Provider-specific datasource options (SQL Server encrypt, Snowflake warehouse/key-pair, etc.).
-- Safe to run more than once.

ALTER TABLE public.jasper_datasource
    ADD COLUMN IF NOT EXISTS options jsonb NOT NULL DEFAULT '{}'::jsonb;

-- SQLite / GeoPackage paths and Snowflake account ids can exceed the old 120-char dbname limit.
ALTER TABLE public.jasper_datasource
    ALTER COLUMN dbname TYPE varchar(500);
