-- migrate:up
-- A renewal whose hooks failed waits here rather than holding the head of every billing run's batch
ALTER TABLE subscriptions
    ADD COLUMN renewal_failures integer NOT NULL DEFAULT 0,
    ADD COLUMN renewal_retry_at timestamptz;

-- migrate:down
ALTER TABLE subscriptions
    DROP COLUMN renewal_retry_at,
    DROP COLUMN renewal_failures;
