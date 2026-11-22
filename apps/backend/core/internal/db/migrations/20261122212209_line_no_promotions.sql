-- migrate:up
-- Whether coupons and promotions leave a line alone. A line whose price someone set is one by default - the price
-- is the deal - and whoever set it can let promotions apply after all. The line still counts toward what the cart
-- holds: a spend threshold sees it, a discount doesn't touch it. The order line keeps the flag
ALTER TABLE public.cart_lines ADD COLUMN no_promotions boolean DEFAULT false NOT NULL;
UPDATE public.cart_lines SET no_promotions = true WHERE price_source <> 'catalog';
ALTER TABLE public.order_lines ADD COLUMN no_promotions boolean DEFAULT false NOT NULL;

-- migrate:down
ALTER TABLE public.order_lines DROP COLUMN IF EXISTS no_promotions;
ALTER TABLE public.cart_lines DROP COLUMN IF EXISTS no_promotions;
