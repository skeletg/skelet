-- Log storage for the Vector collector. Its own database on the ClickHouse
-- server, deliberately NOT the `signals` one: the server is shared
-- infrastructure and each consumer owns its schema.
--
-- Applied by the `logs-schema` one-shot, idempotent so it can re-run on
-- every `up`. Retention is the TTL, not a prune job - ClickHouse drops
-- expired parts on merge with no work from us.
CREATE DATABASE IF NOT EXISTS logs;

CREATE TABLE IF NOT EXISTS logs.container_logs
(
    timestamp DateTime64(3, 'UTC'),  -- explicit, so the collector's UTC strings mean what they say
    container LowCardinality(String),
    stream    LowCardinality(String),  -- stdout | stderr
    level     LowCardinality(String),
    message   String,
    source    String,                  -- file:line, which applog stamps on every record
    raw       String                   -- the untouched line, so a parse miss loses nothing
)
ENGINE = MergeTree
PARTITION BY toDate(timestamp)
-- container first: "show me the app's errors last hour" is the query this exists for
ORDER BY (container, timestamp)
TTL toDateTime(timestamp) + INTERVAL 30 DAY;
