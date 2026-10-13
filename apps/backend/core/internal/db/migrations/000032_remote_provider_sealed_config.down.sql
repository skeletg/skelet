ALTER TABLE remote_providers DROP CONSTRAINT remote_providers_kind_check;
ALTER TABLE remote_providers ADD CONSTRAINT remote_providers_kind_check CHECK (kind = ANY (ARRAY['shipping_carrier'::text, 'tax_provider'::text, 'address_validator'::text, 'promotion_condition'::text, 'promotion_action'::text, 'flow_action'::text, 'fraud_risk'::text, 'currency_rate'::text, 'pricing'::text, 'recommendation'::text, 'reviews'::text, 'search'::text, 'customer_segments'::text, 'returns_policy'::text, 'inventory_availability'::text, 'shipping_tracking'::text]));

-- a sealed config has no JSON form without the key, so rolling back drops it - re-enter it after the downgrade
ALTER TABLE remote_providers ALTER COLUMN config TYPE jsonb USING CASE WHEN config LIKE 'enc:%' THEN NULL ELSE config::jsonb END;
