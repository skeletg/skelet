CREATE TABLE public.customer_consent_events (
    id uuid DEFAULT uuidv7() NOT NULL,
    customer_id uuid NOT NULL,
    kind text NOT NULL,
    opted_in boolean NOT NULL,
    source text NOT NULL,
    admin_id uuid,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT customer_consent_events_pkey PRIMARY KEY (id),
    CONSTRAINT customer_consent_events_customer_id_fkey FOREIGN KEY (customer_id) REFERENCES public.customers(id) ON DELETE CASCADE,
    CONSTRAINT customer_consent_events_admin_id_fkey FOREIGN KEY (admin_id) REFERENCES public.admin_users(id) ON DELETE SET NULL
);

CREATE INDEX idx_customer_consent_events_customer ON public.customer_consent_events USING btree (customer_id, created_at DESC);

CREATE TABLE public.signup_confirmations (
    id uuid DEFAULT uuidv7() NOT NULL,
    token_hash bytea NOT NULL,
    form_id uuid NOT NULL,
    channel_id uuid NOT NULL,
    email text NOT NULL,
    first_name text DEFAULT ''::text NOT NULL,
    consent_kind text NOT NULL,
    customer_id uuid,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    confirmed_at timestamp with time zone,
    CONSTRAINT signup_confirmations_pkey PRIMARY KEY (id),
    CONSTRAINT signup_confirmations_token_hash_key UNIQUE (token_hash),
    CONSTRAINT signup_confirmations_form_id_fkey FOREIGN KEY (form_id) REFERENCES public.signup_forms(id) ON DELETE CASCADE,
    CONSTRAINT signup_confirmations_channel_id_fkey FOREIGN KEY (channel_id) REFERENCES public.channels(id) ON DELETE CASCADE,
    CONSTRAINT signup_confirmations_customer_id_fkey FOREIGN KEY (customer_id) REFERENCES public.customers(id) ON DELETE SET NULL
);

CREATE INDEX idx_signup_confirmations_pending_expiry ON public.signup_confirmations USING btree (expires_at) WHERE (confirmed_at IS NULL);

CREATE INDEX idx_signup_confirmations_channel_email ON public.signup_confirmations USING btree (channel_id, email);

ALTER TABLE public.signup_forms ALTER COLUMN success_message SET DEFAULT 'Check your inbox to confirm your subscription.'::text;
