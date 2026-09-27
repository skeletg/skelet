-- The visitor id the order was placed under, so a guest's later order events reach the same anonymous profile its placement did
ALTER TABLE public.orders ADD COLUMN anon_id text DEFAULT '' NOT NULL;
