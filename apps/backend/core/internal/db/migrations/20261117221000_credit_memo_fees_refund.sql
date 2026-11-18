-- migrate:up
-- What a credit memo refunds of the fees plugins charged, apart from the goods, so loyalty and analytics don't read a
-- refunded handling fee as returned merchandise
ALTER TABLE credit_memos ADD COLUMN fees_refund_cents bigint DEFAULT 0 NOT NULL CHECK (fees_refund_cents >= 0);

-- migrate:down
ALTER TABLE credit_memos DROP COLUMN fees_refund_cents;
