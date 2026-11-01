-- migrate:up
-- A marketing opt-in given under an email with no account behind it - a guest's tick at checkout. It is claimed onto the
-- customer once someone proves they own the address, and until then kept as the proof: what, when, where and the order
CREATE TABLE public.email_opt_ins (
    channel_id uuid NOT NULL REFERENCES public.channels(id) ON DELETE CASCADE,
    email text NOT NULL,
    kind text NOT NULL,
    source text NOT NULL,
    reference text DEFAULT ''::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    PRIMARY KEY (channel_id, email, kind)
);

-- migrate:down
DROP TABLE public.email_opt_ins;
