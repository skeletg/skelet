-- migrate:up
-- A kit line snapshots its components onto the order like a bundle line, so they show on the order and can come back one
-- at a time. A kit component has no bundle option or selection, so those go NULL for it
ALTER TABLE public.order_line_components
    ALTER COLUMN bundle_option_id DROP NOT NULL,
    ALTER COLUMN bundle_selection_id DROP NOT NULL,
    ADD COLUMN returnable boolean DEFAULT true NOT NULL;

-- Whether a kit's items can be returned one at a time, or only the whole kit. Snapshotted onto each order component as
-- returnable, so a sold kit keeps the terms it was bought under
ALTER TABLE public.catalog_products
    ADD COLUMN kit_items_returnable boolean DEFAULT false NOT NULL;

-- migrate:down
ALTER TABLE public.catalog_products DROP COLUMN kit_items_returnable;
UPDATE public.order_return_lines SET order_line_component_id = NULL
WHERE order_line_component_id IN (SELECT id FROM public.order_line_components WHERE bundle_option_id IS NULL);
DELETE FROM public.order_line_components WHERE bundle_option_id IS NULL;
ALTER TABLE public.order_line_components
    DROP COLUMN returnable,
    ALTER COLUMN bundle_option_id SET NOT NULL,
    ALTER COLUMN bundle_selection_id SET NOT NULL;
