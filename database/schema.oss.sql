CREATE TABLE IF NOT EXISTS public.jasper_datasource (
    id serial PRIMARY KEY,
    name varchar(80) NOT NULL UNIQUE,
    host varchar(250) NOT NULL,
    port integer NOT NULL DEFAULT 5432,
    dbname varchar(120) NOT NULL,
    username varchar(250) NOT NULL,
    password varchar(250) NOT NULL DEFAULT '',
    schema_name varchar(80) NOT NULL DEFAULT 'public',
    auth_type varchar(40) NOT NULL DEFAULT 'password',
    public_key_resource_uuid uuid NULL,
    private_key_resource_uuid uuid NULL,
    enabled boolean NOT NULL DEFAULT true,
    created_at timestamp without time zone NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS public.jasper_access_group (
    id serial PRIMARY KEY,
    name varchar(120) NOT NULL UNIQUE
);

INSERT INTO public.jasper_access_group (name)
VALUES ('Public')
ON CONFLICT (name) DO NOTHING;

CREATE TABLE IF NOT EXISTS public.jasper_schedule (
    id serial PRIMARY KEY,
    cron_period varchar(40) NOT NULL DEFAULT 'never',
    cron_custom varchar(120) NOT NULL DEFAULT '',
    schedule_mode varchar(40) NOT NULL DEFAULT '',
    repeat_minutes integer,
    hourly_minute integer,
    daily_time varchar(8) NOT NULL DEFAULT '',
    weekly_day varchar(16) NOT NULL DEFAULT '',
    weekly_time varchar(8) NOT NULL DEFAULT '',
    monthly_day integer,
    monthly_time varchar(8) NOT NULL DEFAULT '',
    once_datetime varchar(32) NOT NULL DEFAULT '',
    report_name varchar(120) NOT NULL,
    datasource_name varchar(80) NOT NULL DEFAULT '',
    format varchar(20) NOT NULL DEFAULT 'pdf',
    filename varchar(250) NOT NULL DEFAULT '',
    email text NOT NULL DEFAULT '',
    email_subject varchar(500) NOT NULL DEFAULT '',
    email_body text NOT NULL DEFAULT '',
    email_template varchar(120) NOT NULL DEFAULT '',
    noemail boolean NOT NULL DEFAULT false,
    url_opt_params text NOT NULL DEFAULT '',
    enabled boolean NOT NULL DEFAULT true,
    created_at timestamp without time zone NOT NULL DEFAULT now(),
    updated_at timestamp without time zone NOT NULL DEFAULT now()
);

-- Email templates are HTML files under {JASPER_HOME}/email_tmpl/ (see jrs_email_tmpl_dir), not DB rows.

CREATE TABLE IF NOT EXISTS public.jasper_recipient (
    id serial PRIMARY KEY,
    name varchar(120) NOT NULL,
    email varchar(250) NOT NULL,
    notes text NOT NULL DEFAULT '',
    created_at timestamp without time zone NOT NULL DEFAULT now(),
    updated_at timestamp without time zone NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS jasper_recipient_email_lower_idx
    ON public.jasper_recipient (lower(email));

CREATE TABLE IF NOT EXISTS public.jasper_schedule_execution (
    id serial PRIMARY KEY,
    schedule_id integer REFERENCES public.jasper_schedule(id) ON DELETE SET NULL,
    status varchar(40) NOT NULL DEFAULT 'pending',
    started_at timestamp without time zone NOT NULL DEFAULT now(),
    finished_at timestamp without time zone,
    output_file text NOT NULL DEFAULT '',
    email_status varchar(80) NOT NULL DEFAULT '',
    error_message text NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS jasper_schedule_execution_schedule_idx
    ON public.jasper_schedule_execution (schedule_id, started_at DESC);

-- ---------------------------------------------------------------------------
-- Local authentication (AcuGIS Report Server stand-alone)
-- Table shapes match Central metadata so lib/auth.php SQL works unchanged.
-- Do not seed a default password hash here — create the admin user on first
-- install via scripts/seed_admin.php using env JRS_ADMIN_USER (default admin)
-- and JRS_ADMIN_PASSWORD (required).
-- ---------------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS public.dashboard_users (
    id serial PRIMARY KEY,
    username text NOT NULL UNIQUE,
    email text NOT NULL DEFAULT '',
    password_hash text NOT NULL,
    groups jsonb NOT NULL DEFAULT '[]'::jsonb,
    is_admin boolean NOT NULL DEFAULT false,
    enabled boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS public.dashboard_sessions (
    token_hash text PRIMARY KEY,
    username text NOT NULL REFERENCES public.dashboard_users (username) ON DELETE CASCADE,
    expires_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

-- Personal access tokens (Bearer API auth). Same model as AcuGIS Cloud; prefix jrs_.
CREATE TABLE IF NOT EXISTS public.personal_access_tokens (
    id serial PRIMARY KEY,
    user_id integer NOT NULL REFERENCES public.dashboard_users (id) ON DELETE CASCADE,
    name text NOT NULL,
    description text NOT NULL DEFAULT '',
    token_hash text NOT NULL UNIQUE,
    token_prefix text NOT NULL,
    scopes jsonb NOT NULL DEFAULT '[]'::jsonb,
    expires_at timestamptz,
    last_used_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    revoked_at timestamptz,
    created_by_ip text NOT NULL DEFAULT '',
    last_used_ip text NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_personal_access_tokens_user_id
    ON public.personal_access_tokens (user_id);

CREATE INDEX IF NOT EXISTS idx_personal_access_tokens_token_hash
    ON public.personal_access_tokens (token_hash);

CREATE TABLE IF NOT EXISTS public.auth_groups (
    id serial PRIMARY KEY,
    name character varying(100) NOT NULL UNIQUE,
    description text,
    created_at timestamp without time zone NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS public.auth_group_members (
    id serial PRIMARY KEY,
    group_id integer REFERENCES public.auth_groups (id) ON DELETE CASCADE,
    user_id integer REFERENCES public.dashboard_users (id) ON DELETE CASCADE,
    created_at timestamp without time zone NOT NULL DEFAULT now(),
    UNIQUE (group_id, user_id)
);

CREATE TABLE IF NOT EXISTS public.resources (
    id serial PRIMARY KEY,
    resource_type character varying(100) NOT NULL,
    resource_key character varying(255) NOT NULL,
    display_name character varying(255),
    service character varying(100),
    created_by integer,
    created_at timestamp without time zone NOT NULL DEFAULT now(),
    UNIQUE (resource_type, resource_key)
);

CREATE TABLE IF NOT EXISTS public.resource_permissions (
    id serial PRIMARY KEY,
    resource_id integer REFERENCES public.resources (id) ON DELETE CASCADE,
    group_id integer REFERENCES public.auth_groups (id) ON DELETE CASCADE,
    permission character varying(50) NOT NULL,
    created_at timestamp without time zone NOT NULL DEFAULT now(),
    UNIQUE (resource_id, group_id, permission)
);

INSERT INTO public.auth_groups (name, description)
VALUES
    ('Administrators', 'Full administrative access'),
    ('Editors', 'Edit reports and configuration'),
    ('Public', 'View-only public access')
ON CONFLICT (name) DO NOTHING;

INSERT INTO public.resources (resource_type, resource_key, display_name, service)
VALUES ('service', 'jasper_reports', 'AcuGIS Report Server', 'platform')
ON CONFLICT (resource_type, resource_key) DO UPDATE
SET display_name = COALESCE(EXCLUDED.display_name, resources.display_name);

INSERT INTO public.resource_permissions (resource_id, group_id, permission)
SELECT r.id, g.id, v.permission
FROM public.resources r
CROSS JOIN (VALUES
    ('Administrators', 'admin'),
    ('Editors', 'edit'),
    ('Public', 'view')
) AS v(group_name, permission)
JOIN public.auth_groups g ON g.name = v.group_name
WHERE r.resource_type = 'service'
    AND r.resource_key = 'jasper_reports'
ON CONFLICT (resource_id, group_id, permission) DO NOTHING;

-- GeoServer connections for Designer-published maps (lookup by stable connection name).
-- Designer Connection Manager UUID is local-only; JRXML embeds connectionName matching name here.
CREATE TABLE IF NOT EXISTS public.jasper_geoserver (
    id serial PRIMARY KEY,
    name varchar(80) NOT NULL UNIQUE,
    url varchar(500) NOT NULL,
    username varchar(250) NOT NULL DEFAULT '',
    password varchar(250) NOT NULL DEFAULT '',
    enabled boolean NOT NULL DEFAULT true,
    created_at timestamp without time zone NOT NULL DEFAULT now()
);

-- ArcGIS Feature Server connections. The same row is used for tabular report
-- fill and ArcGIS map rendering (lookup by stable connection name).
CREATE TABLE IF NOT EXISTS public.jasper_arcgis (
    id serial PRIMARY KEY,
    name varchar(80) NOT NULL UNIQUE,
    url varchar(500) NOT NULL,
    username varchar(250) NOT NULL DEFAULT '',
    password varchar(250) NOT NULL DEFAULT '',
    enabled boolean NOT NULL DEFAULT true,
    created_at timestamp without time zone NOT NULL DEFAULT now()
);

-- ---------------------------------------------------------------------------
-- Hierarchical content folders (generic content hierarchy)
-- See also database/migrations/004_content_folders.sql
-- ---------------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS public.content_folders (
    id serial PRIMARY KEY,
    parent_id integer REFERENCES public.content_folders (id) ON DELETE CASCADE,
    name varchar(255) NOT NULL,
    description text NOT NULL DEFAULT '',
    icon varchar(80) NOT NULL DEFAULT 'folder',
    sort_order integer NOT NULL DEFAULT 0,
    acl_inherit boolean NOT NULL DEFAULT true,
    created_by integer REFERENCES public.dashboard_users (id) ON DELETE SET NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT content_folders_root_name_chk CHECK (
        parent_id IS NOT NULL OR lower(name) = 'home'
    )
);

CREATE INDEX IF NOT EXISTS content_folders_parent_idx
    ON public.content_folders (parent_id, sort_order, name);

CREATE UNIQUE INDEX IF NOT EXISTS content_folders_root_unique_idx
    ON public.content_folders ((1))
    WHERE parent_id IS NULL;

CREATE UNIQUE INDEX IF NOT EXISTS content_folders_sibling_name_idx
    ON public.content_folders (COALESCE(parent_id, 0), lower(name));

CREATE TABLE IF NOT EXISTS public.content_items (
    id serial PRIMARY KEY,
    folder_id integer NOT NULL REFERENCES public.content_folders (id) ON DELETE CASCADE,
    content_type varchar(100) NOT NULL,
    content_key varchar(255) NOT NULL,
    display_name varchar(255) NOT NULL DEFAULT '',
    sort_order integer NOT NULL DEFAULT 0,
    acl_inherit boolean NOT NULL DEFAULT true,
    created_by integer REFERENCES public.dashboard_users (id) ON DELETE SET NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (content_type, content_key)
);

CREATE INDEX IF NOT EXISTS content_items_folder_idx
    ON public.content_items (folder_id, sort_order, content_type, content_key);

CREATE INDEX IF NOT EXISTS content_items_type_idx
    ON public.content_items (content_type);

CREATE TABLE IF NOT EXISTS public.content_favorites (
    id serial PRIMARY KEY,
    user_id integer NOT NULL REFERENCES public.dashboard_users (id) ON DELETE CASCADE,
    content_type varchar(100) NOT NULL,
    content_key varchar(255) NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (user_id, content_type, content_key)
);

CREATE INDEX IF NOT EXISTS content_favorites_user_idx
    ON public.content_favorites (user_id, created_at DESC);

INSERT INTO public.content_folders (parent_id, name, description, icon, sort_order, acl_inherit)
SELECT NULL, 'Home', 'Root content folder', 'home', 0, true
WHERE NOT EXISTS (
    SELECT 1 FROM public.content_folders WHERE parent_id IS NULL
);

INSERT INTO public.resources (resource_type, resource_key, display_name, service, created_by)
SELECT
    'content_folder',
    f.id::text,
    'Home',
    'jasper_reports',
    NULL
FROM public.content_folders f
WHERE f.parent_id IS NULL
ON CONFLICT (resource_type, resource_key) DO UPDATE
SET display_name = COALESCE(EXCLUDED.display_name, resources.display_name);

INSERT INTO public.resource_permissions (resource_id, group_id, permission)
SELECT r.id, g.id, v.permission
FROM public.resources r
CROSS JOIN (VALUES
    ('Administrators', 'admin'),
    ('Editors', 'edit'),
    ('Public', 'view')
) AS v(group_name, permission)
JOIN public.auth_groups g ON g.name = v.group_name
WHERE r.resource_type = 'content_folder'
  AND r.resource_key = (
      SELECT f.id::text FROM public.content_folders f WHERE f.parent_id IS NULL LIMIT 1
  )
ON CONFLICT (resource_id, group_id, permission) DO NOTHING;

-- ---------------------------------------------------------------------------
-- Folder Resources (keys, certificates, generic files)
-- See also database/migrations/005_content_resources.sql
-- ---------------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS public.jasper_content_resource (
    id serial PRIMARY KEY,
    uuid uuid NOT NULL UNIQUE,
    folder_id integer NOT NULL REFERENCES public.content_folders (id) ON DELETE RESTRICT,
    display_name varchar(255) NOT NULL,
    resource_type varchar(80) NOT NULL,
    original_filename varchar(512) NOT NULL DEFAULT '',
    extension varchar(32) NOT NULL DEFAULT '',
    storage_filename varchar(512) NOT NULL,
    mime_type varchar(255) NOT NULL DEFAULT 'application/octet-stream',
    sha256 char(64) NOT NULL DEFAULT '',
    size_bytes bigint NOT NULL DEFAULT 0,
    width integer NULL,
    height integer NULL,
    has_alpha boolean NULL,
    created_by integer REFERENCES public.dashboard_users (id) ON DELETE SET NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS jasper_content_resource_folder_idx
    ON public.jasper_content_resource (folder_id, display_name);

CREATE INDEX IF NOT EXISTS jasper_content_resource_type_idx
    ON public.jasper_content_resource (resource_type);

CREATE INDEX IF NOT EXISTS jasper_content_resource_sha256_idx
    ON public.jasper_content_resource (sha256);

-- Admin-managed Embedded Analytics allowed origins.
-- See database/migrations/017_embed_allowed_origins.sql.
-- Environment origins are not stored in this table.
CREATE TABLE IF NOT EXISTS public.embed_allowed_origin (
    origin text PRIMARY KEY,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by integer NULL REFERENCES public.dashboard_users (id) ON DELETE SET NULL
);

