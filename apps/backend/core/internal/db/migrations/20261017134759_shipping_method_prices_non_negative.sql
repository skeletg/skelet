-- migrate:up
ALTER TABLE shipping_methods
    ADD CONSTRAINT shipping_methods_flat_cents_check CHECK (flat_cents >= 0),
    ADD CONSTRAINT shipping_methods_free_above_cents_check CHECK (free_above_cents >= 0);

-- migrate:down
ALTER TABLE shipping_methods
    DROP CONSTRAINT shipping_methods_flat_cents_check,
    DROP CONSTRAINT shipping_methods_free_above_cents_check;
