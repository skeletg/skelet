-- Flat mode keeps no ledger, so a replayed cancel had nothing to tell it the stock already went back - one row per restored order makes the restore once-only
CREATE TABLE inventory_flat_restores (
    order_id   uuid PRIMARY KEY REFERENCES orders(id) ON DELETE CASCADE,
    reason     text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);
