-- migrate:up
CREATE UNIQUE INDEX tracking_catalog_pending_idx ON tracking_deliveries (destination_id)
WHERE status = 'pending' AND event_name = 'catalog_snapshot';


-- migrate:down
DROP INDEX tracking_catalog_pending_idx;
