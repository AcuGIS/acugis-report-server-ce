-- Hierarchical content folders for AcuGIS Report Server.
-- Generic content_items membership keeps future resource types folder-aware
-- without altering each content table.

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

-- Polymorphic folder membership for reports, dashboards, templates, etc.
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

-- Per-user favorites across any content type.
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

-- Seed the immutable root Home folder (id may vary; lookup by parent_id IS NULL).
INSERT INTO public.content_folders (parent_id, name, description, icon, sort_order, acl_inherit)
SELECT NULL, 'Home', 'Root content folder', 'home', 0, true
WHERE NOT EXISTS (
    SELECT 1 FROM public.content_folders WHERE parent_id IS NULL
);

-- Register Home as an ACL resource and grant default group access.
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
