-- ClickHouse spine schema, applied by the clickhouse-migrate one-shot:
--   migrate -path /app/clickhouse-migrations -database "$SIGNALS_CH_MIGRATE_URL" up
-- NOTE: no semicolons in comments here. The migrate clickhouse driver splits
-- multi-statement files on the statement separator without parsing comments, so one
-- inside a comment cuts the file and the leading fragment fails as an empty query.
-- The database itself comes from the server's CLICKHOUSE_DB, so this only makes
-- tables. Optional UUID columns use the zero UUID as "none" (cheaper than
-- Nullable(UUID)) and the reads treat zero as null.

-- analytics_events: the append-only spine.
--
-- ORDER BY (channel_id, dedupe_key) makes the sort key the dedup identity, so a
-- ReplacingMergeTree collapse means the same thing Postgres's UNIQUE(dedupe_key)
-- does. occurred_at stays OUT of the sort key so a replay whose timestamp drifted
-- a little would still collapse - and every dedupe-stable producer keeps it fixed
-- anyway: outbox redeliveries carry the row's created_at (events.EmittedAt), web
-- retransmits the client timestamp, pixels the minute-bucketed hit time.
--
-- ReplacingMergeTree only dedups WITHIN a partition, so the partition granularity
-- is the dedup window - daily, which covers replays because occurred_at is stable
-- per dedupe key (see above), while keeping time-range pruning (the sort key no
-- longer provides it) and making retention a cheap DROP PARTITION, not a mutation.
--
-- Collapse is eventual (background merge), so every read goes through FINAL - the
-- driver sets do_not_merge_across_partitions_select_final, which is sound here
-- because a stable occurred_at pins duplicates to one partition.
CREATE TABLE IF NOT EXISTS analytics_events (
    id          UUID DEFAULT generateUUIDv7(),
    event_name  LowCardinality(String),
    occurred_at DateTime64(3, 'UTC'),
    received_at DateTime64(3, 'UTC') DEFAULT now64(3),
    channel_id  UUID,
    customer_id UUID,
    anon_id     String,
    source      LowCardinality(String),
    send_id     UUID,
    properties  String,
    dedupe_key  String,
    created_at  DateTime64(3, 'UTC') DEFAULT now64(3),
    -- The property keys the reads touch constantly, promoted to real columns.
    -- MATERIALIZED computes them at insert, so a filter reads one compressed
    -- column instead of parsing the whole JSON blob per row - is_bot alone is in
    -- the WHERE of nearly every query. The blob stays authoritative and keeps the
    -- long tail, including the dynamic attr:<slug> keys, which can't be columns.
    -- Adding one later is safe: ClickHouse computes it on read for existing parts,
    -- and ALTER TABLE ... MATERIALIZE COLUMN backfills it for speed.
    is_bot        Bool   MATERIALIZED JSONExtractBool(properties, 'is_bot'),
    machine       Bool   MATERIALIZED JSONExtractBool(properties, 'machine'),
    title         String MATERIALIZED JSONExtractString(properties, 'title'),
    page_key      String MATERIALIZED JSONExtractString(properties, 'page_key'),
    currency      String MATERIALIZED JSONExtractString(properties, 'currency'),
    revenue_cents Int64  MATERIALIZED JSONExtractInt(properties, 'revenue_cents'),
    custom_name   String MATERIALIZED JSONExtractString(properties, 'name'),
    -- Profile reads filter on customer_id / anon_id with no channel predicate, so
    -- they get no help from the sort key - these keep them off a full scan
    INDEX idx_customer customer_id TYPE bloom_filter(0.01) GRANULARITY 4,
    INDEX idx_anon     anon_id     TYPE bloom_filter(0.01) GRANULARITY 4,
    -- created_at (arrival) rather than occurred_at: the forwarding cursor and the
    -- engagement fresh slice both filter on arrival, and parts are written in arrival
    -- order, so this prunes nearly every old granule for them. On occurred_at it would
    -- prune nothing the partition key had not already - inside a daily partition every
    -- granule's minmax spans the whole day, because the sort key scatters time
    INDEX idx_created created_at TYPE minmax GRANULARITY 4
)
ENGINE = ReplacingMergeTree(received_at)
PARTITION BY toDate(occurred_at)
ORDER BY (channel_id, dedupe_key);

-- analytics_identities: anon -> customer stitching. ReplacingMergeTree(last_seen)
-- keyed on anon_id gives last-write-wins (the Postgres upsert semantics).
-- first_seen and channel_id are carried forward by the driver's read-then-write,
-- mirroring Postgres, which leaves first_seen alone and COALESCEs channel_id.
CREATE TABLE IF NOT EXISTS analytics_identities (
    anon_id     String,
    customer_id UUID,
    channel_id  UUID,
    first_seen  DateTime64(3, 'UTC') DEFAULT now64(3),
    last_seen   DateTime64(3, 'UTC') DEFAULT now64(3)
)
ENGINE = ReplacingMergeTree(last_seen)
ORDER BY (anon_id);
