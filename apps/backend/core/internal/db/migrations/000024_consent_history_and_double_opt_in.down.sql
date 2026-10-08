ALTER TABLE public.signup_forms ALTER COLUMN success_message SET DEFAULT 'Thanks for subscribing!'::text;

DROP TABLE IF EXISTS public.signup_confirmations;

DROP TABLE IF EXISTS public.customer_consent_events;
