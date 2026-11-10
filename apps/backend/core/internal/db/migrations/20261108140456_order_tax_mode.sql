-- migrate:up
-- Whether an order's prices held their tax, as its channel priced it when it was placed - an edit, a refund or an
-- invoice reads the order's own mode, never the channel's current one
ALTER TABLE orders ADD COLUMN tax_inclusive boolean DEFAULT false NOT NULL;

-- migrate:down
ALTER TABLE orders DROP COLUMN tax_inclusive;
