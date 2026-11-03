-- Personal access tokens for Bearer API authentication (AcuGIS Cloud-compatible model).
-- Prefix: jrs_…  Hash: SHA-256 hex of full plaintext. Plaintext is never stored.

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
