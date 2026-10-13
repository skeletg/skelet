-- How long a built-in method takes, in delivery days - 0 gives no estimate. The arrival date checkout shows counts these from tomorrow, skipping Saturday and Sunday unless the method delivers on weekends
ALTER TABLE public.shipping_methods
    ADD COLUMN estimated_days integer DEFAULT 0 NOT NULL,
    ADD COLUMN delivers_on_weekends boolean DEFAULT false NOT NULL,
    ADD CONSTRAINT shipping_methods_estimated_days_check CHECK (estimated_days >= 0);

-- The same weekend rule for a registered carrier's quotes, which carry days but no calendar - the built-in carrier's methods keep their own
ALTER TABLE public.shipping_carriers
    ADD COLUMN delivers_on_weekends boolean DEFAULT false NOT NULL;
