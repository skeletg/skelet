CREATE OR REPLACE FUNCTION public.drop_email_send_month(m date) RETURNS bigint
    LANGUAGE plpgsql
    AS $$
DECLARE
    part text := 'email_sends_' || to_char(m, 'YYYYMM');
    n bigint;
BEGIN
    SELECT count(*) INTO n FROM public.email_sends
    WHERE sent_at >= m AND sent_at < m + interval '1 month';
    IF to_regclass('public.' || part) IS NOT NULL THEN
        EXECUTE format('DROP TABLE public.%I', part);
    END IF;
    DELETE FROM public.email_sends
    WHERE sent_at >= m AND sent_at < m + interval '1 month';
    RETURN n;
END $$;
ALTER FUNCTION public.drop_email_send_month(date) RESET lock_timeout;
ALTER FUNCTION public.ensure_email_send_partitions(integer) RESET lock_timeout;

CREATE OR REPLACE FUNCTION public.ensure_email_send_partitions(months_ahead integer DEFAULT 3) RETURNS void
    LANGUAGE plpgsql
    AS $$
DECLARE
    m date;
BEGIN
    FOR m IN
        SELECT generate_series(date_trunc('month', now() AT TIME ZONE 'UTC'),
                               date_trunc('month', now() AT TIME ZONE 'UTC') + make_interval(months => months_ahead),
                               interval '1 month')::date
    LOOP
        EXECUTE format(
            'CREATE TABLE IF NOT EXISTS public.%I PARTITION OF public.email_sends FOR VALUES FROM (%L) TO (%L)',
            'email_sends_' || to_char(m, 'YYYYMM'), m::text || ' 00:00:00+00', ((m + interval '1 month')::date)::text || ' 00:00:00+00');
    END LOOP;
END $$;

CREATE INDEX idx_customer_segment_memberships_segment ON public.customer_segment_memberships USING btree (segment_id);
DROP INDEX IF EXISTS public.customer_segment_memberships_segment_added;
CREATE INDEX customer_roster_channel ON public.customer_roster USING btree (channel_id);
DROP INDEX IF EXISTS public.customer_roster_channel_order;
DROP INDEX IF EXISTS public.lens_page_snapshots_last_seen;
DROP INDEX IF EXISTS public.browse_notified_last_browse;
DROP INDEX IF EXISTS public.customer_roster_synced;
DROP INDEX IF EXISTS public.email_campaign_recipients_guest_email;
DROP INDEX IF EXISTS public.flow_runs_guest_email;
DROP INDEX IF EXISTS public.flow_runs_finished;
DROP INDEX IF EXISTS public.flow_runs_flow_created;
CREATE INDEX flow_runs_flow_claim ON public.flow_runs USING btree (flow_id, created_at) WHERE (status = ANY (ARRAY['active'::text, 'waiting'::text]));
DROP INDEX IF EXISTS public.flow_runs_flow_due;
DROP INDEX IF EXISTS public.email_campaign_recipients_pending_order;
ALTER TABLE public.email_campaigns DROP COLUMN IF EXISTS materialized_at;
