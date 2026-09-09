--
-- Event destinations - the external receivers Signals forwards spine events to
--
-- Signals owns every row: it writes them from its own admin API and reads them in its forwarding
-- loop, and what kept them in commerce was the admin surface living there, which moved with this
--
-- The channel foreign key does not come along, since channels are core's and a key cannot cross
-- databases - a destination for a deleted channel is inert, and the forwarding loop skips it
--
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
    CONSTRAINT event_destinations_kind_check CHECK ((kind = ANY (ARRAY['webhook'::text, 'ga4_mp'::text, 'meta_capi'::text])))
);

ALTER TABLE ONLY public.event_destinations
    ADD CONSTRAINT event_destinations_pkey PRIMARY KEY (id);


CREATE INDEX idx_event_destinations_channel ON public.event_destinations USING btree (channel_id, is_active);
