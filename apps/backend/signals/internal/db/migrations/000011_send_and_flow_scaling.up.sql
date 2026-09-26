-- Set once every recipient row is written - sending waits for it
ALTER TABLE public.email_campaigns ADD COLUMN materialized_at timestamp with time zone;
UPDATE public.email_campaigns c SET materialized_at = c.updated_at
WHERE EXISTS (SELECT 1 FROM public.email_campaign_recipients r WHERE r.campaign_id = c.id);

-- The send pass pages pending recipients in id order
CREATE INDEX email_campaign_recipients_pending_order ON public.email_campaign_recipients USING btree (campaign_id, id) WHERE status = 'pending';

-- The flow engine claims runs in due order
CREATE INDEX flow_runs_flow_due ON public.flow_runs USING btree (flow_id, (COALESCE(resume_at, CASE WHEN status = 'active' THEN '-infinity'::timestamptz END)))
    WHERE status IN ('active', 'waiting');
DROP INDEX IF EXISTS public.flow_runs_flow_claim;
-- A flow's run history, newest first, and the cascade when a flow is deleted
CREATE INDEX flow_runs_flow_created ON public.flow_runs USING btree (flow_id, created_at DESC);
-- Finished runs age out on the send-history window
CREATE INDEX flow_runs_finished ON public.flow_runs USING btree (updated_at) WHERE status IN ('completed', 'cancelled', 'failed');

-- Guest erasures scrub runs and campaign rows by address
CREATE INDEX flow_runs_guest_email ON public.flow_runs USING btree (lower(email)) WHERE customer_id IS NULL;
CREATE INDEX email_campaign_recipients_guest_email ON public.email_campaign_recipients USING btree (lower(email)) WHERE customer_id IS NULL;

-- Retention sweeps filter on these
CREATE INDEX customer_roster_synced ON public.customer_roster USING btree (synced_at);
CREATE INDEX browse_notified_last_browse ON public.browse_notified USING btree (last_browse_at);
CREATE INDEX lens_page_snapshots_last_seen ON public.lens_page_snapshots USING btree (last_seen_at);

-- A channel's contacts in sync order, read whole by campaign audiences
CREATE INDEX customer_roster_channel_order ON public.customer_roster USING btree (channel_id, synced_at, customer_id);
DROP INDEX IF EXISTS public.customer_roster_channel;

-- A segment's members page newest-joined first
CREATE INDEX customer_segment_memberships_segment_added ON public.customer_segment_memberships USING btree (segment_id, added_at DESC, customer_id);
DROP INDEX IF EXISTS public.idx_customer_segment_memberships_segment;

-- A month whose sends already sit in DEFAULT gets them moved in before it attaches, or CREATE ... PARTITION OF would fail every day
CREATE OR REPLACE FUNCTION public.ensure_email_send_partitions(months_ahead integer DEFAULT 3) RETURNS void
    LANGUAGE plpgsql
    SET lock_timeout TO '5s'
    AS $$
DECLARE
    m date;
    part text;
    lo text;
    hi text;
BEGIN
    FOR m IN
        SELECT generate_series(date_trunc('month', now() AT TIME ZONE 'UTC'),
                               date_trunc('month', now() AT TIME ZONE 'UTC') + make_interval(months => months_ahead),
                               interval '1 month')::date
    LOOP
        part := 'email_sends_' || to_char(m, 'YYYYMM');
        lo := m::text || ' 00:00:00+00';
        hi := ((m + interval '1 month')::date)::text || ' 00:00:00+00';
        CONTINUE WHEN to_regclass('public.' || part) IS NOT NULL;
        IF EXISTS (SELECT 1 FROM public.email_sends_default WHERE sent_at >= lo::timestamptz AND sent_at < hi::timestamptz) THEN
            EXECUTE format('CREATE TABLE public.%I (LIKE public.email_sends INCLUDING DEFAULTS INCLUDING CONSTRAINTS)', part);
            EXECUTE format('WITH moved AS (DELETE FROM public.email_sends_default WHERE sent_at >= %L AND sent_at < %L RETURNING *) INSERT INTO public.%I SELECT * FROM moved', lo, hi, part);
            EXECUTE format('ALTER TABLE public.email_sends ATTACH PARTITION public.%I FOR VALUES FROM (%L) TO (%L)', part, lo, hi);
        ELSE
            EXECUTE format('CREATE TABLE public.%I PARTITION OF public.email_sends FOR VALUES FROM (%L) TO (%L)', part, lo, hi);
        END IF;
    END LOOP;
END $$;

-- Dropping a partition locks the whole send log, so give up rather than queue behind a long reader - the next daily run retries
CREATE OR REPLACE FUNCTION public.drop_email_send_month(m date) RETURNS bigint
    LANGUAGE plpgsql
    SET lock_timeout TO '5s'
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
