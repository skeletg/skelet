ALTER TABLE analytics_identities DROP INDEX IF EXISTS idx_channel;
ALTER TABLE analytics_identities DROP INDEX IF EXISTS idx_customer;
ALTER TABLE analytics_events DROP INDEX IF EXISTS idx_person;
