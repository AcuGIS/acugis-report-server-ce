-- Admin-managed Embedded Analytics allowed origins.
-- Environment origins (ACUGIS_EMBED_ALLOWED_ORIGINS / JRS_EMBED_ALLOWED_ORIGINS)
-- are not stored here. The server merges both lists at request time.

CREATE TABLE IF NOT EXISTS public.embed_allowed_origin (
    origin text PRIMARY KEY,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by integer NULL REFERENCES public.dashboard_users (id) ON DELETE SET NULL
);
