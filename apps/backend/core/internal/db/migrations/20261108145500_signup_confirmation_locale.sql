-- migrate:up
-- The storefront language a signup form was sent from, so the contact its confirmation makes gets their email in it
ALTER TABLE signup_confirmations ADD COLUMN locale text DEFAULT ''::text NOT NULL;

-- migrate:down
ALTER TABLE signup_confirmations DROP COLUMN locale;
