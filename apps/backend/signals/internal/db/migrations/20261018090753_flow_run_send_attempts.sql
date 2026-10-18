-- migrate:up
-- A failed send keeps the run on its node and retries with backoff - the count is what ends a run that never gets through
ALTER TABLE flow_runs ADD COLUMN send_attempts integer NOT NULL DEFAULT 0;

-- migrate:down
ALTER TABLE flow_runs DROP COLUMN send_attempts;
