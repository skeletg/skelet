CREATE TABLE public.customer_order_agg (
    customer_id uuid NOT NULL,
    channel_id uuid NOT NULL,
    order_count integer DEFAULT 0 NOT NULL,
    monetary_cents bigint DEFAULT 0 NOT NULL,
    first_order_at timestamp with time zone,
    last_order_at timestamp with time zone,
    computed_at timestamp with time zone DEFAULT now() NOT NULL
);
ALTER TABLE ONLY public.customer_order_agg ADD CONSTRAINT customer_order_agg_pkey PRIMARY KEY (customer_id);
CREATE INDEX customer_order_agg_channel ON public.customer_order_agg USING btree (channel_id);
ALTER TABLE public.customer_roster DROP COLUMN IF EXISTS last_order_at;
ALTER TABLE public.customer_roster DROP COLUMN IF EXISTS first_order_at;
ALTER TABLE public.customer_roster DROP COLUMN IF EXISTS monetary_cents;
ALTER TABLE public.customer_roster DROP COLUMN IF EXISTS order_count;
