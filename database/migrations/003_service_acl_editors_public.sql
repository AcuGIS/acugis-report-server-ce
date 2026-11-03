-- Allow Editors/Public into the jasper_reports service (catalog UI).
-- Administrators already have admin from the base schema.

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
