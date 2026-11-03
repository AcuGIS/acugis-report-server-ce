-- Docker demo seed (packaging only). Safe to re-run.

CREATE TABLE IF NOT EXISTS public.demo_items (
    id serial PRIMARY KEY,
    name text NOT NULL,
    quantity integer NOT NULL DEFAULT 0,
    created_at timestamptz NOT NULL DEFAULT now()
);

INSERT INTO public.demo_items (name, quantity)
SELECT v.name, v.quantity
FROM (VALUES
    ('Alpha', 10),
    ('Beta', 20),
    ('Gamma', 30)
) AS v(name, quantity)
WHERE NOT EXISTS (SELECT 1 FROM public.demo_items LIMIT 1);

-- Ensure columns expected by the Go datasource repository exist.
ALTER TABLE public.jasper_datasource
    ADD COLUMN IF NOT EXISTS db_type varchar(20) NOT NULL DEFAULT 'postgresql';
ALTER TABLE public.jasper_datasource
    ADD COLUMN IF NOT EXISTS options jsonb NOT NULL DEFAULT '{}'::jsonb;

INSERT INTO public.jasper_datasource (
    name, host, port, dbname, username, password, schema_name, db_type, enabled
) VALUES (
    'DemoPG',
    'postgres',
    5432,
    'jasper_reports',
    'jasper',
    :'jrs_db_pass',
    'public',
    'postgresql',
    true
)
ON CONFLICT (name) DO UPDATE
SET host = EXCLUDED.host,
    port = EXCLUDED.port,
    dbname = EXCLUDED.dbname,
    username = EXCLUDED.username,
    password = EXCLUDED.password,
    db_type = 'postgresql',
    enabled = true;
