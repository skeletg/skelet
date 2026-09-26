-- Lifetime order totals as core counts them, pushed with each customer - the event log only keeps a year
ALTER TABLE public.customer_roster ADD COLUMN order_count integer NOT NULL DEFAULT 0;
ALTER TABLE public.customer_roster ADD COLUMN monetary_cents bigint NOT NULL DEFAULT 0;
ALTER TABLE public.customer_roster ADD COLUMN first_order_at timestamp with time zone;
ALTER TABLE public.customer_roster ADD COLUMN last_order_at timestamp with time zone;
DROP TABLE IF EXISTS public.customer_order_agg;
