-- migrate:up
-- Per selection: a whole percent off its component price (an add-on deal), and whether the customer sets how many of it
-- go in each bundle, with quantity as the starting amount
ALTER TABLE public.catalog_product_bundle_selections
    ADD COLUMN discount_percent smallint DEFAULT 0 NOT NULL,
    ADD COLUMN customer_quantity boolean DEFAULT false NOT NULL,
    ADD CONSTRAINT catalog_product_bundle_selections_discount_percent_check CHECK (discount_percent BETWEEN 0 AND 100);

-- migrate:down
ALTER TABLE public.catalog_product_bundle_selections
    DROP CONSTRAINT catalog_product_bundle_selections_discount_percent_check,
    DROP COLUMN customer_quantity,
    DROP COLUMN discount_percent;
