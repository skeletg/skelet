-- The admin list searches with leading-wildcard ILIKE, which no btree can serve
CREATE INDEX idx_customers_email_trgm ON public.customers USING gin (email public.gin_trgm_ops);
CREATE INDEX idx_customers_first_name_trgm ON public.customers USING gin (first_name public.gin_trgm_ops);
CREATE INDEX idx_customers_last_name_trgm ON public.customers USING gin (last_name public.gin_trgm_ops);

-- Emails are stored normalized and customers_store_email_lower_key already enforces (channel_id, lower(btrim(email))),
-- which the lookup now reads through; it also leads with channel_id, so the single-column index was a duplicate
DROP INDEX IF EXISTS public.customers_channel_email_key;
DROP INDEX IF EXISTS public.idx_customers_channel_id;
