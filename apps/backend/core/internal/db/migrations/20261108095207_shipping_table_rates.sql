-- migrate:up
-- A table method prices by tiers over the shipment's weight, subtotal or item count
ALTER TABLE shipping_methods
    DROP CONSTRAINT shipping_methods_calc_type_check,
    ADD CONSTRAINT shipping_methods_calc_type_check CHECK (calc_type IN ('flat', 'free', 'free_above', 'table')),
    ADD COLUMN table_basis text NOT NULL DEFAULT 'weight',
    ADD COLUMN rate_tiers jsonb NOT NULL DEFAULT '[]'::jsonb,
    ADD CONSTRAINT shipping_methods_table_basis_check CHECK (table_basis IN ('weight', 'subtotal', 'quantity'));

-- migrate:down
-- A table can't be expressed as a flat price, so it goes back switched off rather than free
UPDATE shipping_methods SET calc_type = 'flat', is_active = false WHERE calc_type = 'table';
ALTER TABLE shipping_methods
    DROP CONSTRAINT shipping_methods_table_basis_check,
    DROP COLUMN rate_tiers,
    DROP COLUMN table_basis,
    DROP CONSTRAINT shipping_methods_calc_type_check,
    ADD CONSTRAINT shipping_methods_calc_type_check CHECK (calc_type IN ('flat', 'free', 'free_above'));
