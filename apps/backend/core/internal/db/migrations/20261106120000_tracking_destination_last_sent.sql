-- migrate:up
-- A sent delivery is deleted, so a destination's last success is kept on the destination itself
ALTER TABLE tracking_destinations ADD COLUMN last_sent_at timestamptz;

-- migrate:down
ALTER TABLE tracking_destinations DROP COLUMN last_sent_at;
