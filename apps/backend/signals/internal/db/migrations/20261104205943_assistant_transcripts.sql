-- migrate:up
CREATE EXTENSION IF NOT EXISTS pg_trgm;

CREATE TABLE public.assistant_conversations (
    id uuid PRIMARY KEY,
    channel_id uuid NOT NULL,
    customer_id uuid,
    anon_id text DEFAULT '' NOT NULL,
    last_at timestamp with time zone NOT NULL
);
CREATE INDEX idx_assistant_conversations_channel ON public.assistant_conversations (channel_id, last_at DESC, id DESC);
CREATE INDEX idx_assistant_conversations_last ON public.assistant_conversations (last_at);
CREATE INDEX idx_assistant_conversations_customer ON public.assistant_conversations (customer_id) WHERE customer_id IS NOT NULL;
CREATE INDEX idx_assistant_conversations_anon ON public.assistant_conversations (anon_id) WHERE anon_id <> '';

CREATE TABLE public.assistant_turns (
    id uuid PRIMARY KEY,
    conversation_id uuid NOT NULL REFERENCES public.assistant_conversations (id) ON DELETE CASCADE,
    customer_id uuid,
    page text DEFAULT '' NOT NULL,
    question text NOT NULL,
    reply text DEFAULT '' NOT NULL,
    recap text DEFAULT '' NOT NULL,
    follow_ups text[] DEFAULT '{}' NOT NULL,
    rounds jsonb DEFAULT '[]'::jsonb NOT NULL,
    products integer DEFAULT 0 NOT NULL,
    cart_changed boolean DEFAULT false NOT NULL,
    checkout boolean DEFAULT false NOT NULL,
    incomplete boolean DEFAULT false NOT NULL,
    found_nothing boolean DEFAULT false NOT NULL,
    unsure boolean DEFAULT false NOT NULL,
    model text DEFAULT '' NOT NULL,
    input_tokens bigint DEFAULT 0 NOT NULL,
    output_tokens bigint DEFAULT 0 NOT NULL,
    cache_read_tokens bigint DEFAULT 0 NOT NULL,
    cache_write_tokens bigint DEFAULT 0 NOT NULL,
    at timestamp with time zone NOT NULL
);
CREATE INDEX idx_assistant_turns_conversation ON public.assistant_turns (conversation_id, at);
CREATE INDEX idx_assistant_turns_at ON public.assistant_turns (at);
CREATE INDEX idx_assistant_turns_customer ON public.assistant_turns (customer_id) WHERE customer_id IS NOT NULL;
CREATE INDEX idx_assistant_turns_question_trgm ON public.assistant_turns USING gin (question gin_trgm_ops);
CREATE INDEX idx_assistant_turns_reply_trgm ON public.assistant_turns USING gin (reply gin_trgm_ops);
-- the turns the store couldn't answer, newest first per conversation - its latest one is a single probe
CREATE INDEX idx_assistant_turns_unanswered ON public.assistant_turns (conversation_id, at DESC, id DESC)
WHERE incomplete OR found_nothing OR unsure OR (reply = '' AND recap = '');

CREATE VIEW public.assistant_conversation_summaries AS
SELECT c.id, c.channel_id, c.customer_id, c.anon_id, c.last_at,
    f.question AS first_question, f.at AS started_at,
    t.turns, t.cart_changed, t.checkout, t.incomplete
FROM public.assistant_conversations c
CROSS JOIN LATERAL (
    SELECT question, at FROM public.assistant_turns WHERE conversation_id = c.id ORDER BY at, id LIMIT 1
) f
CROSS JOIN LATERAL (
    SELECT count(*)::integer AS turns,
        COALESCE(bool_or(cart_changed), false)::boolean AS cart_changed,
        COALESCE(bool_or(checkout), false)::boolean AS checkout,
        COALESCE(bool_or(incomplete), false)::boolean AS incomplete
    FROM public.assistant_turns WHERE conversation_id = c.id
) t;

-- migrate:down
DROP VIEW public.assistant_conversation_summaries;
DROP TABLE public.assistant_turns;
DROP TABLE public.assistant_conversations;
