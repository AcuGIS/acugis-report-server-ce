-- Optional repeat-scheduler columns (backward compatible; safe to run multiple times).
ALTER TABLE public.jasper_schedule
    ADD COLUMN IF NOT EXISTS schedule_mode varchar(40) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS repeat_minutes integer,
    ADD COLUMN IF NOT EXISTS hourly_minute integer,
    ADD COLUMN IF NOT EXISTS daily_time varchar(8) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS weekly_day varchar(16) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS weekly_time varchar(8) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS monthly_day integer,
    ADD COLUMN IF NOT EXISTS monthly_time varchar(8) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS once_datetime varchar(32) NOT NULL DEFAULT '';
