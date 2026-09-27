-- The share of tax_amount charged on shipping, so line refunds don't hand it back
ALTER TABLE public.orders ADD COLUMN shipping_tax_amount bigint DEFAULT 0 NOT NULL;
