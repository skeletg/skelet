-- config carries the merchant's third-party API keys, so it is sealed at rest like the signing secret - ciphertext isn't JSON, so the column becomes text holding either
ALTER TABLE remote_providers ALTER COLUMN config TYPE text USING config::text;

-- flow_action left the code; NOT VALID so an inert legacy row doesn't block the migration while new writes can't use it
ALTER TABLE remote_providers DROP CONSTRAINT remote_providers_kind_check;
ALTER TABLE remote_providers ADD CONSTRAINT remote_providers_kind_check CHECK (kind = ANY (ARRAY['shipping_carrier'::text, 'tax_provider'::text, 'address_validator'::text, 'promotion_condition'::text, 'promotion_action'::text, 'fraud_risk'::text, 'currency_rate'::text, 'pricing'::text, 'recommendation'::text, 'reviews'::text, 'search'::text, 'customer_segments'::text, 'returns_policy'::text, 'inventory_availability'::text, 'shipping_tracking'::text])) NOT VALID;
