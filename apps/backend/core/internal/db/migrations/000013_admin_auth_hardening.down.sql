ALTER TABLE public.admin_users
    DROP COLUMN IF EXISTS totp_locked_until,
    DROP COLUMN IF EXISTS totp_failed_attempts,
    DROP COLUMN IF EXISTS totp_last_step;

DROP INDEX IF EXISTS public.admin_users_email_lower_key;
ALTER TABLE public.admin_users ADD CONSTRAINT admin_users_email_key UNIQUE (email);
CREATE INDEX idx_admin_users_email ON public.admin_users USING btree (email);
