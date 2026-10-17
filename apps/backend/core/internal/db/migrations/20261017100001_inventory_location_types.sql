-- migrate:up
-- Preserve free-form classifiers for operators reviewing legacy locations, and every value
-- this migration changes (an untyped pickup point becomes retail below) so down can restore it.
UPDATE inventory_locations
SET metadata = jsonb_build_object('_legacy_location_type', type) || metadata
WHERE type <> '' OR pickup_enabled;

UPDATE inventory_locations SET type = CASE lower(replace(replace(trim(type), '_', '-'), ' ', '-'))
    WHEN 'retail' THEN 'retail' WHEN 'retail-store' THEN 'retail' WHEN 'store' THEN 'retail' WHEN 'shop' THEN 'retail'
    WHEN 'warehouse' THEN 'warehouse'
    WHEN 'storage' THEN 'storage'
    WHEN 'dropshipper' THEN 'drop-shipper' WHEN 'drop-shipper' THEN 'drop-shipper'
    WHEN 'fulfillment-partner' THEN 'fulfillment-center' WHEN 'fulfillment-center' THEN 'fulfillment-center'
    -- Pickup never needed a type before, so an unknown-typed pickup point is a store: keep click-and-collect
    ELSE CASE WHEN pickup_enabled THEN 'retail' ELSE 'unclassified' END END;

UPDATE inventory_locations SET storefront_visible = false, pickup_enabled = false
WHERE type <> 'retail';
ALTER TABLE inventory_locations ALTER COLUMN type SET DEFAULT 'unclassified';
ALTER TABLE inventory_locations ADD CONSTRAINT inventory_locations_type_check
    CHECK (type IN ('retail', 'warehouse', 'storage', 'drop-shipper', 'fulfillment-center', 'unclassified'));
ALTER TABLE inventory_locations ADD CONSTRAINT inventory_locations_public_retail_check
    CHECK (NOT storefront_visible OR type = 'retail');
ALTER TABLE inventory_locations ADD CONSTRAINT inventory_locations_pickup_retail_check
    CHECK (NOT pickup_enabled OR type = 'retail');

-- migrate:down
ALTER TABLE inventory_locations DROP CONSTRAINT inventory_locations_pickup_retail_check;
ALTER TABLE inventory_locations DROP CONSTRAINT inventory_locations_public_retail_check;
ALTER TABLE inventory_locations DROP CONSTRAINT inventory_locations_type_check;
ALTER TABLE inventory_locations ALTER COLUMN type SET DEFAULT '';
-- Restore every value up recorded; rows created after rollout keep their type, except unclassified, which was ''.
UPDATE inventory_locations
SET type = metadata->>'_legacy_location_type', metadata = metadata - '_legacy_location_type'
WHERE metadata ? '_legacy_location_type';
UPDATE inventory_locations SET type = '' WHERE type = 'unclassified';
