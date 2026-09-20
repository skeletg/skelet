-- Exercises the plugin-migrate runner end to end - the table itself carries nothing
CREATE TABLE example_plugin_records (
    id         BIGSERIAL PRIMARY KEY,
    note       TEXT        NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
