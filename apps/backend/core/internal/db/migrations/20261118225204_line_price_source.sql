-- migrate:up
-- Where a line's price came from. A 'catalog' line is priced again from the catalog whenever checkout reads the cart;
-- any other source holds its price. The catalog price it replaced, who set it and why ride along, and the order line
-- keeps all four. No FK on who: the console may act for a service or integration identity with no admin row
ALTER TABLE public.cart_lines
    ADD COLUMN price_source text DEFAULT 'catalog' NOT NULL,
    ADD COLUMN price_set_by uuid,
    ADD COLUMN price_reason text DEFAULT '' NOT NULL,
    ADD COLUMN list_unit_price bigint,
    ADD CONSTRAINT cart_lines_price_source_check CHECK (price_source IN ('catalog', 'admin')),
    ADD CONSTRAINT cart_lines_list_unit_price_check CHECK (list_unit_price IS NULL OR list_unit_price >= 0);
UPDATE public.cart_lines SET price_source = 'admin' WHERE price_override;
ALTER TABLE public.cart_lines DROP COLUMN price_override;

ALTER TABLE public.order_lines
    ADD COLUMN price_source text DEFAULT 'catalog' NOT NULL,
    ADD COLUMN price_set_by uuid,
    ADD COLUMN price_reason text DEFAULT '' NOT NULL,
    ADD COLUMN list_unit_price bigint,
    ADD CONSTRAINT order_lines_price_source_check CHECK (price_source IN ('catalog', 'admin')),
    ADD CONSTRAINT order_lines_list_unit_price_check CHECK (list_unit_price IS NULL OR list_unit_price >= 0);
UPDATE public.order_lines SET price_source = 'admin' WHERE is_custom;

-- migrate:down
ALTER TABLE public.order_lines
    DROP CONSTRAINT IF EXISTS order_lines_list_unit_price_check,
    DROP CONSTRAINT IF EXISTS order_lines_price_source_check,
    DROP COLUMN IF EXISTS list_unit_price,
    DROP COLUMN IF EXISTS price_reason,
    DROP COLUMN IF EXISTS price_set_by,
    DROP COLUMN IF EXISTS price_source;

ALTER TABLE public.cart_lines ADD COLUMN price_override boolean DEFAULT false NOT NULL;
UPDATE public.cart_lines SET price_override = true WHERE price_source <> 'catalog';
ALTER TABLE public.cart_lines
    DROP CONSTRAINT IF EXISTS cart_lines_list_unit_price_check,
    DROP CONSTRAINT IF EXISTS cart_lines_price_source_check,
    DROP COLUMN IF EXISTS list_unit_price,
    DROP COLUMN IF EXISTS price_reason,
    DROP COLUMN IF EXISTS price_set_by,
    DROP COLUMN IF EXISTS price_source;
