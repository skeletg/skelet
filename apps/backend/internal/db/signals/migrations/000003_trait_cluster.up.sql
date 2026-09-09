-- The Synapse trait cluster - Signals writes every one of these and, with the roster below, computes them
-- without reading commerce at all.
--
-- What would otherwise keep them next door is RecomputeChannelTraits needing customers and orders:
-- orders are a spine-fed projection, and the customer roster is synced from commerce below.

CREATE TABLE public.customer_traits (
    customer_id uuid NOT NULL,
    channel_id uuid NOT NULL,
    recency_days integer DEFAULT '-1'::integer NOT NULL,
    frequency integer DEFAULT 0 NOT NULL,
    monetary_cents bigint DEFAULT 0 NOT NULL,
    rfm_r smallint DEFAULT 0 NOT NULL,
    rfm_f smallint DEFAULT 0 NOT NULL,
    rfm_m smallint DEFAULT 0 NOT NULL,
    lifecycle_stage text DEFAULT 'prospect'::text NOT NULL,
    engagement_score integer DEFAULT 0 NOT NULL,
    computed_at timestamp with time zone DEFAULT now() NOT NULL,
    churn_risk smallint DEFAULT 0 NOT NULL,
    predicted_clv_cents bigint DEFAULT 0 NOT NULL,
    next_order_days integer DEFAULT '-1'::integer NOT NULL,
    CONSTRAINT customer_traits_pkey PRIMARY KEY (customer_id)
);

CREATE INDEX idx_customer_traits_channel ON public.customer_traits USING btree (channel_id);
CREATE INDEX idx_customer_traits_lifecycle ON public.customer_traits USING btree (channel_id, lifecycle_stage);

-- The 30-day spine event count, and the lifetime order aggregate the spine feeds
CREATE TABLE public.customer_engagement_30d (
    customer_id uuid NOT NULL,
    channel_id uuid NOT NULL,
    engagement integer NOT NULL,
    computed_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT customer_engagement_30d_pkey PRIMARY KEY (customer_id)
);

CREATE INDEX customer_engagement_30d_channel ON public.customer_engagement_30d USING btree (channel_id);

CREATE TABLE public.customer_engagement_refresh (
    channel_id uuid NOT NULL,
    ran_at timestamp with time zone NOT NULL,
    CONSTRAINT customer_engagement_refresh_pkey PRIMARY KEY (channel_id)
);

CREATE TABLE public.customer_order_agg (
    customer_id uuid NOT NULL,
    channel_id uuid NOT NULL,
    order_count integer DEFAULT 0 NOT NULL,
    monetary_cents bigint DEFAULT 0 NOT NULL,
    first_order_at timestamp with time zone,
    last_order_at timestamp with time zone,
    computed_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT customer_order_agg_pkey PRIMARY KEY (customer_id)
);

CREATE INDEX customer_order_agg_channel ON public.customer_order_agg USING btree (channel_id);


--
-- The customer roster, synced from commerce.
--
-- The recompute needs every customer in a channel, not only the ones who bought something: a
-- never-ordered customer has to land as a prospect, and prospects are most of the table. Across
-- databases that list has to be copied rather than joined.
--
-- Carries the lifecycle-window override with it, because the only other thing the recompute took
-- from commerce was the shortest override among a customer's segments - resolving that in core,
-- where the segments live, keeps this to one sync instead of two.
--
-- Deletions matter more here than the values do: customer_traits had an ON DELETE CASCADE to
-- customers, which cannot cross databases, so a customer gone from this roster is what tells the
-- recompute to drop their traits.
--
CREATE TABLE public.customer_roster (
    customer_id uuid NOT NULL,
    channel_id uuid NOT NULL,
    lifecycle_new_days integer,
    lifecycle_active_days integer,
    lifecycle_at_risk_days integer,
    synced_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT customer_roster_pkey PRIMARY KEY (customer_id)
);

CREATE INDEX customer_roster_channel ON public.customer_roster USING btree (channel_id);
