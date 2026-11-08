-- migrate:up
-- Core's tracking destinations send shop events to analytics and ad platforms now, so the suite forwards nothing
DROP TABLE public.event_destinations;

-- migrate:down
CREATE TABLE public.event_destinations (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    channel_id uuid NOT NULL,
    name text NOT NULL,
    kind text DEFAULT 'webhook'::text NOT NULL,
    target_url text NOT NULL,
    secret text DEFAULT ''::text NOT NULL,
    event_sources text[] DEFAULT '{}'::text[] NOT NULL,
    event_names text[] DEFAULT '{}'::text[] NOT NULL,
    is_active boolean DEFAULT true NOT NULL,
    cursor_created_at timestamp with time zone,
    cursor_event_id uuid,
    last_error text DEFAULT ''::text NOT NULL,
    last_forwarded_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT event_destinations_kind_check CHECK ((kind = ANY (ARRAY['webhook'::text, 'ga4_mp'::text, 'meta_capi'::text]))),
    CONSTRAINT event_destinations_pkey PRIMARY KEY (id)
);
CREATE INDEX idx_event_destinations_channel ON public.event_destinations USING btree (channel_id, is_active);
