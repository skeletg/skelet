-- migrate:up
-- How the product page asks a product's options: all at once, or one step at a time with the earlier picks above
ALTER TABLE public.catalog_products
    ADD COLUMN configurator_layout text DEFAULT 'list'::text NOT NULL,
    ADD CONSTRAINT catalog_products_configurator_layout CHECK (configurator_layout IN ('list', 'steps'));

-- migrate:down
ALTER TABLE public.catalog_products DROP COLUMN configurator_layout;
