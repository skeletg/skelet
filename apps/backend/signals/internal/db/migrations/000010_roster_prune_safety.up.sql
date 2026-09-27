CREATE TABLE public.channel_roster_tombstones (
    channel_id uuid PRIMARY KEY,
    pruned_at timestamp with time zone NOT NULL DEFAULT now()
);

ALTER TABLE public.trait_compute_runs ALTER COLUMN finished_at DROP NOT NULL;
ALTER TABLE public.trait_compute_runs ADD COLUMN attempted_at timestamp with time zone;
