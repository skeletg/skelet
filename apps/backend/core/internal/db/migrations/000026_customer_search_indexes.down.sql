CREATE INDEX idx_customers_channel_id ON public.customers USING btree (channel_id);
CREATE UNIQUE INDEX customers_channel_email_key ON public.customers USING btree (channel_id, email);

DROP INDEX IF EXISTS public.idx_customers_last_name_trgm;
DROP INDEX IF EXISTS public.idx_customers_first_name_trgm;
DROP INDEX IF EXISTS public.idx_customers_email_trgm;
