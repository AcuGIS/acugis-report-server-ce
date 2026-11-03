-- First-class folder Resources (keys, certificates, generic files).
-- UUID-based filesystem storage under {appRoot}/resources/{uuid}{ext}.
-- Connections (jasper_datasource) may soft-reference resource UUIDs for auth.

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

-- Datasource authentication: password | oauth | key_pair (soft UUID refs).
ALTER TABLE public.jasper_datasource
    ADD COLUMN IF NOT EXISTS auth_type varchar(40) NOT NULL DEFAULT 'password';

ALTER TABLE public.jasper_datasource
    ADD COLUMN IF NOT EXISTS public_key_resource_uuid uuid NULL;

ALTER TABLE public.jasper_datasource
    ADD COLUMN IF NOT EXISTS private_key_resource_uuid uuid NULL;
