-- Customers core erased, kept by id alone so a late or replayed write can't put them back on the spine
-- The roster row is only scrubbed, and a hard delete removes it, so neither can carry the marker
CREATE TABLE public.erased_customers (
    customer_id uuid NOT NULL,
    erased_at timestamp with time zone NOT NULL DEFAULT now()
);
ALTER TABLE ONLY public.erased_customers ADD CONSTRAINT erased_customers_pkey PRIMARY KEY (customer_id);
