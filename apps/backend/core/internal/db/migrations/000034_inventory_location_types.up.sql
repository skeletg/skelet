-- Preserve free-form classifiers for operators reviewing legacy locations.
UPDATE inventory_locations
SET metadata = jsonb_build_object('_legacy_location_type', type) || metadata
WHERE type <> '';

UPDATE inventory_locations SET type = CASE lower(replace(replace(trim(type), '_', '-'), ' ', '-'))
    WHEN 'retail' THEN 'retail' WHEN 'retail-store' THEN 'retail' WHEN 'store' THEN 'retail' WHEN 'shop' THEN 'retail'
    WHEN 'warehouse' THEN 'warehouse'
    WHEN 'storage' THEN 'storage'
    WHEN 'dropshipper' THEN 'drop-shipper' WHEN 'drop-shipper' THEN 'drop-shipper'
    WHEN 'fulfillment-partner' THEN 'fulfillment-center' WHEN 'fulfillment-center' THEN 'fulfillment-center'
    ELSE 'unclassified' END;

UPDATE inventory_locations SET storefront_visible = false, pickup_enabled = false
WHERE type <> 'retail';
ALTER TABLE inventory_locations ALTER COLUMN type SET DEFAULT 'unclassified';
ALTER TABLE inventory_locations ADD CONSTRAINT inventory_locations_type_check
    CHECK (type IN ('retail', 'warehouse', 'storage', 'drop-shipper', 'fulfillment-center', 'unclassified'));
ALTER TABLE inventory_locations ADD CONSTRAINT inventory_locations_public_retail_check
    CHECK (NOT storefront_visible OR type = 'retail');
ALTER TABLE inventory_locations ADD CONSTRAINT inventory_locations_pickup_retail_check
    CHECK (NOT pickup_enabled OR type = 'retail');
