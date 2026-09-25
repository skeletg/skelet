DROP INDEX IF EXISTS public.idx_orders_payment_sync;
DROP INDEX IF EXISTS public.idx_orders_created;
CREATE INDEX IF NOT EXISTS idx_orders_channel_id ON public.orders USING btree (channel_id);
DROP INDEX IF EXISTS public.idx_orders_channel_created;
