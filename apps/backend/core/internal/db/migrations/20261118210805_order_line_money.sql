-- migrate:up
-- Each line carries its own share of the order's discount and its own tax, written when the order is priced - a refund,
-- a report and an edit read them rather than working the shares out again from the order's totals
ALTER TABLE public.order_lines ADD COLUMN tax_cents bigint DEFAULT 0 NOT NULL;
ALTER TABLE public.order_lines ADD COLUMN "position" integer DEFAULT 0 NOT NULL;
ALTER TABLE public.order_lines ADD CONSTRAINT order_lines_money_check CHECK (discount_cents >= 0 AND tax_cents >= 0);

-- migrate:down
ALTER TABLE public.order_lines DROP CONSTRAINT IF EXISTS order_lines_money_check;
ALTER TABLE public.order_lines DROP COLUMN IF EXISTS "position";
ALTER TABLE public.order_lines DROP COLUMN IF EXISTS tax_cents;
