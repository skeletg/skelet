-- A memo is written before its refund moves money - a declined refund left it counting as credited, so the units could never be refunded again
ALTER TABLE credit_memos ADD COLUMN status text NOT NULL DEFAULT 'issued'
    CONSTRAINT credit_memos_status_check CHECK (status IN ('issued', 'failed'));
