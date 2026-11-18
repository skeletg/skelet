-- migrate:up
-- What plugins still have to scrub about a customer who was erased or deleted, one row per plugin data set, kept until
-- that plugin's Erase succeeds. No key to customers: a deleted one is gone, and its address has to outlive it here,
-- since rows a plugin keeps by email can't be found any other way. The row goes the moment the plugin has erased
CREATE TABLE plugin_customer_erasures (
    id uuid DEFAULT uuidv7() NOT NULL PRIMARY KEY,
    customer_id uuid NOT NULL,
    channel_id uuid NOT NULL,
    email text DEFAULT '' NOT NULL,
    plugin_slug text NOT NULL,
    data_key text NOT NULL,
    attempts integer DEFAULT 0 NOT NULL,
    last_error text DEFAULT '' NOT NULL,
    next_attempt_at timestamptz DEFAULT now() NOT NULL,
    created_at timestamptz DEFAULT now() NOT NULL,
    UNIQUE (customer_id, plugin_slug, data_key)
);
CREATE INDEX plugin_customer_erasures_due_idx ON plugin_customer_erasures (next_attempt_at);

-- migrate:down
DROP TABLE plugin_customer_erasures;
