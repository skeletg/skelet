-- migrate:up
-- "When these answers, then these effects" across a product's options - the condition has the visible_if shape,
-- actions are require/optional/set_quantity/adjust_price, labels map a locale to the name a price change shows under
CREATE TABLE public.catalog_product_option_rules (
    id uuid DEFAULT uuidv7() NOT NULL PRIMARY KEY,
    product_id uuid NOT NULL REFERENCES public.catalog_products(id) ON DELETE CASCADE,
    sort_order integer DEFAULT 0 NOT NULL,
    labels jsonb DEFAULT '{}'::jsonb NOT NULL,
    condition jsonb NOT NULL,
    actions jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT catalog_product_option_rules_labels_object CHECK (jsonb_typeof(labels) = 'object'),
    CONSTRAINT catalog_product_option_rules_condition_object CHECK (jsonb_typeof(condition) = 'object'),
    CONSTRAINT catalog_product_option_rules_actions_array CHECK (jsonb_typeof(actions) = 'array')
);

CREATE INDEX idx_catalog_product_option_rules_product ON public.catalog_product_option_rules (product_id, sort_order);

-- migrate:down
DROP TABLE public.catalog_product_option_rules;
