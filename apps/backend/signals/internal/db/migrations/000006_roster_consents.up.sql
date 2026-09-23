ALTER TABLE customer_roster ADD COLUMN consents jsonb NOT NULL DEFAULT '{}'::jsonb;
