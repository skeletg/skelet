-- migrate:up
-- What the storefront's AI cost and sold, a row per store and UTC day
CREATE TABLE public.assistant_usage_daily (
    channel_id uuid NOT NULL REFERENCES public.channels (id) ON DELETE CASCADE,
    day date NOT NULL,
    turns integer DEFAULT 0 NOT NULL,
    failed_turns integer DEFAULT 0 NOT NULL,
    cart_turns integer DEFAULT 0 NOT NULL,
    checkout_turns integer DEFAULT 0 NOT NULL,
    input_tokens bigint DEFAULT 0 NOT NULL,
    output_tokens bigint DEFAULT 0 NOT NULL,
    cache_read_tokens bigint DEFAULT 0 NOT NULL,
    cache_write_tokens bigint DEFAULT 0 NOT NULL,
    unanswered_turns integer DEFAULT 0 NOT NULL,
    overviews integer DEFAULT 0 NOT NULL,
    PRIMARY KEY (channel_id, day)
);

-- Who helped fill a cart, when it last did, and the AI trace of the turn that did - an order placed from it carries the
-- credit and scores that turn
ALTER TABLE public.carts
    ADD COLUMN assisted_by text DEFAULT '' NOT NULL,
    ADD CONSTRAINT carts_assisted_by_check CHECK (assisted_by IN ('', 'assistant', 'agent')),
    ADD COLUMN assisted_at timestamp with time zone,
    ADD COLUMN assist_trace text DEFAULT '' NOT NULL;

ALTER TABLE public.orders
    ADD COLUMN assisted_by text DEFAULT '' NOT NULL,
    ADD CONSTRAINT orders_assisted_by_check CHECK (assisted_by IN ('', 'assistant', 'agent'));

-- migrate:down
ALTER TABLE public.orders DROP CONSTRAINT orders_assisted_by_check, DROP COLUMN assisted_by;
ALTER TABLE public.carts DROP COLUMN assist_trace, DROP COLUMN assisted_at, DROP CONSTRAINT carts_assisted_by_check, DROP COLUMN assisted_by;
DROP TABLE public.assistant_usage_daily;
