-- migrate:up
-- How a bundle is priced on a store: the sum of its picks (as before), the listing's own price, or the sum less a whole
-- percentage. Only bundles read it; every other listing keeps the default
CREATE TYPE public.catalog_bundle_pricing AS ENUM ('sum', 'fixed', 'sum_minus_percent');

ALTER TABLE public.catalog_product_channel_listings
    ADD COLUMN bundle_pricing public.catalog_bundle_pricing DEFAULT 'sum'::public.catalog_bundle_pricing NOT NULL,
    ADD COLUMN bundle_discount_percent smallint DEFAULT 0 NOT NULL,
    ADD CONSTRAINT catalog_product_channel_listings_bundle_discount_percent_check CHECK (bundle_discount_percent BETWEEN 0 AND 100);

-- A cart.bundle.adding hook set this line's price - checkout keeps it rather than repricing from the components. Said
-- outright because a discounted bundle no longer equals its components' sum, which is how this used to be guessed
ALTER TABLE public.cart_lines
    ADD COLUMN bundle_hook_priced boolean DEFAULT false NOT NULL;

-- Lines already in carts keep the old guess, so a hook price set before this migration still survives checkout
UPDATE public.cart_lines l
SET bundle_hook_priced = true
WHERE l.quantity > 0
  AND EXISTS (SELECT 1 FROM public.cart_line_components c WHERE c.cart_line_id = l.id)
  AND l.unit_price <> (
    SELECT SUM(round(c.unit_price * (c.quantity / l.quantity)))
    FROM public.cart_line_components c
    WHERE c.cart_line_id = l.id
  );

-- migrate:down
ALTER TABLE public.cart_lines DROP COLUMN bundle_hook_priced;
ALTER TABLE public.catalog_product_channel_listings
    DROP COLUMN bundle_discount_percent,
    DROP COLUMN bundle_pricing;
DROP TYPE public.catalog_bundle_pricing;
