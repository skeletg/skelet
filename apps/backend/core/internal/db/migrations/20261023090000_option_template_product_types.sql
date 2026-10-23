-- migrate:up
-- A product type's configurator templates reach every simple and configurable product of that type, ahead of the
-- templates the product picks itself. RESTRICT: a template still on a type isn't deleted out from under it
CREATE TABLE public.catalog_option_template_product_types (
    product_type_id uuid NOT NULL REFERENCES public.catalog_product_types(id) ON DELETE CASCADE,
    template_id uuid NOT NULL REFERENCES public.catalog_option_templates(id) ON DELETE RESTRICT,
    "position" integer DEFAULT 0 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    PRIMARY KEY (product_type_id, template_id)
);

CREATE INDEX idx_catalog_option_template_product_types_template ON public.catalog_option_template_product_types (template_id, product_type_id);

-- migrate:down
DROP TABLE public.catalog_option_template_product_types;
