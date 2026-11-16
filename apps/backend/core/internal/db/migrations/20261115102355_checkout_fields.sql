-- migrate:up
-- What the shopper answered to the plugins' checkout fields, by key - the order keeps a snapshot with each field's label
ALTER TABLE carts ADD COLUMN checkout_fields jsonb NOT NULL DEFAULT '{}'::jsonb;
ALTER TABLE orders ADD COLUMN checkout_fields jsonb NOT NULL DEFAULT '[]'::jsonb;

-- migrate:down
ALTER TABLE orders DROP COLUMN checkout_fields;
ALTER TABLE carts DROP COLUMN checkout_fields;
