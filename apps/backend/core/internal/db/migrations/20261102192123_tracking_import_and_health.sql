-- migrate:up
-- The Signals event destination a row was imported from, so importing again finds it instead of copying it twice
ALTER TABLE tracking_destinations ADD COLUMN imported_from uuid;
CREATE UNIQUE INDEX tracking_destinations_imported_from_idx ON tracking_destinations (imported_from) WHERE imported_from IS NOT NULL;

-- A destination's health reads its recent failures; dead rows are few, so a partial index keeps that off the full log
CREATE INDEX tracking_deliveries_dead_idx ON tracking_deliveries (destination_id, created_at DESC) WHERE status = 'dead';

-- migrate:down
DROP INDEX tracking_deliveries_dead_idx;
DROP INDEX tracking_destinations_imported_from_idx;
ALTER TABLE tracking_destinations DROP COLUMN imported_from;
