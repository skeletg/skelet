-- migrate:up
-- A custom offer line has no product to take a tax class from - the order keeps the one the admin gave it, so an edit re-taxes it the same way placement did
ALTER TABLE public.order_lines ADD COLUMN tax_class text DEFAULT '' NOT NULL;

-- migrate:down
ALTER TABLE public.order_lines DROP COLUMN IF EXISTS tax_class;
