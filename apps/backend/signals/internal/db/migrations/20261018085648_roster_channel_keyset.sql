-- migrate:up
-- Campaign audiences page the roster by customer within a channel, not by sync order
DROP INDEX IF EXISTS customer_roster_channel_order;
CREATE INDEX customer_roster_channel_customer ON customer_roster (channel_id, customer_id);

-- migrate:down
DROP INDEX IF EXISTS customer_roster_channel_customer;
CREATE INDEX customer_roster_channel_order ON customer_roster (channel_id, synced_at, customer_id);
