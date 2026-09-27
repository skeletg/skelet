CREATE TABLE customer_roster_state (
    one_row boolean PRIMARY KEY DEFAULT true CHECK (one_row),
    filled_version integer NOT NULL DEFAULT 0,
    filled_at timestamp with time zone
);
