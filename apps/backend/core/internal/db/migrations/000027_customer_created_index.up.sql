-- The customer export pages on (created_at, id), which otherwise re-sorts the whole table for every page
CREATE INDEX IF NOT EXISTS idx_customers_created_id ON public.customers USING btree (created_at, id);
