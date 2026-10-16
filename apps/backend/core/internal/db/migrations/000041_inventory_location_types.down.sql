ALTER TABLE inventory_locations DROP CONSTRAINT inventory_locations_pickup_retail_check;
ALTER TABLE inventory_locations DROP CONSTRAINT inventory_locations_public_retail_check;
ALTER TABLE inventory_locations DROP CONSTRAINT inventory_locations_type_check;
ALTER TABLE inventory_locations ALTER COLUMN type SET DEFAULT '';
-- Restore every value up recorded; rows created after rollout keep their type, except unclassified, which was ''.
UPDATE inventory_locations
SET type = metadata->>'_legacy_location_type', metadata = metadata - '_legacy_location_type'
WHERE metadata ? '_legacy_location_type';
UPDATE inventory_locations SET type = '' WHERE type = 'unclassified';
