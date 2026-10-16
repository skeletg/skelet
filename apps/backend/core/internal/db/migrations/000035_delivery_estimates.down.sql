ALTER TABLE public.shipping_carriers DROP COLUMN IF EXISTS delivers_on_weekends;

ALTER TABLE public.shipping_methods
    DROP CONSTRAINT IF EXISTS shipping_methods_estimated_days_check,
    DROP COLUMN IF EXISTS delivers_on_weekends,
    DROP COLUMN IF EXISTS estimated_days;
