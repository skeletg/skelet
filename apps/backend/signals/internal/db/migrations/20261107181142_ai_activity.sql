-- migrate:up
-- What the shopping assistant and shoppers' own AI agents did, counted per store and day - never a person
CREATE TABLE public.ai_activity_seen (
    dedupe_key text PRIMARY KEY,
    seen_at timestamp with time zone DEFAULT now() NOT NULL
);
CREATE INDEX idx_ai_activity_seen_at ON public.ai_activity_seen (seen_at);

CREATE TABLE public.ai_tool_daily (
    channel_id uuid NOT NULL,
    day date NOT NULL,
    source text NOT NULL,
    agent text NOT NULL,
    tool text NOT NULL,
    calls bigint NOT NULL,
    PRIMARY KEY (channel_id, day, source, agent, tool),
    CONSTRAINT ai_tool_daily_source_check CHECK (source IN ('assistant', 'agent'))
);
CREATE INDEX idx_ai_tool_daily_day ON public.ai_tool_daily (day);

CREATE TABLE public.ai_search_daily (
    channel_id uuid NOT NULL,
    day date NOT NULL,
    source text NOT NULL,
    agent text NOT NULL,
    term text NOT NULL,
    searches bigint NOT NULL,
    empty bigint NOT NULL,
    PRIMARY KEY (channel_id, day, source, agent, term),
    CONSTRAINT ai_search_daily_source_check CHECK (source IN ('assistant', 'agent'))
);
CREATE INDEX idx_ai_search_daily_day ON public.ai_search_daily (day);

-- migrate:down
DROP TABLE IF EXISTS public.ai_search_daily;
DROP TABLE IF EXISTS public.ai_tool_daily;
DROP TABLE IF EXISTS public.ai_activity_seen;
