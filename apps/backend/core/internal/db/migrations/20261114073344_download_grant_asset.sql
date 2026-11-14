-- migrate:up
-- A grant can hand a customer a library file directly, with no product or order behind it - one per customer and file
ALTER TABLE download_grants
    ADD COLUMN asset_id uuid REFERENCES media_assets(id) ON DELETE CASCADE,
    ALTER COLUMN product_id DROP NOT NULL,
    ALTER COLUMN download_id DROP NOT NULL,
    ADD CONSTRAINT download_grants_source_check CHECK (
        (asset_id IS NOT NULL AND product_id IS NULL AND download_id IS NULL)
        OR (asset_id IS NULL AND product_id IS NOT NULL AND download_id IS NOT NULL)
    );
CREATE UNIQUE INDEX download_grants_asset_customer_key ON download_grants (asset_id, customer_id) WHERE asset_id IS NOT NULL;

-- migrate:down
DELETE FROM download_grants WHERE asset_id IS NOT NULL;
DROP INDEX download_grants_asset_customer_key;
ALTER TABLE download_grants
    DROP CONSTRAINT download_grants_source_check,
    DROP COLUMN asset_id,
    ALTER COLUMN product_id SET NOT NULL,
    ALTER COLUMN download_id SET NOT NULL;
