ALTER TABLE public.customer_roster ADD COLUMN source_read_at timestamp with time zone;

CREATE TABLE public.customer_roster_tombstones (
    customer_id uuid PRIMARY KEY,
    deleted_at timestamp with time zone NOT NULL DEFAULT now()
);
