-- Neutralises LIKE's wildcards in a search term, so a typed % or _ matches itself - immutable SQL, so the planner inlines it
CREATE FUNCTION public.escape_like(s text) RETURNS text
    LANGUAGE sql IMMUTABLE STRICT PARALLEL SAFE
    AS $$ SELECT replace(replace(replace(s, '\', '\\'), '%', '\%'), '_', '\_') $$;

-- The unique constraint's index already serves every object_key lookup
DROP INDEX IF EXISTS public.media_assets_object_key_idx;

-- Foreign keys whose parent delete otherwise scans the child table
CREATE INDEX IF NOT EXISTS idx_customers_default_billing_address ON public.customers USING btree (default_billing_address_id) WHERE default_billing_address_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_customers_default_shipping_address ON public.customers USING btree (default_shipping_address_id) WHERE default_shipping_address_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_catalog_product_links_linked_product ON public.catalog_product_links USING btree (linked_product_id);
CREATE INDEX IF NOT EXISTS idx_product_also_bought_other_product ON public.product_also_bought USING btree (other_product_id);
CREATE INDEX IF NOT EXISTS idx_promotion_redemptions_code ON public.promotion_redemptions USING btree (code_id) WHERE code_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_review_votes_customer ON public.review_votes USING btree (customer_id) WHERE customer_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_subscriptions_payment_method ON public.subscriptions USING btree (payment_method_id) WHERE payment_method_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_review_media_asset ON public.review_media USING btree (asset_id);
