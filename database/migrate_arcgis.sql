-- ArcGIS Feature Server connections for authenticated REST queries after publish.
-- Designer embeds connectionName (Connection Manager display name), not local UUID.
-- Apply against the application database (same as jasper_datasource / jasper_geoserver).

CREATE TABLE IF NOT EXISTS public.jasper_arcgis (
    id serial PRIMARY KEY,
    name varchar(80) NOT NULL UNIQUE,
    url varchar(500) NOT NULL,
    username varchar(250) NOT NULL DEFAULT '',
    password varchar(250) NOT NULL DEFAULT '',
    enabled boolean NOT NULL DEFAULT true,
    created_at timestamp without time zone NOT NULL DEFAULT now()
);
