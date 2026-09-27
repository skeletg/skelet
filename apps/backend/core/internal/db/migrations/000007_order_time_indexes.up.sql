-- The order list pages newest-first, and the sales reports read created_at ranges
CREATE INDEX IF NOT EXISTS idx_orders_channel_created ON public.orders USING btree (channel_id, created_at DESC);
DROP INDEX IF EXISTS public.idx_orders_channel_id;
CREATE INDEX IF NOT EXISTS idx_orders_created ON public.orders USING btree (created_at DESC);

CREATE INDEX IF NOT EXISTS idx_orders_payment_sync ON public.orders USING btree (updated_at)
    WHERE payment_status = 'unpaid' AND payment_provider <> '' AND payment_reference <> '' AND status <> 'cancelled';
