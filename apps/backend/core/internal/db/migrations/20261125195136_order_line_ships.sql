-- migrate:up
-- Whether a line is something to ship, as its product type said when the order was placed. A line that isn't - a
-- service, a protection plan, a download, a custom charge - doesn't hold the order back from fulfilled
ALTER TABLE public.order_lines ADD COLUMN requires_shipping boolean DEFAULT true NOT NULL;
UPDATE public.order_lines SET requires_shipping = false WHERE is_custom;

-- migrate:down
ALTER TABLE public.order_lines DROP COLUMN IF EXISTS requires_shipping;
