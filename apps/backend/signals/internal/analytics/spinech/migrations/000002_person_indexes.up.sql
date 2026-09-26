-- Profiles filter on customer_id OR anon_id, which only one bloom index over both columns can prune
ALTER TABLE analytics_events ADD INDEX IF NOT EXISTS idx_person (customer_id, anon_id) TYPE bloom_filter(0.01) GRANULARITY 4;
ALTER TABLE analytics_events MATERIALIZE INDEX idx_person;

-- A customer's anon ids and a channel's identities, on a table sorted by anon_id alone
ALTER TABLE analytics_identities ADD INDEX IF NOT EXISTS idx_customer customer_id TYPE bloom_filter(0.01) GRANULARITY 1;
ALTER TABLE analytics_identities ADD INDEX IF NOT EXISTS idx_channel channel_id TYPE bloom_filter(0.01) GRANULARITY 1;
ALTER TABLE analytics_identities MATERIALIZE INDEX idx_customer;
ALTER TABLE analytics_identities MATERIALIZE INDEX idx_channel;
