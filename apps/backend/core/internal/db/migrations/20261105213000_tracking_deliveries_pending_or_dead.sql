-- migrate:up
-- A sent delivery is deleted, so a row is only ever waiting or dead - the log is what still needs attention
DELETE FROM public.tracking_deliveries WHERE status = 'sent';
ALTER TABLE public.tracking_deliveries
    DROP CONSTRAINT tracking_deliveries_status_check,
    ADD CONSTRAINT tracking_deliveries_status_check CHECK (status = ANY (ARRAY['pending'::text, 'dead'::text]));
DROP INDEX public.tracking_deliveries_prune_idx;
-- The hourly prune of dead letters, which would otherwise read the whole queue
CREATE INDEX tracking_deliveries_prune_idx ON public.tracking_deliveries (updated_at) WHERE status = 'dead';

-- migrate:down
DROP INDEX public.tracking_deliveries_prune_idx;
CREATE INDEX tracking_deliveries_prune_idx ON public.tracking_deliveries (updated_at) WHERE status <> 'pending';
ALTER TABLE public.tracking_deliveries
    DROP CONSTRAINT tracking_deliveries_status_check,
    ADD CONSTRAINT tracking_deliveries_status_check CHECK (status = ANY (ARRAY['pending'::text, 'sent'::text, 'dead'::text]));
