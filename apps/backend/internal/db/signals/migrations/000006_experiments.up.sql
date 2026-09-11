--
-- Prism experiments - the definitions the storefront SDK assigns visitors against
--
-- Signals owns every row: it writes them from its own admin API and reads them beside the exposure
-- and goal events on the spine, which is the other half of every results query
--
-- The channel foreign key does not come along, since channels are core's and a key cannot cross
-- databases - an experiment on a deleted channel is inert, since assignment and listing are both
-- channel-scoped and neither will ever name it
--
CREATE TABLE public.experiments (
    id uuid DEFAULT uuidv7() NOT NULL,
    channel_id uuid NOT NULL,
    key text NOT NULL,
    name text NOT NULL,
    status text DEFAULT 'draft'::text NOT NULL,
    coverage integer DEFAULT 100 NOT NULL,
    goal_event text DEFAULT ''::text NOT NULL,
    variations jsonb DEFAULT '[]'::jsonb NOT NULL,
    segment_slug text DEFAULT ''::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

ALTER TABLE ONLY public.experiments
    ADD CONSTRAINT experiments_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.experiments
    ADD CONSTRAINT experiments_channel_id_key_key UNIQUE (channel_id, key);
