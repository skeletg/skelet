-- migrate:up
-- A notice for one admin, such as a request for access to their board - everyone else's inbox skips it
ALTER TABLE admin_notifications ADD COLUMN recipient_admin_id uuid REFERENCES admin_users(id) ON DELETE CASCADE;

-- migrate:down
ALTER TABLE admin_notifications DROP COLUMN recipient_admin_id;
