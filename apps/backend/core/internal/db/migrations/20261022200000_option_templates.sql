-- migrate:up
-- Configurator templates: a named set of options and rules kept once and assigned to many products. An option or rule
-- belongs to exactly one product or one template; a bare FIXED amount on a template's options is in its currency
CREATE TABLE public.catalog_option_templates (
    id uuid DEFAULT uuidv7() NOT NULL PRIMARY KEY,
    name text NOT NULL,
    description text DEFAULT ''::text NOT NULL,
    currency_code text DEFAULT 'USD'::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT catalog_option_templates_name_not_blank CHECK (btrim(name) <> ''),
    CONSTRAINT catalog_option_templates_currency_code CHECK (currency_code ~ '^[A-Z]{3}$')
);

CREATE UNIQUE INDEX ux_catalog_option_templates_name ON public.catalog_option_templates (lower(name));

ALTER TABLE public.catalog_product_options
    ALTER COLUMN product_id DROP NOT NULL,
    ADD COLUMN template_id uuid REFERENCES public.catalog_option_templates(id) ON DELETE CASCADE,
    ADD CONSTRAINT catalog_product_options_one_owner CHECK ((product_id IS NULL) <> (template_id IS NULL));

CREATE INDEX idx_catalog_product_options_template ON public.catalog_product_options (template_id, sort_order)
    WHERE template_id IS NOT NULL;

ALTER TABLE public.catalog_product_option_rules
    ALTER COLUMN product_id DROP NOT NULL,
    ADD COLUMN template_id uuid REFERENCES public.catalog_option_templates(id) ON DELETE CASCADE,
    ADD CONSTRAINT catalog_product_option_rules_one_owner CHECK ((product_id IS NULL) <> (template_id IS NULL));

CREATE INDEX idx_catalog_product_option_rules_template ON public.catalog_product_option_rules (template_id, sort_order)
    WHERE template_id IS NOT NULL;

-- RESTRICT: a template still on a product isn't deleted out from under it
CREATE TABLE public.catalog_option_template_products (
    product_id uuid NOT NULL REFERENCES public.catalog_products(id) ON DELETE CASCADE,
    template_id uuid NOT NULL REFERENCES public.catalog_option_templates(id) ON DELETE RESTRICT,
    "position" integer DEFAULT 0 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    PRIMARY KEY (product_id, template_id)
);

CREATE INDEX idx_catalog_option_template_products_template ON public.catalog_option_template_products (template_id, product_id);

-- migrate:down
DROP TABLE public.catalog_option_template_products;
DELETE FROM public.catalog_product_option_rules WHERE template_id IS NOT NULL;
DROP INDEX public.idx_catalog_product_option_rules_template;
ALTER TABLE public.catalog_product_option_rules
    DROP CONSTRAINT catalog_product_option_rules_one_owner,
    DROP COLUMN template_id,
    ALTER COLUMN product_id SET NOT NULL;
DELETE FROM public.catalog_product_options WHERE template_id IS NOT NULL;
DROP INDEX public.idx_catalog_product_options_template;
ALTER TABLE public.catalog_product_options
    DROP CONSTRAINT catalog_product_options_one_owner,
    DROP COLUMN template_id,
    ALTER COLUMN product_id SET NOT NULL;
DROP TABLE public.catalog_option_templates;
