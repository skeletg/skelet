--
-- Customer segments - audiences, which are a Signals capability rather than a commerce one
--
-- Core offers segment-gated promotions and CMS blocks only when Signals is running: with it off
-- there are no audiences to gate on, and both consumers fall through to matching nobody
--
-- The membership foreign key to customers does not come along, since customers are core's and a
-- key cannot cross databases - the roster sync prunes a deleted customer's memberships instead,
-- the same way it prunes their traits
--
CREATE TABLE public.customer_segments (
    id uuid DEFAULT uuidv7() NOT NULL,
    slug text NOT NULL,
    name text NOT NULL,
    description text DEFAULT ''::text NOT NULL,
    refresh_sql text DEFAULT ''::text NOT NULL,
    is_active boolean DEFAULT true NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    lifecycle_new_days integer,
    lifecycle_active_days integer,
    lifecycle_at_risk_days integer,
    rules jsonb
);

ALTER TABLE ONLY public.customer_segments
    ADD CONSTRAINT customer_segments_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.customer_segments
    ADD CONSTRAINT customer_segments_slug_key UNIQUE (slug);

CREATE TABLE public.customer_segment_memberships (
    customer_id uuid NOT NULL,
    segment_id uuid NOT NULL,
    added_at timestamp with time zone DEFAULT now() NOT NULL
);

ALTER TABLE ONLY public.customer_segment_memberships
    ADD CONSTRAINT customer_segment_memberships_pkey PRIMARY KEY (customer_id, segment_id);


ALTER TABLE ONLY public.customer_segment_memberships
    ADD CONSTRAINT customer_segment_memberships_segment_id_fkey FOREIGN KEY (segment_id) REFERENCES public.customer_segments(id) ON DELETE CASCADE;

CREATE TABLE public.segment_membership_daily (
    segment_id uuid NOT NULL,
    day date NOT NULL,
    member_count integer NOT NULL
);

ALTER TABLE ONLY public.segment_membership_daily
    ADD CONSTRAINT segment_membership_daily_pkey PRIMARY KEY (segment_id, day);

ALTER TABLE ONLY public.segment_membership_daily
    ADD CONSTRAINT segment_membership_daily_segment_id_fkey FOREIGN KEY (segment_id) REFERENCES public.customer_segments(id) ON DELETE CASCADE;

CREATE INDEX idx_customer_segment_memberships_segment ON public.customer_segment_memberships USING btree (segment_id);

CREATE INDEX idx_customer_segments_active ON public.customer_segments USING btree (is_active);
