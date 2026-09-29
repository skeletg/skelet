-- Admin emails compare case-insensitively: stored lowercased and unique on lower(email), so a mixed-case sign-up can't shadow an existing account or miss its own reset mail
UPDATE public.admin_users SET email = lower(email) WHERE email <> lower(email);
ALTER TABLE public.admin_users DROP CONSTRAINT admin_users_email_key;
DROP INDEX IF EXISTS public.idx_admin_users_email;
CREATE UNIQUE INDEX admin_users_email_lower_key ON public.admin_users USING btree (lower(email));

-- Second-factor state: the last TOTP time step accepted (a code can't be replayed inside its window) and a failure counter that locks the factor out independent of the rate limiter
ALTER TABLE public.admin_users
    ADD COLUMN totp_last_step bigint,
    ADD COLUMN totp_failed_attempts integer DEFAULT 0 NOT NULL,
    ADD COLUMN totp_locked_until timestamp with time zone;
