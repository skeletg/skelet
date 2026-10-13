DROP INDEX IF EXISTS public.idx_review_media_asset;
DROP INDEX IF EXISTS public.idx_subscriptions_payment_method;
DROP INDEX IF EXISTS public.idx_review_votes_customer;
DROP INDEX IF EXISTS public.idx_promotion_redemptions_code;
DROP INDEX IF EXISTS public.idx_product_also_bought_other_product;
DROP INDEX IF EXISTS public.idx_catalog_product_links_linked_product;
DROP INDEX IF EXISTS public.idx_customers_default_shipping_address;
DROP INDEX IF EXISTS public.idx_customers_default_billing_address;

CREATE INDEX IF NOT EXISTS media_assets_object_key_idx ON public.media_assets USING btree (object_key);

DROP FUNCTION IF EXISTS public.escape_like(text);
