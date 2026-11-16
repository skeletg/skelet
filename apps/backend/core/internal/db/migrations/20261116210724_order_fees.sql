-- migrate:up
-- The fees plugins charged at placement, each with the label the shopper saw and the tax it carried, so the total
-- adds up from what the order keeps rather than from what a plugin would charge today
ALTER TABLE orders ADD COLUMN fees jsonb NOT NULL DEFAULT '[]';

-- migrate:down
ALTER TABLE orders DROP COLUMN fees;
