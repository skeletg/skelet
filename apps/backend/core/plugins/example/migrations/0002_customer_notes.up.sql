-- Linked to core's customers the way a plugin links its data to a core record: a foreign key that takes the rows with a
-- deleted customer. An erasure keeps the customer's row, so the plugin's RegisterCustomerData scrubs these then
CREATE TABLE example_plugin_customer_notes (
    id          BIGSERIAL PRIMARY KEY,
    customer_id UUID        NOT NULL REFERENCES customers (id) ON DELETE CASCADE,
    note        TEXT        NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX example_plugin_customer_notes_customer ON example_plugin_customer_notes (customer_id);
