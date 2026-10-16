ALTER TABLE data_jobs DROP COLUMN integration_id;
ALTER TABLE store_credit_entries DROP COLUMN integration_id;
ALTER TABLE product_questions DROP COLUMN answered_by_integration_id;
ALTER TABLE reviews DROP COLUMN moderated_by_integration_id;
DROP INDEX idx_audit_log_integration;
ALTER TABLE audit_log DROP COLUMN integration_id;
