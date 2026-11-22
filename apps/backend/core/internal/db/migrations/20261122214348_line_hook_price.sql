-- migrate:up
-- A third source for a line's price: a hook - a plugin's handler or a sync webhook answering cart.line.adding or
-- cart.line.updating. Like an admin's it holds through repricing, and price_ref names who set it ("plugin:<slug>",
-- "webhook:<name>"). The order line keeps both
ALTER TABLE public.cart_lines
    ADD COLUMN price_ref text DEFAULT '' NOT NULL,
    DROP CONSTRAINT cart_lines_price_source_check,
    ADD CONSTRAINT cart_lines_price_source_check CHECK (price_source IN ('catalog', 'admin', 'hook'));
ALTER TABLE public.order_lines
    ADD COLUMN price_ref text DEFAULT '' NOT NULL,
    DROP CONSTRAINT order_lines_price_source_check,
    ADD CONSTRAINT order_lines_price_source_check CHECK (price_source IN ('catalog', 'admin', 'hook'));

-- migrate:down
UPDATE public.order_lines SET price_source = 'catalog' WHERE price_source = 'hook';
ALTER TABLE public.order_lines
    DROP CONSTRAINT order_lines_price_source_check,
    ADD CONSTRAINT order_lines_price_source_check CHECK (price_source IN ('catalog', 'admin')),
    DROP COLUMN IF EXISTS price_ref;
UPDATE public.cart_lines SET price_source = 'catalog' WHERE price_source = 'hook';
ALTER TABLE public.cart_lines
    DROP CONSTRAINT cart_lines_price_source_check,
    ADD CONSTRAINT cart_lines_price_source_check CHECK (price_source IN ('catalog', 'admin')),
    DROP COLUMN IF EXISTS price_ref;
