-- migrate:up
ALTER TABLE orders ADD COLUMN stage text NOT NULL DEFAULT '';
CREATE INDEX orders_stage_idx ON orders (stage, created_at DESC) WHERE stage <> '';

-- migrate:down
DROP INDEX orders_stage_idx;
ALTER TABLE orders DROP COLUMN stage;
