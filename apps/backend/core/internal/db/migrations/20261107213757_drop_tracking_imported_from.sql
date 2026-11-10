-- migrate:up
-- The suite's event destinations are retired, so nothing is imported from them any more
DROP INDEX tracking_destinations_imported_from_idx;
ALTER TABLE tracking_destinations DROP COLUMN imported_from;

-- migrate:down
ALTER TABLE tracking_destinations ADD COLUMN imported_from uuid;
CREATE UNIQUE INDEX tracking_destinations_imported_from_idx ON tracking_destinations (imported_from) WHERE imported_from IS NOT NULL;
