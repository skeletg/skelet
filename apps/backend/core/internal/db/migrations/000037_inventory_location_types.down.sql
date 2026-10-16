ALTER TABLE inventory_locations DROP CONSTRAINT inventory_locations_pickup_retail_check;
ALTER TABLE inventory_locations DROP CONSTRAINT inventory_locations_public_retail_check;
ALTER TABLE inventory_locations DROP CONSTRAINT inventory_locations_type_check;
ALTER TABLE inventory_locations ALTER COLUMN type SET DEFAULT '';
-- Restore unknown legacy classifiers; keep intentional classifications made after rollout.
UPDATE inventory_locations SET type = COALESCE(metadata->>'_legacy_location_type', '') WHERE type = 'unclassified';
