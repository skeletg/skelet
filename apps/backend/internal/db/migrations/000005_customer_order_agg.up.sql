--
-- The RFM inputs as a projection, so trait compute stops joining the orders table
--
-- RecomputeChannelTraits read customers LEFT JOIN orders to derive recency, frequency and
-- monetary. That join is what keeps the Signals trait job inside the commerce database, and
-- orders is the table that grows fastest. The spine already carries order_placed with the
-- customer, channel, total and timestamp, so the aggregate can be read there and landed here,
-- the same shape customer_engagement_30d already uses for the 30-day event count.
--
-- Refreshed wholesale per channel by the trait-compute job right before the recompute reads it.
--
CREATE TABLE public.customer_order_agg (
    customer_id uuid NOT NULL,
    channel_id uuid NOT NULL,
    order_count integer DEFAULT 0 NOT NULL,
    monetary_cents bigint DEFAULT 0 NOT NULL,
    first_order_at timestamp with time zone,
    last_order_at timestamp with time zone,
    computed_at timestamp with time zone DEFAULT now() NOT NULL
);

ALTER TABLE ONLY public.customer_order_agg
    ADD CONSTRAINT customer_order_agg_pkey PRIMARY KEY (customer_id);

CREATE INDEX customer_order_agg_channel ON public.customer_order_agg USING btree (channel_id);

--
-- Whether a channel's projection has been built at all - the recompute must not read an empty
-- table as "nobody ever ordered", which would reset every customer's RFM to zero
--
CREATE TABLE public.customer_order_agg_refresh (
    channel_id uuid NOT NULL,
    ran_at timestamp with time zone NOT NULL
);

ALTER TABLE ONLY public.customer_order_agg_refresh
    ADD CONSTRAINT customer_order_agg_refresh_pkey PRIMARY KEY (channel_id);

--
-- Seed it from the orders table, so an existing install's RFM is correct the moment this lands.
-- The spine only goes back to when tracking started; `skelet signals backfill-orders` replays the
-- rest, and until it has, the refresh below leaves these seeded rows alone rather than swapping in
-- a truncated history. Matches what RecomputeChannelTraits computed inline before this migration.
--
INSERT INTO public.customer_order_agg (customer_id, channel_id, order_count, monetary_cents, first_order_at, last_order_at)
SELECT c.id, c.channel_id,
       COUNT(o.id)::int,
       COALESCE(SUM(o.total_amount), 0)::bigint,
       MIN(o.created_at),
       MAX(o.created_at)
FROM public.customers c
JOIN public.orders o
    ON o.customer_id = c.id
   AND o.status NOT IN ('cancelled', 'draft')
GROUP BY c.id, c.channel_id
ON CONFLICT (customer_id) DO NOTHING;
