-- migrate:up
-- An option can fill itself from a rule - every SIMPLE product in a category, optionally narrowed to some attribute
-- values - beside or instead of hand-picked selections, so new products in the category show up without a bundle edit
ALTER TABLE public.catalog_product_bundle_options
    ADD COLUMN rule_category_id uuid REFERENCES public.catalog_categories(id) ON DELETE SET NULL,
    ADD COLUMN rule_attribute_value_ids uuid[] DEFAULT '{}'::uuid[] NOT NULL,
    ADD COLUMN rule_quantity numeric(14,4) DEFAULT 1 NOT NULL,
    ADD CONSTRAINT catalog_bundle_options_rule_quantity_check CHECK (rule_quantity > (0)::numeric);

-- migrate:down
ALTER TABLE public.catalog_product_bundle_options
    DROP CONSTRAINT catalog_bundle_options_rule_quantity_check,
    DROP COLUMN rule_quantity,
    DROP COLUMN rule_attribute_value_ids,
    DROP COLUMN rule_category_id;
