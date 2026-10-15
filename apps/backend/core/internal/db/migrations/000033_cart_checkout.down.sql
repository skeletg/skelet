DROP INDEX IF EXISTS public.idx_carts_shipping_address_id;
DROP INDEX IF EXISTS public.idx_carts_billing_address_id;

ALTER TABLE public.carts
    DROP COLUMN IF EXISTS shipping_address,
    DROP COLUMN IF EXISTS shipping_address_id,
    DROP COLUMN IF EXISTS billing_same_as_shipping,
    DROP COLUMN IF EXISTS billing_address,
    DROP COLUMN IF EXISTS billing_address_id,
    DROP COLUMN IF EXISTS shipping_carrier_slug,
    DROP COLUMN IF EXISTS shipping_method_code,
    DROP COLUMN IF EXISTS payment_provider_slug,
    DROP COLUMN IF EXISTS po_number,
    DROP COLUMN IF EXISTS use_store_credit,
    DROP COLUMN IF EXISTS gift_card_codes,
    DROP COLUMN IF EXISTS destination_country,
    DROP COLUMN IF EXISTS destination_region,
    DROP COLUMN IF EXISTS destination_postal_code,
    DROP COLUMN IF EXISTS contact_phone,
    DROP COLUMN IF EXISTS purchase_type,
    DROP COLUMN IF EXISTS company_name,
    DROP COLUMN IF EXISTS business_ids,
    DROP COLUMN IF EXISTS pickup_contact,
    DROP COLUMN IF EXISTS checkout_version;

ALTER TABLE public.orders DROP COLUMN IF EXISTS business_ids;

ALTER TABLE public.countries DROP COLUMN IF EXISTS business_id_fields;
