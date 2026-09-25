DELETE FROM public.trait_compute_runs WHERE finished_at IS NULL;
ALTER TABLE public.trait_compute_runs DROP COLUMN IF EXISTS attempted_at;
ALTER TABLE public.trait_compute_runs ALTER COLUMN finished_at SET NOT NULL;

DROP TABLE IF EXISTS public.channel_roster_tombstones;
