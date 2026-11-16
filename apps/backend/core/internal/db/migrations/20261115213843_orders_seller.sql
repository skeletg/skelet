-- migrate:up
-- Who sold it as the documents settings read when the order was placed, so a later change of address never rewrites an old invoice
ALTER TABLE orders ADD COLUMN seller jsonb;

-- migrate:down
ALTER TABLE orders DROP COLUMN seller;
