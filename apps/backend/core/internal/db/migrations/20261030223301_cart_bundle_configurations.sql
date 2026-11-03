-- migrate:up
-- A cart can hold the same bundle more than once, each line a different set of picks - the line is unique on its SKU and
-- that configuration. Lines without one (simple and configured) keep the empty key, so they stay one line per SKU
ALTER TABLE public.cart_lines ADD COLUMN config_key text DEFAULT ''::text NOT NULL;
DROP INDEX public.idx_cart_lines_cart_sku;
CREATE UNIQUE INDEX idx_cart_lines_cart_sku ON public.cart_lines USING btree (cart_id, product_sku, config_key);

-- migrate:down
DELETE FROM public.cart_lines a USING public.cart_lines b
WHERE a.cart_id = b.cart_id AND a.product_sku = b.product_sku AND a.config_key <> '' AND a.id > b.id;
DROP INDEX public.idx_cart_lines_cart_sku;
CREATE UNIQUE INDEX idx_cart_lines_cart_sku ON public.cart_lines USING btree (cart_id, product_sku);
ALTER TABLE public.cart_lines DROP COLUMN config_key;
