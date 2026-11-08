-- migrate:up
-- The storefront language an order was placed in and a customer last signed in with, so their emails can go out in it
ALTER TABLE orders ADD COLUMN locale text DEFAULT ''::text NOT NULL;
ALTER TABLE customers ADD COLUMN locale text DEFAULT ''::text NOT NULL;

-- migrate:down
ALTER TABLE customers DROP COLUMN locale;
ALTER TABLE orders DROP COLUMN locale;
