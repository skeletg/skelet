-- migrate:up
-- An option choice backed by a product can be sold as that product's own line: its own SKU, tax class and price on
-- the cart and the order, tied to the line it was picked on. line_promotions lets coupons and promotions lower it -
-- they leave an add-on alone otherwise
ALTER TABLE public.catalog_product_option_values
    ADD COLUMN sold_as_line boolean DEFAULT false NOT NULL,
    ADD COLUMN line_promotions boolean DEFAULT false NOT NULL,
    ADD CONSTRAINT catalog_product_option_values_sold_as_line_check CHECK (NOT sold_as_line OR source_product_id IS NOT NULL);

-- A line that belongs to another: it goes when its parent goes, and holds per_parent of itself for each of the
-- parent. parent_value_id is the option choice on the parent it was picked as, so removing the line takes the pick back
ALTER TABLE public.cart_lines
    ADD COLUMN parent_line_id uuid REFERENCES public.cart_lines(id) ON DELETE CASCADE,
    ADD COLUMN per_parent numeric(14,4),
    ADD COLUMN parent_value_id uuid,
    ADD CONSTRAINT cart_lines_parent_check CHECK ((parent_line_id IS NULL) = (per_parent IS NULL)),
    ADD CONSTRAINT cart_lines_per_parent_check CHECK (per_parent IS NULL OR per_parent > 0),
    ADD CONSTRAINT cart_lines_parent_not_self_check CHECK (parent_line_id IS DISTINCT FROM id);
CREATE INDEX idx_cart_lines_parent ON public.cart_lines USING btree (parent_line_id) WHERE parent_line_id IS NOT NULL;

ALTER TABLE public.order_lines
    ADD COLUMN parent_line_id uuid REFERENCES public.order_lines(id),
    ADD CONSTRAINT order_lines_parent_not_self_check CHECK (parent_line_id IS DISTINCT FROM id);
CREATE INDEX idx_order_lines_parent ON public.order_lines USING btree (parent_line_id) WHERE parent_line_id IS NOT NULL;

-- migrate:down
DROP INDEX IF EXISTS public.idx_order_lines_parent;
ALTER TABLE public.order_lines
    DROP CONSTRAINT IF EXISTS order_lines_parent_not_self_check,
    DROP COLUMN IF EXISTS parent_line_id;
DROP INDEX IF EXISTS public.idx_cart_lines_parent;
ALTER TABLE public.cart_lines
    DROP CONSTRAINT IF EXISTS cart_lines_parent_not_self_check,
    DROP CONSTRAINT IF EXISTS cart_lines_per_parent_check,
    DROP CONSTRAINT IF EXISTS cart_lines_parent_check,
    DROP COLUMN IF EXISTS parent_value_id,
    DROP COLUMN IF EXISTS per_parent,
    DROP COLUMN IF EXISTS parent_line_id;
ALTER TABLE public.catalog_product_option_values
    DROP CONSTRAINT IF EXISTS catalog_product_option_values_sold_as_line_check,
    DROP COLUMN IF EXISTS line_promotions,
    DROP COLUMN IF EXISTS sold_as_line;
