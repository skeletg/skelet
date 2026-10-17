-- migrate:up
CREATE INDEX idx_carts_order ON public.carts (order_id) WHERE order_id IS NOT NULL;

-- migrate:down
DROP INDEX public.idx_carts_order;
