-- An integration token acting through the admin API is recorded beside the admin column it can't fill
-- No foreign key on the audit trail, the same as its admin_id: a row must outlive the integration, and a best-effort insert must not fail on a token deleted mid-request
ALTER TABLE audit_log ADD COLUMN integration_id uuid;
CREATE INDEX idx_audit_log_integration ON audit_log (integration_id, created_at DESC) WHERE integration_id IS NOT NULL;

ALTER TABLE reviews ADD COLUMN moderated_by_integration_id uuid REFERENCES integrations(id) ON DELETE SET NULL;
ALTER TABLE product_questions ADD COLUMN answered_by_integration_id uuid REFERENCES integrations(id) ON DELETE SET NULL;
ALTER TABLE store_credit_entries ADD COLUMN integration_id uuid REFERENCES integrations(id) ON DELETE SET NULL;
ALTER TABLE data_jobs ADD COLUMN integration_id uuid REFERENCES integrations(id) ON DELETE SET NULL;
