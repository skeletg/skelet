-- Recreates the send log here, empty. Rows written since the move live in the commerce
-- database and do not come back with this.

CREATE FUNCTION public.ensure_email_send_partitions(months_ahead integer DEFAULT 3) RETURNS void
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

CREATE FUNCTION public.email_send_months_before(cutoff timestamp with time zone) RETURNS TABLE(month date)
    LANGUAGE plpgsql STABLE
    AS $_$
BEGIN
    RETURN QUERY
    SELECT DISTINCT m FROM (
        SELECT to_date(substring(c.relname FROM '\d{6}$'), 'YYYYMM') AS m
        FROM pg_inherits i
        JOIN pg_class c ON c.oid = i.inhrelid
        WHERE i.inhparent = 'public.email_sends'::regclass
          AND c.relname ~ '^email_sends_\d{6}$'
        UNION ALL
        SELECT DISTINCT date_trunc('month', sent_at)::date
        FROM public.email_sends_default
    ) months
    WHERE m + interval '1 month' <= cutoff
    ORDER BY 1;
END $_$;

CREATE FUNCTION public.drop_email_send_month(m date) RETURNS bigint
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

CREATE TABLE public.email_sends (
    id uuid DEFAULT uuidv7() NOT NULL,
    channel_id uuid NOT NULL,
    source text NOT NULL,
    customer_id uuid,
    email text NOT NULL,
    campaign_id uuid,
    recipient_id uuid,
    variant text DEFAULT ''::text NOT NULL,
    flow_id uuid,
    flow_run_id uuid,
    flow_node_id text DEFAULT ''::text NOT NULL,
    message_key text DEFAULT ''::text NOT NULL,
    order_id uuid,
    sent_at timestamp with time zone DEFAULT now() NOT NULL,
    first_open_at timestamp with time zone,
    first_click_at timestamp with time zone,
    PRIMARY KEY (id, sent_at)
)
PARTITION BY RANGE (sent_at);

CREATE TABLE public.email_sends_default PARTITION OF public.email_sends DEFAULT;

CREATE INDEX email_sends_customer ON public.email_sends USING btree (customer_id, sent_at DESC) WHERE (customer_id IS NOT NULL);
CREATE INDEX email_sends_campaign_variant ON public.email_sends USING btree (campaign_id, variant) WHERE (campaign_id IS NOT NULL);
CREATE INDEX email_sends_email ON public.email_sends USING btree (email);

SELECT public.ensure_email_send_partitions(3);
