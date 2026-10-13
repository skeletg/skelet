DELETE FROM public.remote_providers WHERE kind = 'delivery_points';
ALTER TABLE public.remote_providers DROP CONSTRAINT remote_providers_kind_check;
ALTER TABLE public.remote_providers ADD CONSTRAINT remote_providers_kind_check CHECK ((kind = ANY (ARRAY['shipping_carrier'::text, 'tax_provider'::text, 'address_validator'::text, 'promotion_condition'::text, 'promotion_action'::text, 'flow_action'::text, 'fraud_risk'::text, 'currency_rate'::text, 'pricing'::text, 'recommendation'::text, 'reviews'::text, 'search'::text, 'customer_segments'::text, 'returns_policy'::text, 'inventory_availability'::text, 'shipping_tracking'::text])));

DROP TABLE IF EXISTS public.shipping_method_type_rates;

ALTER TABLE public.shipping_methods
    DROP CONSTRAINT IF EXISTS shipping_methods_units_per_charge_check,
    DROP CONSTRAINT IF EXISTS shipping_methods_delivery_kind_check,
    DROP COLUMN IF EXISTS units_per_charge,
    DROP COLUMN IF EXISTS delivery_kind;

DROP TABLE IF EXISTS public.order_shipments;

ALTER TABLE public.orders
    DROP CONSTRAINT IF EXISTS orders_fulfillment_need_check,
    DROP COLUMN IF EXISTS fulfillment_need;

ALTER TABLE public.carts
    DROP COLUMN IF EXISTS split_delivery,
    DROP COLUMN IF EXISTS deliveries;

ALTER TABLE public.catalog_product_types DROP COLUMN IF EXISTS delivery_kinds;

DROP TABLE IF EXISTS public.delivery_points;
DROP TABLE IF EXISTS public.delivery_options;
