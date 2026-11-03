-- Add multi-database support to jasper_datasource (PostgreSQL, MySQL, Oracle).
-- Safe to run more than once.

ALTER TABLE public.jasper_datasource
    ADD COLUMN IF NOT EXISTS db_type varchar(20) NOT NULL DEFAULT 'postgresql';

UPDATE public.jasper_datasource
SET db_type = 'postgresql'
WHERE db_type IS NULL OR trim(db_type) = '';
