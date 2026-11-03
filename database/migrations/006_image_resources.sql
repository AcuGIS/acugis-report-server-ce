-- Image Resource metadata for jasper_content_resource.
-- Images remain a resource_type ('image'); these columns are nullable for other types.

ALTER TABLE public.jasper_content_resource
    ADD COLUMN IF NOT EXISTS width integer NULL;

ALTER TABLE public.jasper_content_resource
    ADD COLUMN IF NOT EXISTS height integer NULL;

ALTER TABLE public.jasper_content_resource
    ADD COLUMN IF NOT EXISTS has_alpha boolean NULL;
