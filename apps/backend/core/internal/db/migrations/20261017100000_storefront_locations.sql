-- migrate:up
ALTER TABLE inventory_locations ADD COLUMN storefront_visible boolean NOT NULL DEFAULT false;

-- migrate:down
ALTER TABLE inventory_locations DROP COLUMN storefront_visible;
