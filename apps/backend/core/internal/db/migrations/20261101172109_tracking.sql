-- migrate:up
-- Where a channel's shop events go server to server - an analytics or ad platform, one row per configured account. The
-- kind names the code that speaks to it; purposes are the cookie categories an event needs before it may be sent there
CREATE TABLE public.tracking_destinations (
    id uuid DEFAULT uuidv7() NOT NULL PRIMARY KEY,
    channel_id uuid NOT NULL REFERENCES public.channels(id) ON DELETE CASCADE,
    kind text NOT NULL,
    name text NOT NULL,
    enabled boolean DEFAULT true NOT NULL,
    purposes text[] DEFAULT '{}'::text[] NOT NULL,
    events text[] DEFAULT '{}'::text[] NOT NULL,
    config jsonb DEFAULT '{}'::jsonb NOT NULL,
    secret text DEFAULT ''::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);
CREATE INDEX tracking_destinations_channel_idx ON public.tracking_destinations (channel_id) WHERE enabled;

-- One event bound for one destination: the queue while pending, the delivery log once sent or dead. The unique key makes
-- a replayed event a no-op, and one failing event never holds up the rest of its destination
CREATE TABLE public.tracking_deliveries (
    id uuid DEFAULT uuidv7() NOT NULL PRIMARY KEY,
    destination_id uuid NOT NULL REFERENCES public.tracking_destinations(id) ON DELETE CASCADE,
    event_id text NOT NULL,
    event_name text NOT NULL,
    payload jsonb NOT NULL,
    status text DEFAULT 'pending'::text NOT NULL,
    attempts integer DEFAULT 0 NOT NULL,
    next_attempt_at timestamp with time zone DEFAULT now() NOT NULL,
    last_error text DEFAULT ''::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT tracking_deliveries_status_check CHECK (status = ANY (ARRAY['pending'::text, 'sent'::text, 'dead'::text])),
    CONSTRAINT tracking_deliveries_event_key UNIQUE (destination_id, event_id)
);
CREATE INDEX tracking_deliveries_due_idx ON public.tracking_deliveries (next_attempt_at) WHERE status = 'pending';
CREATE INDEX tracking_deliveries_log_idx ON public.tracking_deliveries (destination_id, created_at DESC);

-- migrate:down
DROP TABLE public.tracking_deliveries;
DROP TABLE public.tracking_destinations;
