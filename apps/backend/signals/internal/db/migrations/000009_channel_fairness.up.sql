CREATE TABLE public.trait_compute_runs (
    channel_id uuid PRIMARY KEY,
    finished_at timestamp with time zone NOT NULL
);

CREATE INDEX flow_runs_flow_claim ON public.flow_runs USING btree (flow_id, created_at) WHERE (status = ANY (ARRAY['active'::text, 'waiting'::text]));
