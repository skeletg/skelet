-- migrate:up
-- Each tracking event is stored once, with who it is about in columns of its own, and every destination's delivery points
-- at it - one copy of a visitor's email, IP and click ids instead of one per platform, and an erasure that finds them by
-- index rather than by reading every payload. The name is in the key so a browser event can't take a server event's id
CREATE TABLE public.tracking_events (
    id uuid DEFAULT uuidv7() NOT NULL PRIMARY KEY,
    channel_id uuid NOT NULL REFERENCES public.channels(id) ON DELETE CASCADE,
    event_id text NOT NULL,
    event_name text NOT NULL,
    customer_id uuid,
    anon_id text DEFAULT ''::text NOT NULL,
    email text DEFAULT ''::text NOT NULL, -- trimmed and lower-cased, the form erasure matches on
    payload jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT tracking_events_event_key UNIQUE (channel_id, event_name, event_id)
);
CREATE INDEX tracking_events_customer_idx ON public.tracking_events (channel_id, customer_id) WHERE customer_id IS NOT NULL;
CREATE INDEX tracking_events_email_idx ON public.tracking_events (channel_id, email) WHERE email <> '';
CREATE INDEX tracking_events_anon_idx ON public.tracking_events (channel_id, anon_id) WHERE anon_id <> '';
-- The hourly prune of events whose deliveries are all gone
CREATE INDEX tracking_events_created_idx ON public.tracking_events (created_at);

INSERT INTO public.tracking_events (channel_id, event_id, event_name, customer_id, anon_id, email, payload, created_at)
SELECT DISTINCT ON (t.channel_id, d.event_name, d.event_id)
       t.channel_id, d.event_id, d.event_name,
       NULLIF(d.payload->'user'->>'customer_id', '')::uuid,
       coalesce(d.payload->'user'->>'anon_id', ''),
       lower(btrim(coalesce(d.payload->'user'->>'email', ''))),
       d.payload, d.created_at
FROM   public.tracking_deliveries d
JOIN   public.tracking_destinations t ON t.id = d.destination_id
ORDER  BY t.channel_id, d.event_name, d.event_id, d.created_at;

ALTER TABLE public.tracking_deliveries ADD COLUMN tracking_event_id uuid REFERENCES public.tracking_events(id) ON DELETE CASCADE;
UPDATE public.tracking_deliveries d
SET    tracking_event_id = e.id
FROM   public.tracking_destinations t, public.tracking_events e
WHERE  t.id = d.destination_id AND e.channel_id = t.channel_id AND e.event_name = d.event_name AND e.event_id = d.event_id;
ALTER TABLE public.tracking_deliveries
    ALTER COLUMN tracking_event_id SET NOT NULL,
    DROP CONSTRAINT tracking_deliveries_event_key,
    DROP COLUMN event_id,
    DROP COLUMN event_name,
    DROP COLUMN payload,
    ADD CONSTRAINT tracking_deliveries_event_key UNIQUE (destination_id, tracking_event_id);
CREATE INDEX tracking_deliveries_event_idx ON public.tracking_deliveries (tracking_event_id);

-- migrate:down
ALTER TABLE public.tracking_deliveries
    ADD COLUMN event_id text,
    ADD COLUMN event_name text,
    ADD COLUMN payload jsonb;
UPDATE public.tracking_deliveries d
SET    event_id = e.event_id, event_name = e.event_name, payload = e.payload
FROM   public.tracking_events e
WHERE  e.id = d.tracking_event_id;
DROP INDEX public.tracking_deliveries_event_idx;
ALTER TABLE public.tracking_deliveries
    DROP CONSTRAINT tracking_deliveries_event_key,
    DROP COLUMN tracking_event_id,
    ALTER COLUMN event_id SET NOT NULL,
    ALTER COLUMN event_name SET NOT NULL,
    ALTER COLUMN payload SET NOT NULL,
    ADD CONSTRAINT tracking_deliveries_event_key UNIQUE (destination_id, event_id);
DROP TABLE public.tracking_events;
